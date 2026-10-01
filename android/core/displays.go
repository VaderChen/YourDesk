package core

import (
	"encoding/json"
	"fmt"
	"time"
	"yourdeskandroid/core/internal/p2p"
)

// 狀態只接受目前連線的 Host 回報；清單與確認不依賴畫面頁面的載入時序。
type displayState struct {
	Known     bool   `json:"known"`
	Count     int    `json:"count"`
	Current   int    `json:"current"`
	Requested int    `json:"requested"`
	Request   uint64 `json:"request"`
	Pending   bool   `json:"pending"`
	Error     string `json:"error"`
	deadline  time.Time
}

func (v *Viewer) resetDisplaysLocked() { v.displays = displayState{Current: -1, Requested: -1} }

func (v *Viewer) receiveDisplaysLocked(c p2p.Control) {
	if c.Display == nil || c.DisplayCount < 0 || c.DisplayCount > 64 ||
		(c.DisplayCount > 0 && (*c.Display < 0 || *c.Display >= c.DisplayCount)) {
		return
	}
	d := &v.displays
	// 舊確認不能覆蓋更新的選擇；即使逾時，晚到的同一要求仍可恢復。
	if c.DisplayRequest < d.Request {
		return
	}
	if c.DisplayRequest > d.Request {
		return
	}
	d.Known, d.Count, d.Current = true, c.DisplayCount, *c.Display
	if d.Count == 0 {
		d.Current = -1
	}
	if d.Request > 0 && (d.Pending || d.Error != "") {
		d.Pending, d.Error = false, ""
		if d.Current != d.Requested {
			d.Error = "遠端未切換至指定螢幕，已同步目前螢幕"
		}
	}
}

// SelectDisplay 使用既有 display-select 協定；成功送出不代表 Host 已套用。
func (v *Viewer) SelectDisplay(display int) error {
	v.displaySendMu.Lock()
	defer v.displaySendMu.Unlock()
	v.mu.Lock()
	p, generation := v.peer, v.generation
	if p == nil || !v.displays.Known || display < 0 || display >= v.displays.Count {
		v.mu.Unlock()
		return fmt.Errorf("遠端尚未提供可選擇的螢幕")
	}
	d := &v.displays
	d.Request++
	request := d.Request
	d.Requested, d.Pending, d.Error, d.deadline = display, true, "", time.Now().Add(5*time.Second)
	v.mu.Unlock()
	err := p.SendControl(p2p.Control{Type: "display-select", Display: &display, DisplayRequest: request})
	if err != nil {
		v.mu.Lock()
		if v.generation == generation && v.displays.Request == request {
			v.displays.Pending, v.displays.Error = false, "螢幕切換傳送失敗，請重試"
		}
		v.mu.Unlock()
	}
	return err
}

// DisplayStateJSON 供原生 frame pump 輪詢；未確認時禁止將舊畫面當成新螢幕。
func (v *Viewer) DisplayStateJSON() string {
	v.mu.Lock()
	if v.displays.Pending && time.Now().After(v.displays.deadline) {
		v.displays.Pending, v.displays.Error = false, "螢幕切換逾時，請重試或重新連線"
	}
	state := v.displays
	v.mu.Unlock()
	b, _ := json.Marshal(state)
	return string(b)
}
