package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/image/draw"
	"image"
	"image/png"
	"os"
	"time"
	"yourdesk/internal/agentremote"
	"yourdesk/internal/p2p"
)

// 操作僅在遠端顯示主迴圈執行，避免與滑鼠、螢幕切換狀態競爭。
func (g *game) updateAgent() {
	if !g.agentDeadline.IsZero() && (!g.controlEnabled || time.Now().After(g.agentDeadline)) {
		g.clipboard.CancelKeys("Agent 操作逾時或停用")
		for _, c := range g.agentButtons {
			c.Down = false
			_ = g.sendControl(c)
		}
		clear(g.agentButtons)
		g.agentDeadline = time.Time{}
	}
	if g.agentRequests == nil {
		return
	}
	select {
	case req := <-g.agentRequests:
		res := g.agentAction(req)
		data, _ := json.Marshal(res)
		fmt.Fprintln(os.Stdout, agentremote.Prefix+string(data))
	default:
	}
}
func (g *game) agentAction(r agentremote.Request) (out agentremote.Response) {
	out.ID = r.ID
	if time.Now().UnixMilli() > r.Expires {
		out.Error = "操作已逾時，未執行"
		return
	}
	if r.Action == "hide" {
		g.agentShow = false
		if !g.agentHeadless {
			nativeHideAgentWindow()
		}
		return
	}
	if r.Action == "show" {
		g.agentShow = true
		if !g.agentHeadless {
			nativeShowAgentWindow()
		}
		return
	}
	if g.peer == nil || !g.displayInputReady() {
		out.Error = "遠端畫面尚未就緒"
		return
	}
	g.mu.RLock()
	out.Display, out.DisplayCount = g.display, g.displayCount
	frame := g.frame
	var snapshot *image.RGBA
	if r.Action == "screenshot" && frame != nil {
		snapshot = image.NewRGBA(frame.Bounds())
		copy(snapshot.Pix, frame.Pix)
	}
	g.mu.RUnlock()
	if r.Action == "screenshot" {
		if snapshot == nil {
			out.Error = "尚未收到畫面"
			return
		}
		w, h := snapshot.Bounds().Dx(), snapshot.Bounds().Dy()
		if w > 1280 || h > 1280 {
			scale := 1280.0 / float64(max(w, h))
			w = max(1, int(float64(w)*scale))
			h = max(1, int(float64(h)*scale))
			small := image.NewRGBA(image.Rect(0, 0, w, h))
			draw.ApproxBiLinear.Scale(small, small.Bounds(), snapshot, snapshot.Bounds(), draw.Src, nil)
			snapshot = small
		}
		var b bytes.Buffer
		if err := png.Encode(&b, snapshot); err != nil {
			out.Error = err.Error()
			return
		}
		out.Image = b.Bytes()
		out.Width = w
		out.Height = h
		return
	}
	if !g.controlEnabled {
		out.Error = "遠端控制已停用，請在遠端顯示中啟用"
		return
	}
	g.agentDeadline = time.Now().Add(30 * time.Second)
	var err error
	switch r.Action {
	case "move", "button":
		if r.X < 0 || r.X > 1 || r.Y < 0 || r.Y > 1 || r.Button < 0 || r.Button > 2 {
			out.Error = "座標須為 0～1，按鈕須為 0～2"
			return
		}
		c := p2p.Control{Type: r.Action, X: r.X, Y: r.Y, Button: r.Button + 1, Down: r.Down}
		err = g.sendControl(c)
		if err == nil && r.Action == "button" {
			if g.agentButtons == nil {
				g.agentButtons = map[int]p2p.Control{}
			}
			if r.Down {
				g.agentButtons[r.Button] = c
			} else {
				delete(g.agentButtons, r.Button)
			}
		}
	case "key":
		valid := false
		for _, k := range commonKeys {
			if k == r.Key {
				valid = true
				break
			}
		}
		if !valid {
			out.Error = "不支援的按鍵"
			return
		}
		err = g.sendControl(p2p.Control{Type: "key", Key: r.Key, Down: r.Down})
	case "scroll":
		if r.Delta < -100 || r.Delta > 100 {
			out.Error = "捲動量須為 -100～100"
			return
		}
		err = g.sendControl(p2p.Control{Type: "wheel", Delta: r.Delta})
	case "text":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = g.clipboard.PasteAgentText(ctx, r.Text)
	case "display":
		if r.Display < 0 || r.Display >= out.DisplayCount {
			out.Error = "螢幕編號不存在"
			return
		}
		g.selectDisplay(r.Display)
	default:
		out.Error = "不支援的操作"
		return
	}
	if err != nil {
		out.Error = err.Error()
	}
	return
}

// 背景模式沿用相同連線與解碼器；使用者開啟前不建立顯示視窗。
func (g *game) runAgentHidden(ctx context.Context) bool {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-g.peer.Done():
			return false
		case <-ticker.C:
		}
		g.mu.Lock()
		ready := g.frame != nil && !g.displayPending
		if ready {
			g.displayedDisplay = g.frameDisplay
			g.finalWidth = g.width
			g.finalHeight = g.height + g.toolbarHeight()
		}
		g.mu.Unlock()
		if ready && !g.firstPresented {
			g.firstPresented = true
			emitUIEvent("frame", "")
		}
		g.updateAgent()
		if ready {
			g.updateQuality()
		}
		if g.agentShow {
			g.agentHeadless = false
			return true
		}
	}
}
