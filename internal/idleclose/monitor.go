// Package idleclose 只以使用者輸入計算遠端桌面的閒置時間。
package idleclose

import (
	"sync"
	"time"
)

const Timeout = 15 * time.Minute

// PointerRadius 是遠端畫面像素；錨點只在有效移動後更新，保留慢速移動的累積量。
const PointerRadius = 4.0

type Monitor struct {
	mu         sync.Mutex
	active     bool
	last       time.Time
	hasPointer bool
	x, y       float64
}

// x、y 使用遠端画面像素；同座標重送與小範圍抖動不算操作。
func (m *Monitor) Input(now time.Time, kind string, x, y float64, down bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	activity := false
	switch kind {
	case "move":
		dx, dy := x-m.x, y-m.y
		activity = m.hasPointer && dx*dx+dy*dy > PointerRadius*PointerRadius
		if !m.hasPointer || activity {
			m.x, m.y, m.hasPointer = x, y, true
		}
	case "key", "raw-key", "button":
		activity = down
	case "wheel":
		activity = true
	}
	if activity && m.active {
		m.last = now
	}
}

// 開啟設定或建立連線時重新起算；預設關閉不會累計先前閒置。
func (m *Monitor) Expired(now time.Time, enabled, connected bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !enabled || !connected {
		m.active = false
		return false
	}
	if !m.active {
		m.active, m.last = true, now
		return false
	}
	return now.Sub(m.last) >= Timeout
}
