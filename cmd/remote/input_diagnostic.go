package main

import (
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"yourdesk/internal/authlog"
)

// 僅記錄類別與數量，不保留按鍵、座標或剪貼簿內容。
// queued 表示已交給剪貼簿輸入佇列，不代表遠端已執行。
type inputDiagnostic struct {
	last                             time.Time
	move, button, key, wheel, failed atomic.Uint64
}

func (d *inputDiagnostic) record(kind string, err error) {
	if !authlog.IsEnabled() {
		return
	}
	if err != nil {
		d.failed.Add(1)
		return
	}
	switch kind {
	case "move":
		d.move.Add(1)
	case "button":
		d.button.Add(1)
	case "key", "raw-key":
		d.key.Add(1)
	case "wheel":
		d.wheel.Add(1)
	}
}

func (g *game) reportInputDiagnostic() {
	d := &g.inputDiagnostic
	if !authlog.IsEnabled() {
		d.last = time.Time{}
		d.move.Store(0)
		d.button.Store(0)
		d.key.Store(0)
		d.wheel.Store(0)
		d.failed.Store(0)
		return
	}
	now := time.Now()
	if now.Sub(d.last) < 5*time.Second {
		return
	}
	d.last = now
	fields := map[string]any{
		"focused": ebiten.IsFocused(), "controlEnabled": g.controlEnabled,
		"popupOpen": nativeTitlebarPopupOpen(), "cropBlocksInput": g.crop.blockInput(),
		"displayInputReady": g.displayInputReady(), "rawActive": g.rawActive,
		"rawSupported": g.rawSupported.Load(), "rawAvailable": rawKeyboardAvailable(),
		"queuedMove": d.move.Swap(0), "queuedButton": d.button.Swap(0),
		"queuedKey": d.key.Swap(0), "queuedWheel": d.wheel.Swap(0),
		"queueFailed": d.failed.Swap(0),
		"canvasWidth": g.finalWidth, "canvasHeight": g.finalHeight,
		"displayedWidth": g.displayedWidth, "displayedHeight": g.displayedHeight,
	}
	g.mu.RLock()
	fields["displayKnown"], fields["displayPending"] = g.displayKnown, g.displayPending
	fields["display"], fields["displayedDisplay"], fields["displayCount"] = g.display, g.displayedDisplay, g.displayCount
	fields["viewID"], fields["displayedViewID"], fields["viewChanging"] = g.agentViewID, g.displayedViewID, g.agentViewChanging
	g.mu.RUnlock()
	authlog.Event("viewer-input", fields)
}
