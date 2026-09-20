package main

import (
	"context"
	"sync/atomic"
	"time"
	"yourdesk/internal/p2p"
)

const frameRecoveryInterval = 2 * time.Second

// 低位表示仍需恢復，其餘位元是丟幀世代。舊的已排隊 keyframe 不可
// 清除較晚發生的丟幀；網路回呼只更新原子狀態及單槽通知，完全不等命令。
type frameRecovery struct {
	state atomic.Uint64
	wake  chan struct{}
}

func newFrameRecovery() *frameRecovery { return &frameRecovery{wake: make(chan struct{}, 1)} }
func (r *frameRecovery) epoch() uint64 { return r.state.Load() &^ 1 }
func (r *frameRecovery) pending() bool { return r.state.Load()&1 != 0 }
func (r *frameRecovery) notify() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}
func (r *frameRecovery) dropped() {
	for {
		before := r.state.Load()
		if r.state.CompareAndSwap(before, (before+2)|1) {
			break
		}
	}
	r.notify()
}
func (r *frameRecovery) request() { r.state.Or(1); r.notify() }
func (r *frameRecovery) complete(epoch uint64) bool {
	for {
		before := r.state.Load()
		if before&^1 != epoch {
			return false
		}
		if r.state.CompareAndSwap(before, before&^1) {
			return true
		}
	}
}

// 每連線僅一個 worker。命令丟失、對端尚未公告能力或回覆成功但補幀又
// 遺失時都會重試，直到完整影格真正套用；不是收到命令 ack 就視為復原。
func (r *frameRecovery) run(ctx context.Context, request func(context.Context)) {
	ticker := time.NewTicker(frameRecoveryInterval)
	defer ticker.Stop()
	var next time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-ticker.C:
		}
		if ctx.Err() != nil || !r.pending() || time.Now().Before(next) {
			continue
		}
		next = time.Now().Add(frameRecoveryInterval)
		attempt, cancel := context.WithTimeout(ctx, frameRecoveryInterval)
		request(attempt)
		cancel()
	}
}

type receivedFrame struct {
	p2p.Frame
	recoveryEpoch uint64
}

func (r *frameRecovery) submit(f p2p.Frame, submit func(receivedFrame) bool) bool {
	if submit(receivedFrame{Frame: f, recoveryEpoch: r.epoch()}) {
		return true
	}
	r.dropped()
	return false
}

// 只由解碼 worker 使用；JPEG 每個 patch 雖可獨立解碼，合成基底仍相依。
type jpegContinuity struct {
	seen, valid bool
	last        p2p.Frame
}

func (s *jpegContinuity) accept(f p2p.Frame, recovering bool) (allow, gap bool) {
	same := s.seen && f.ViewID == s.last.ViewID && f.Display == s.last.Display &&
		f.Width == s.last.Width && f.Height == s.last.Height
	gap = same && f.Sequence != s.last.Sequence+1
	if !same || gap || recovering {
		s.valid = false
	}
	s.seen = true
	s.last = f
	s.last.JPEG = nil // 只記錄標頭，不延長編碼資料生命週期。
	return f.Keyframe || s.valid, gap
}
