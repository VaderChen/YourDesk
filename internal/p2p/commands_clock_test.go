package p2p

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// 修改收到的對端 Unix 期限來模擬時差，不修改作業系統時間。
// 實際能力協商、請求及回覆均走 WebRTC DataChannel。
func TestCommandClockSkewAcrossDataChannel(t *testing.T) {
	a, b := clipboardTestPeers(t, nil)
	var offset atomic.Int64
	var wireBudget atomic.Int64
	var executions atomic.Int64
	requests := make(chan CommandRequest, 8)
	if err := b.RegisterCommand("terminal.clock-smoke", func(ctx context.Context) (any, error) {
		executions.Add(1)
		deadline, _ := ctx.Deadline()
		return map[string]int64{"remainingMs": time.Until(deadline).Milliseconds()}, nil
	}); err != nil {
		t.Fatal(err)
	}
	clipboardEventually(t, func() bool { return a.SupportsCommand("terminal.clock-smoke") })
	b.mu.RLock()
	dc := b.control
	b.mu.RUnlock()
	b.onMessage(dc, func(m webrtc.DataChannelMessage) {
		var c Control
		if decodeControl(m.Data, &c) != nil {
			return
		}
		if r := c.CommandRequest; r != nil {
			requests <- *r
			r.Expires += offset.Load()
			wireBudget.Store(0)
			if r.TimeoutMS != nil {
				wireBudget.Store(*r.TimeoutMS)
			}
		}
		b.handleCommand(c)
	})
	for _, skew := range []time.Duration{-24 * time.Hour, 24 * time.Hour} {
		offset.Store(skew.Milliseconds())
		out, err := a.CallCommand(context.Background(), "terminal.clock-smoke")
		if err != nil {
			t.Fatalf("新版時差 %v：%+v %v", skew, out, err)
		}
		if budget := wireBudget.Load(); budget < 1 || budget > 5000 {
			t.Fatalf("未送出有限相對預算：%d", budget)
		}
		var result struct {
			RemainingMS int64 `json:"remainingMs"`
		}
		if err := json.Unmarshal(out.Result, &result); err != nil || result.RemainingMS <= 0 || result.RemainingMS > 5000 {
			t.Fatalf("未以接收端本機時間建立期限：%s %v", out.Result, err)
		}
		// 同一 ID 的封包經通道重送只能取回原結果，不能重複開啟 Shell／寫入。
		request := <-requests
		replies := make(chan CommandResponse, 1)
		a.commands.mu.Lock()
		a.commands.pending[request.ID] = replies
		a.commands.mu.Unlock()
		if err := a.SendControl(Control{Type: "command-request", CommandRequest: &request}); err != nil {
			t.Fatal(err)
		}
		select {
		case cached := <-replies:
			if cached.Code != "" || string(cached.Result) != string(out.Result) {
				t.Fatalf("未保留原執行結果：%+v", cached)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("重複請求沒有回覆")
		}
		<-requests
		a.commands.mu.Lock()
		delete(a.commands.pending, request.ID)
		a.commands.mu.Unlock()
	}
	if executions.Load() != 2 {
		t.Fatal("重複執行", executions.Load())
	}
	// 舊版未公告相對期限時，傳送端不得逕自變更協定。
	a.commands.mu.Lock()
	a.commands.remote.RelativeTimeout = false
	a.commands.mu.Unlock()
	for _, tc := range []struct {
		skew time.Duration
		code string
	}{{-time.Minute, "expired"}, {time.Minute, "invalid_request"}, {0, ""}} {
		offset.Store(tc.skew.Milliseconds())
		out, err := a.CallCommand(context.Background(), "ping")
		if out.Code != tc.code {
			t.Fatalf("舊版時差 %v：%+v %v", tc.skew, out, err)
		}
		<-requests
		if wireBudget.Load() != 0 {
			t.Fatal("送給舊版的請求多出相對期限")
		}
	}
}
