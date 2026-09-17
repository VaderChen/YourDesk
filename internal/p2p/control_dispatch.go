package p2p

import (
	"log/slog"
	"yourdesk/internal/authlog"
)

// 控制事件仍依接收順序執行，確保舊版文字剪貼簿寫入完成後才貼上。
// 命令回覆、ping 與剪貼簿額度由接收回呼先處理，不等原生剪貼簿或輸入 API。
func (p *Peer) dispatchControl(c Control, handler func(Control)) {
	if handler == nil {
		return
	}
	p.controlDispatchOnce.Do(func() {
		p.controlInbox = make(chan Control, 128)
		go runControlWorker(p.Done(), p.controlInbox, handler, func() {
			slog.Error("控制事件處理超過 30 秒，結束停滯連線")
			authlog.Event("control-worker-stalled", nil)
			authlog.Stacks()
			p.markClosed()
			go p.pc.Close()
		})
	})
	select {
	case <-p.Done():
		return
	default:
	}
	// 滑鼠移動可略過，預留半數槽位給不可遺失的剪貼簿、按鍵與按鈕。
	if c.Type == "move" && len(p.controlInbox) >= cap(p.controlInbox)/2 {
		return
	}
	select {
	case p.controlInbox <- c:
	default:
		// 不默默遺失按鍵釋放或執行順序；接收回呼不等待失效程序清理。
		p.controlOverflowOnce.Do(func() {
			slog.Error("控制事件工作者佇列已滿，結束連線以釋放輸入狀態")
			p.markClosed()
			go p.pc.Close()
		})
	}
}
