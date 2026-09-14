package p2p

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// 普通資料與 pull／ack 各有 32 筆額度，共 64 × 20 KiB，低於 128 槽接收佇列。
// 檔案不可用丟棄資料的方式降載，額度不足時只讓送出工作者等待。
const clipboardReceiveWindow = 32

type clipboardFlowState struct {
	mu          sync.Mutex
	window      uint64
	sent, acked [2]uint64
	processed   [2]atomic.Uint64
	once        sync.Once
	changed     chan struct{}
}

func (p *Peer) setClipboardWindow(window uint32) {
	if window == 0 || window > clipboardReceiveWindow {
		return
	}
	f := &p.clipboardFlow
	f.mu.Lock()
	f.window = uint64(window)
	f.mu.Unlock()
	f.once.Do(func() {
		f.mu.Lock()
		f.changed = make(chan struct{}, 1)
		f.mu.Unlock()
		go p.sendClipboardCredits()
	})
	// 包含能力公告前已處理的 hello；重複公告不重設累計值。
	p.notifyClipboardCredit()
}

func (p *Peer) reserveClipboardSlot(lane int) bool {
	f := &p.clipboardFlow
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.window != 0 && f.sent[lane]-f.acked[lane] >= f.window {
		return false
	}
	f.sent[lane]++
	return true
}

func (p *Peer) rollbackClipboardSlot(lane int) {
	f := &p.clipboardFlow
	f.mu.Lock()
	f.sent[lane]--
	f.mu.Unlock()
}

func (p *Peer) acceptClipboardCredit(lane int, consumed uint64) {
	f := &p.clipboardFlow
	f.mu.Lock()
	defer f.mu.Unlock()
	// 累計確認可合併、可重送；過期或超過實際送出量的數值不增加額度。
	if consumed > f.acked[lane] && consumed <= f.sent[lane] {
		f.acked[lane] = consumed
	}
}

// ClipboardConsumed 由應用層在處理完一筆 ClipboardMessages 資料後呼叫一次。
// 不代表整份檔案成功，原有大小、摘要與傳輸完成 ack 仍需通過。
func (p *Peer) ClipboardConsumed(data []byte) {
	p.clipboardFlow.processed[clipboardLane(data)].Add(1)
	p.notifyClipboardCredit()
}

func (p *Peer) notifyClipboardCredit() {
	f := &p.clipboardFlow
	// once.Do 與 ClipboardConsumed 可並行，channel 以 mutex 保護初始化讀取。
	f.mu.Lock()
	changed := f.changed
	f.mu.Unlock()
	if changed != nil {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
}

func (p *Peer) sendClipboardCredits() {
	f := &p.clipboardFlow
	var last [2]uint64
	for {
		select {
		case <-p.Done():
			return
		case <-p.ClipboardDone():
			return
		case <-f.changed:
		}
		// 合併短時間內完成的多筆資料，不為每個檔案區塊產生一個確認封包。
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-p.Done():
			timer.Stop()
			return
		case <-p.ClipboardDone():
			timer.Stop()
			return
		case <-timer.C:
		}
		consumed := [2]uint64{f.processed[0].Load(), f.processed[1].Load()}
		if consumed != last {
			if p.SendControl(Control{Type: "clipboard-credit", ClipboardConsumed: consumed[0], ClipboardPriorityConsumed: consumed[1]}) == nil {
				last = consumed
			} else {
				p.notifyClipboardCredit()
			}
		}
	}
}

// pull 回覆、讀取請求與完成 ack 使用獨立額度，避免原生剪貼簿發布
// 正等待遠端內容時，所有額度都被尚未處理的普通傳輸資料佔滿。
func clipboardLane(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	if data[0] == 2 {
		return 1
	}
	if data[0] == 0 {
		var header struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data[1:], &header) == nil {
			switch header.Type {
			case "ack", "pull-read", "pull-error":
				return 1
			}
		}
	}
	return 0
}
