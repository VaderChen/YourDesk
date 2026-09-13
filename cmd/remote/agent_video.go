package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"time"
	"yourdesk/internal/agentremote"
	"yourdesk/internal/agentvideo"
)

type agentVideoResult struct {
	out   agentremote.Response
	state agentvideo.State
	frame image.Image
}

func (g *game) applyAgentVideo() {
	if g.agentVideoResults == nil {
		return
	}
	select {
	case result := <-g.agentVideoResults:
		g.agentVideoBusy = false
		if result.out.Error == "" {
			g.mu.Lock()
			g.agentViewID = result.state.ViewID
			g.agentRegion = result.state.Region
			g.agentPaused = result.state.Mode == "paused"
			g.agentViewChanging = false
			if result.frame != nil {
				bounds := result.frame.Bounds()
				g.frame = agentvideo.Full().Crop(result.frame)
				g.width, g.height = bounds.Dx(), bounds.Dy()
				g.frameViewID = result.state.ViewID
				if g.agentHeadless {
					g.displayedViewID = result.state.ViewID
				}
				g.display, g.frameDisplay, g.displayedDisplay = result.state.Display, result.state.Display, result.state.Display
				g.displayCount = result.state.DisplayCount
				g.displayKnown = true
				g.displayPending = false
				g.dirty = true
			}
			g.mu.Unlock()
			if result.state.Mode == "streaming" {
				peer := g.peer
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					_, _ = peer.CallCommand(ctx, "video.keyframe")
				}()
			}
		}
		data, _ := json.Marshal(result.out)
		fmt.Fprintln(os.Stdout, agentremote.Prefix+string(data))
	default:
	}
}
func (g *game) dispatchAgentVideo(r agentremote.Request) bool {
	if r.Action != "video.stream" && r.Action != "video.snapshot" {
		return false
	}
	replyError := func(err error) {
		data, _ := json.Marshal(agentremote.Response{ID: r.ID, Error: err.Error()})
		fmt.Fprintln(os.Stdout, agentremote.Prefix+string(data))
	}
	if !g.controlEnabled || g.peer == nil || !g.peer.Connected() {
		replyError(fmt.Errorf("遠端尚未連線或控制已停用"))
		return true
	}
	var request agentvideo.Request
	if err := json.Unmarshal(r.Params, &request); err != nil {
		replyError(err)
		return true
	}
	if err := request.Validate(); err != nil {
		replyError(err)
		return true
	}
	if time.Now().UnixMilli() >= r.Expires {
		replyError(fmt.Errorf("操作已逾時"))
		return true
	}
	if !g.peer.SupportsCommand(r.Action) {
		out := g.legacyAgentVideo(r, request)
		data, _ := json.Marshal(out)
		fmt.Fprintln(os.Stdout, agentremote.Prefix+string(data))
		return true
	}
	if g.agentVideoBusy {
		replyError(fmt.Errorf("視野切換或截圖進行中"))
		return true
	}
	g.agentVideoBusy = true
	if g.agentVideoResults == nil {
		g.agentVideoResults = make(chan agentVideoResult, 1)
	}
	g.clipboard.CancelKeys("視野切換")
	g.releaseRawKeys("視野切換")
	g.interpolation.reset()
	if g.ml != nil {
		g.ml.reset()
	}
	clear(g.agentButtons)
	clear(g.lastButtons)
	clear(g.lastKeys)
	g.mu.Lock()
	g.agentViewChanging = true
	g.mu.Unlock()
	peer, results := g.peer, g.agentVideoResults
	go func() {
		result := agentVideoResult{out: agentremote.Response{ID: r.ID}}
		defer func() { results <- result }()
		ctx, cancel := context.WithDeadline(context.Background(), time.UnixMilli(r.Expires))
		defer cancel()
		response, err := peer.CallCommandParams(ctx, r.Action, r.Params)
		if err != nil {
			result.out.Error = err.Error()
			return
		}
		if err = json.Unmarshal(response.Result, &result.state); err != nil || result.state.ViewID == 0 || result.state.Region.Validate() != nil {
			result.out.Error = "遠端視野回覆無效"
			return
		}
		result.out.Result = response.Result
		if r.Action != "video.snapshot" {
			return
		}
		state := result.state
		if state.Token == "" || state.Bytes <= 0 || state.Bytes > agentvideo.MaxBytes || state.Width < 1 || state.Height < 1 || max(state.Width, state.Height) > 1280 {
			result.out.Error = "遠端截圖大小無效"
			return
		}
		defer func() {
			closeCtx, done := context.WithTimeout(context.Background(), time.Second)
			defer done()
			params, _ := json.Marshal(agentvideo.Read{Token: state.Token})
			_, _ = peer.CallCommandParams(closeCtx, "video.snapshot.close", params)
		}()
		data := make([]byte, state.Bytes)
		// 三個讀取同時進行，保留一個 P2P 指令工作槽給其他控制。
		for base := 0; base < len(data); base += 3 * agentvideo.ChunkSize {
			type chunk struct {
				offset int
				data   []byte
				err    error
			}
			ch := make(chan chunk, 3)
			n := 0
			for offset := base; offset < min(base+3*agentvideo.ChunkSize, len(data)); offset += agentvideo.ChunkSize {
				n++
				go func(offset int) {
					params, _ := json.Marshal(agentvideo.Read{Token: state.Token, Offset: offset})
					response, err := peer.CallCommandParams(ctx, "video.snapshot.read", params)
					var b []byte
					if err == nil {
						err = json.Unmarshal(response.Result, &b)
					}
					ch <- chunk{offset, b, err}
				}(offset)
			}
			var failure error
			for i := 0; i < n; i++ {
				part := <-ch
				if part.err != nil {
					failure = part.err
					continue
				}
				if len(part.data) != min(agentvideo.ChunkSize, len(data)-part.offset) {
					failure = fmt.Errorf("截圖區塊大小不符")
					continue
				}
				copy(data[part.offset:], part.data)
			}
			if failure != nil {
				result.out.Error = failure.Error()
				return
			}
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width != state.Width || config.Height != state.Height {
			result.out.Error = "截圖尺寸不符"
			return
		}
		result.frame, err = png.Decode(bytes.NewReader(data))
		if err != nil {
			result.out.Error = err.Error()
			return
		}
		result.out.Image = data
		result.out.Width, result.out.Height = state.Width, state.Height
		result.out.Display, result.out.DisplayCount = state.Display, state.DisplayCount
	}()
	return true
}

// Host 註冊指令後會再次公告能力；等待公告，不將舊版 Host 誤認為已暫停。
func (g *game) initializePausedVideo(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for !g.peer.SupportsCommand("video.stream") {
		select {
		case <-ctx.Done():
			if parent.Err() != nil {
				return parent.Err()
			}
			g.mu.Lock()
			g.agentViewChanging = false
			g.mu.Unlock()
			emitUIEvent("video-compatibility", "舊版 Host 不支援暫停串流，已改用全螢幕串流；截圖使用最近收到的畫面。")
			if g.peer.SupportsCommand("video.keyframe") {
				requestCtx, done := context.WithTimeout(parent, 2*time.Second)
				_, _ = g.peer.CallCommand(requestCtx, "video.keyframe")
				done()
			}
			return nil
		case <-g.peer.Done():
			return fmt.Errorf("遠端連線已結束")
		case <-ticker.C:
		}
	}
	response, err := g.peer.CallCommandParams(ctx, "video.stream", json.RawMessage(`{"mode":"paused"}`))
	if err != nil {
		return err
	}
	var state agentvideo.State
	if err = json.Unmarshal(response.Result, &state); err != nil || state.Mode != "paused" || state.ViewID == 0 {
		return fmt.Errorf("遠端未確認暫停串流")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.agentViewID = state.ViewID
	g.agentRegion = state.Region
	g.agentPaused = true
	g.agentViewChanging = false
	g.display = state.Display
	g.displayCount = state.DisplayCount
	g.displayKnown = true
	return nil
}

// 舊版只接受全螢幕語義；拒絕不能實現的區域／暫停，不修改座標狀態。
func (g *game) legacyAgentVideo(r agentremote.Request, request agentvideo.Request) agentremote.Response {
	out := agentremote.Response{ID: r.ID}
	if request.Region != nil && *request.Region != agentvideo.Full() {
		out.Error = "舊版 Host 不支援指定區域，請使用全螢幕或更新 Host"
		return out
	}
	if request.Mode == "paused" {
		out.Error = "舊版 Host 不支援暫停串流，目前仍為全螢幕串流"
		return out
	}
	g.mu.RLock()
	active := g.agentViewID != 0 || g.agentViewChanging
	display, count := g.display, g.displayCount
	g.mu.RUnlock()
	if active {
		out.Error = "遠端缺少所需指令，無法切回舊版全螢幕模式"
		return out
	}
	if r.Action == "video.snapshot" {
		r.Action = "screenshot"
		out = g.agentAction(r)
	}
	metadata := map[string]any{"mode": "streaming", "region": agentvideo.Full(), "viewID": 0, "display": display, "displayCount": count, "legacy": true, "fresh": false, "notice": "舊版 Host：全螢幕串流；截圖為最近收到的影格，不是遠端按需擷取。"}
	out.Result, _ = json.Marshal(metadata)
	return out
}

// 狀態查詢不等待影格、不改變串流，也不要求啟用鍵鼠控制。
func (g *game) agentVideoStatus(r agentremote.Request) agentremote.Response {
	out := agentremote.Response{ID: r.ID}
	if g.peer == nil || !g.peer.Connected() {
		out.Error = "遠端尚未連線"
		return out
	}
	stream := g.peer.SupportsCommand("video.stream")
	snapshot := g.peer.SupportsCommand("video.snapshot") && g.peer.SupportsCommand("video.snapshot.read") && g.peer.SupportsCommand("video.snapshot.close")
	ready := g.displayInputReady()
	g.mu.RLock()
	defer g.mu.RUnlock()
	mode := "streaming"
	if g.agentPaused {
		mode = "paused"
	}
	region := g.agentRegion
	if g.agentViewID == 0 {
		region = agentvideo.Full()
	}
	status := map[string]any{"contractVersion": 1, "mode": mode, "region": region, "viewID": g.agentViewID, "display": g.display, "displayCount": g.displayCount, "inputReady": ready && g.frame != nil, "changing": g.agentViewChanging, "controlEnabled": g.controlEnabled, "capabilitiesKnown": g.peer.RemoteCommands().Version == 1, "capabilities": map[string]bool{"region": stream, "pause": stream, "freshSnapshot": snapshot}}
	out.Result, _ = json.Marshal(status)
	return out
}
