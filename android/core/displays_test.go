package core

import (
	"encoding/json"
	"testing"
	"time"
	"yourdeskandroid/core/internal/p2p"
)

func TestDisplayReportsValidateSessionAndSelection(t *testing.T) {
	v := NewViewer()
	_, cancel, epoch := v.beginConnect()
	defer cancel()
	current := 1
	report := p2p.Control{Type: "displays", Display: &current, DisplayCount: 3}
	v.receiveStreamControl(report, epoch-1)
	if v.displays.Known {
		t.Fatal("舊連線不應更新螢幕")
	}
	v.receiveStreamControl(report, epoch)
	if !v.displays.Known || v.displays.Current != 1 || v.displays.Count != 3 {
		t.Fatal(v.displays)
	}
	v.displays.Request, v.displays.Requested, v.displays.Pending = 2, 2, true
	report.DisplayRequest = 1
	v.receiveStreamControl(report, epoch)
	if !v.displays.Pending || v.displays.Current != 1 {
		t.Fatal("舊確認覆蓋新選擇")
	}
	report.DisplayRequest, current = 2, 2
	v.receiveStreamControl(report, epoch)
	if v.displays.Pending || v.displays.Current != 2 || v.displays.Error != "" {
		t.Fatal(v.displays)
	}
	report.DisplayCount, current = 1, 0
	v.receiveStreamControl(report, epoch)
	if v.displays.Current != 0 || v.displays.Count != 1 {
		t.Fatal("螢幕移除後未同步")
	}
	for _, count := range []int{-1, 65, 1} {
		report.DisplayCount, current = count, 2
		v.receiveStreamControl(report, epoch)
		if v.displays.Count != 1 || v.displays.Current != 0 {
			t.Fatal("接受無效清單")
		}
	}
	v.Close()
	if v.displays.Known || v.displays.Request != 0 || v.displays.Current != -1 {
		t.Fatal("斷線未清除螢幕")
	}
}

func TestDisplayTimeoutCanRecoverAndRejectsOfflineSelection(t *testing.T) {
	v := NewViewer()
	if v.SelectDisplay(0) == nil {
		t.Fatal("離線不應切換")
	}
	v.displays = displayState{Known: true, Count: 2, Current: 0, Requested: 1, Request: 1, Pending: true, deadline: time.Now().Add(-time.Second)}
	var state displayState
	if err := json.Unmarshal([]byte(v.DisplayStateJSON()), &state); err != nil {
		t.Fatal(err)
	}
	if state.Pending || state.Error == "" {
		t.Fatal("逾時未解除等待並回報")
	}
	current := 1
	v.receiveStreamControl(p2p.Control{Type: "displays", Display: &current, DisplayCount: 2, DisplayRequest: 1}, 0)
	if v.displays.Current != 1 || v.displays.Error != "" {
		t.Fatal("晚到的有效確認無法恢復")
	}
}
