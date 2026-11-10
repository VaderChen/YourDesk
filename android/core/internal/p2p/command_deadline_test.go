package p2p

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestCommandDeadlineClockSkew(t *testing.T) {
	now := time.Now()
	for _, offset := range []time.Duration{-24 * time.Hour, -time.Minute, 0, time.Minute, 24 * time.Hour} {
		budget := int64(1250)
		r := CommandRequest{Method: "terminal.open", Expires: now.Add(offset + 5*time.Second).UnixMilli(), TimeoutMS: &budget}
		deadline, code, message := commandDeadline(r, now)
		if code != "" || deadline.Sub(now) != 1250*time.Millisecond {
			t.Fatalf("時差 %v：%v %s %s", offset, deadline.Sub(now), code, message)
		}
		// 同一封舊版請求仍按原規則拒絕，不以重試來掩蓋時鐘誤差。
		r.TimeoutMS = nil
		_, code, _ = commandDeadline(r, now)
		want := ""
		if offset < 0 {
			want = "expired"
		} else if offset > 0 {
			want = "invalid_request"
		}
		if code != want {
			t.Fatalf("舊版時差 %v：%s，預期 %s", offset, code, want)
		}
	}
	for _, budget := range []int64{-1, 0, 5001, 1<<63 - 1} {
		_, code, _ := commandDeadline(CommandRequest{Method: "ping", Expires: now.Add(time.Second).UnixMilli(), TimeoutMS: &budget}, now)
		if code != "invalid_request" {
			t.Fatalf("接受無效預算 %d：%s", budget, code)
		}
	}
}

func TestCommandTimeoutNegotiation(t *testing.T) {
	for _, relative := range []bool{false, true} {
		p := &Peer{}
		capabilities := p.localCommands()
		if !capabilities.RelativeTimeout {
			t.Fatal("未公告相對期限能力")
		}
		capabilities.RelativeTimeout = relative
		p.handleCommand(Control{Type: "command-capabilities", CommandCapabilities: &capabilities})
		r := CommandRequest{}
		if err := r.setTimeout(time.Now().Add(200*time.Millisecond), p.RemoteCommands().RelativeTimeout); err != nil {
			t.Fatal(err)
		}
		if relative && (r.TimeoutMS == nil || *r.TimeoutMS < 1 || *r.TimeoutMS > 200) {
			t.Fatalf("未保留呼叫端較短的預算：%+v", r)
		}
		wire, err := json.Marshal(r)
		if err != nil || strings.Contains(string(wire), "timeoutMs") != relative {
			t.Fatalf("舊版封包相容性：%s %v", wire, err)
		}
	}
	r := CommandRequest{}
	if err := r.setTimeout(time.Now().Add(-time.Second), true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("送出已取消期限的請求", err)
	}
}

func TestRelativeCommandHandlerLifetime(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			p := &Peer{done: make(chan struct{})}
			ended := make(chan error, 1)
			if err := p.RegisterCommand("terminal.wait", func(ctx context.Context) (any, error) {
				<-ctx.Done()
				ended <- ctx.Err()
				return nil, ctx.Err()
			}); err != nil {
				t.Fatal(err)
			}
			budget := int64(50)
			request := &CommandRequest{Version: CommandVersion, ID: 1, Method: "terminal.wait", Expires: time.Now().Add(24 * time.Hour).UnixMilli(), TimeoutMS: &budget}
			p.handleCommand(Control{Type: "command-request", CommandRequest: request})
			synctest.Wait()
			time.Sleep(40 * time.Millisecond)
			// 重複請求不能延長原有 handler 的期限。
			p.handleCommand(Control{Type: "command-request", CommandRequest: request})
			want := context.DeadlineExceeded
			if disconnect {
				close(p.done)
				want = context.Canceled
			} else {
				time.Sleep(10 * time.Millisecond)
			}
			synctest.Wait()
			select {
			case err := <-ended:
				if !errors.Is(err, want) {
					t.Fatal(err)
				}
			default:
				t.Fatal("相對期限屆滿或 Peer 關閉後 handler 未取消")
			}
		})
	}
}
