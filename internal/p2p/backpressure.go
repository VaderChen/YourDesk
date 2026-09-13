package p2p

import (
	"yourdesk/internal/authlog"
)

const controlHighWater = 4 * 1024
const controlLowWater = 1024

// controlPressure 使用不同的進出門檻，避免在門檻附近反覆切換。
func controlPressure(congested bool, buffered uint64) bool {
	if congested {
		return buffered > controlLowWater
	}
	return buffered >= controlHighWater
}

// controlBackpressure 停止擴大共用 SCTP association 的負載，讓控制資料排空。
// 不重設可靠通道，也不假裝能刪除已交給 SCTP 的訊息。
func (p *Peer) controlBackpressure() bool {
	p.mu.RLock()
	dc := p.control
	p.mu.RUnlock()
	if dc == nil {
		return false
	}
	buffered := dc.BufferedAmount()
	for {
		before := p.controlCongested.Load()
		after := controlPressure(before, buffered)
		if before == after {
			return after
		}
		if p.controlCongested.CompareAndSwap(before, after) {
			authlog.Event("control-backpressure", map[string]any{"active": after, "buffered": buffered})
			return after
		}
	}
}
