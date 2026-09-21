package main

import (
	"context"
	"sync/atomic"
	"time"
)

var secureAttentionPending atomic.Bool

// 選單及虛擬鍵盤共用同一條 SAS 指令；不把指令錯誤誤報為連線失敗。
func (g *game) sendSecureAttention() {
	notifyError := func(message string) { nativeShowInputNotice(message); emitUIEvent("input-error", message) }
	if g.peer == nil || !g.peer.Connected() || !g.controlEnabled || !g.displayInputReady() {
		notifyError("遠端目前無法接受輸入。")
		return
	}
	if !g.peer.SupportsCommand("input.secure-attention") {
		notifyError("遠端未提供 Ctrl+Alt+Del。若已更新 Windows App，請在遠端停用再啟用「未登入開機服務」，更新服務副本後重新連線。")
		return
	}
	if !secureAttentionPending.CompareAndSwap(false, true) {
		return
	}
	nativeShowInputNotice("正在傳送 Ctrl+Alt+Del…")
	peer := g.peer
	go func() {
		defer secureAttentionPending.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := peer.CallCommand(ctx, "input.secure-attention")
		if err != nil {
			notifyError("Ctrl+Alt+Del 傳送未確認：" + err.Error())
			return
		}
		nativeShowInputNotice("Ctrl+Alt+Del 已交給遠端 Windows 服務。")
	}()
}
