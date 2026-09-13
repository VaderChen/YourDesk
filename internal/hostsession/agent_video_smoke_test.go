//go:build darwin || linux

package hostsession

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"yourdesk/internal/agentremote"
	"yourdesk/internal/agentvideo"
	"yourdesk/internal/desktop"
	"yourdesk/internal/p2p"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
)

// 真實 Remote 子程序、密碼驗證、P2P 與分塊截圖；影像與輸入接收端使用測試資料。
func TestAgentVideoHelperSmoke(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "current"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) { agentVideoHelperSmoke(t, legacy) })
	}
}
func agentVideoHelperSmoke(t *testing.T, legacy bool) {
	binary := os.Getenv("YOURDESK_AGENT_VIDEO_SMOKE_BINARY")
	if binary == "" {
		t.Skip("需指定已建置 Remote")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	text, _ := security.NewSecret()
	secret, _ := security.DecodeSecret(text)
	inputs := make(chan p2p.Control, 20)
	errs := make(chan error, 2)
	go func() {
		errs <- signaling.ListenDirect(ctx, address, secret, func(c context.Context, s *signaling.Client) {
			peer, e := p2p.NewHost(c, s, func(control p2p.Control) {
				if control.Type == "move" {
					inputs <- control
				}
			})
			if e != nil {
				errs <- e
				return
			}
			defer peer.Close()
			v := newAgentVideo()
			captures := 0
			source := image.NewRGBA(image.Rect(0, 0, 640, 360))
			for y := 0; y < 360; y++ {
				for x := 0; x < 640; x++ {
					source.Set(x, y, color.RGBA{uint8(x*71 + y*53), uint8(x*31 + y*17), uint8(x*7 + y*29), 255})
				}
			}
			if !legacy {
				v.register(peer, func(ctx context.Context, r agentvideo.Request) (agentvideo.State, error) {
					v.view.ViewID++
					if r.Mode != "" {
						v.view.Mode = r.Mode
					}
					if r.FullScreen {
						v.view.Region = agentvideo.Full()
					}
					if r.Region != nil {
						v.view.Region = *r.Region
					}
					v.view.DisplayCount = 1
					return v.view, nil
				}, func(context.Context, agentvideo.State) (image.Image, error) {
					captures++
					img := agentvideo.Full().Crop(source)
					img.Set(160, 90, color.RGBA{uint8(captures), 0, 0, 255})
					return img, nil
				}, func() bool { return c.Err() == nil })
				_ = peer.RegisterCommand("video.keyframe", func(context.Context) (any, error) { return map[string]bool{"accepted": true}, nil })
			}
			tick := time.NewTicker(40 * time.Millisecond)
			defer tick.Stop()
			var seq uint64
			for {
				select {
				case <-c.Done():
					return
				case <-peer.Done():
					return
				case <-tick.C:
				}
				v.gate.Lock()
				state := v.view
				if state.Mode == "streaming" {
					payload, e := desktop.JPEG(state.Region.Crop(source), 70)
					if e == nil {
						seq++
						bounds := state.Region.Bounds(source.Bounds())
						_ = peer.SendFrameLimited(c, p2p.Frame{ViewID: state.ViewID, Sequence: seq, Display: 0, Width: uint32(bounds.Dx()), Height: uint32(bounds.Dy()), Keyframe: true, JPEG: payload}, 12000000)
					}
				}
				v.gate.Unlock()
			}
		})
	}()
	for {
		conn, e := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if e == nil {
			conn.Close()
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case e := <-errs:
			t.Fatal(e)
		case <-time.After(20 * time.Millisecond):
		}
	}
	cmd := exec.CommandContext(ctx, binary, "-room", address, "-secret-stdin", "-interactive-auth", "-mcp-managed", "-mcp-hidden", "-mcp-paused", "-codec", "software-jpeg")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	lines := make(chan string, 100)
	go func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 16<<20)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		close(lines)
	}()
	json.NewEncoder(input).Encode(map[string]string{"secret": text})
	wait := func(match func(string) bool) string {
		t.Helper()
		for {
			select {
			case line, ok := <-lines:
				if !ok {
					t.Fatal("Remote 提前結束")
				}
				if match(line) {
					return line
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}
	wait(func(line string) bool {
		if legacy {
			return strings.Contains(line, `"event":"frame"`)
		}
		return strings.Contains(line, `"event":"agent-ready"`)
	})
	sequence := 0
	call := func(action string, params string, wantError ...bool) agentremote.Response {
		t.Helper()
		sequence++
		id := fmt.Sprint(sequence)
		request := agentremote.Request{ID: id, Action: action, Params: json.RawMessage(params), X: .5, Y: .5, Expires: time.Now().Add(30 * time.Second).UnixMilli()}
		data, _ := json.Marshal(map[string]any{"agent": request})
		fmt.Fprintln(input, string(data))
		var response agentremote.Response
		wait(func(line string) bool {
			if !strings.HasPrefix(line, agentremote.Prefix) {
				return false
			}
			_ = json.Unmarshal([]byte(strings.TrimPrefix(line, agentremote.Prefix)), &response)
			return response.ID == id
		})
		if len(wantError) > 0 && wantError[0] {
			if response.Error == "" {
				t.Fatal("舊版意外接受新功能")
			}
			return response
		}
		if response.Error != "" {
			t.Fatalf("%s: %s", action, response.Error)
		}
		return response
	}

	status := call("video.status", `{}`)
	var statusBody struct {
		ContractVersion int    `json:"contractVersion"`
		Mode            string `json:"mode"`
		Capabilities    struct {
			Region bool `json:"region"`
			Pause  bool `json:"pause"`
			Fresh  bool `json:"freshSnapshot"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(status.Result, &statusBody); err != nil {
		t.Fatal(err)
	}
	if statusBody.ContractVersion != 1 || statusBody.Capabilities.Region == legacy || statusBody.Capabilities.Pause == legacy || statusBody.Capabilities.Fresh == legacy {
		t.Fatalf("能力契約不符 %s", status.Result)
	}
	if legacy && statusBody.Mode != "streaming" || !legacy && statusBody.Mode != "paused" {
		t.Fatalf("實際模式不符 %s", status.Result)
	}
	if legacy {
		snapshot := call("video.snapshot", `{"fullScreen":true}`)
		if snapshot.Width != 640 || snapshot.Height != 360 || len(snapshot.Image) == 0 || !strings.Contains(string(snapshot.Result), `"legacy":true`) {
			t.Fatal("舊版全螢幕截圖失敗")
		}
		call("video.snapshot", `{"region":{"x":0.25,"y":0.25,"width":0.5,"height":0.5}}`, true)
		call("video.stream", `{"mode":"paused"}`, true)
		call("video.stream", `{"mode":"streaming","fullScreen":true}`)
		call("move", `{}`)
		select {
		case point := <-inputs:
			if point.ViewID != 0 || point.X != .5 || point.Y != .5 {
				t.Fatal("舊版座標遭修改")
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		return
	}
	result := call("video.snapshot", `{"region":{"x":0.25,"y":0.25,"width":0.5,"height":0.5}}`)
	if result.Width != 320 || result.Height != 180 || len(result.Image) < agentvideo.ChunkSize {
		t.Fatalf("截圖尺寸或分塊未通過 %dx%d %d bytes", result.Width, result.Height, len(result.Image))
	}
	var state agentvideo.State
	_ = json.Unmarshal(result.Result, &state)
	if state.Mode != "paused" {
		t.Fatal("截圖意外恢復串流")
	}
	call("move", `{}`)
	select {
	case point := <-inputs:
		if point.ViewID != state.ViewID || point.X != .5 || point.Y != .5 {
			t.Fatalf("座標/視野錯誤 %+v", point)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	result = call("video.stream", `{"mode":"streaming","fullScreen":true}`)
	_ = json.Unmarshal(result.Result, &state)
	if state.Mode != "streaming" || state.Region != agentvideo.Full() {
		t.Fatal("恢復失敗")
	}
	// 真實影格走延伸標頭、重組、解碼及隱藏 Viewer 的呈現路徑。
	time.Sleep(500 * time.Millisecond)
	call("move", `{}`)
	select {
	case point := <-inputs:
		if point.ViewID != state.ViewID {
			t.Fatal("恢復後仍使用舊視野")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	call("video.stream", `{"mode":"paused"}`)
	call("video.snapshot", `{}`)
}
