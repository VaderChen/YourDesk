package p2p

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func clipboardTestPeers(t *testing.T, handle func(Control)) (*Peer, *Peer) {
	t.Helper()
	t.Setenv("YOURDESK_AUTH_LOG_DIR", t.TempDir())
	makePeer := func() *Peer {
		pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			t.Fatal(err)
		}
		p := &Peer{pc: pc, done: make(chan struct{}), clipboardInbox: make(chan []byte, 128), clipboardDone: make(chan struct{})}
		t.Cleanup(func() { _ = p.Close() })
		return p
	}
	a, b := makePeer(), makePeer()
	bind := func(p *Peer, dc *webrtc.DataChannel) {
		if dc.Label() == ClipboardChannel {
			p.bindClipboard(dc)
			return
		}
		p.mu.Lock()
		p.control = dc
		p.mu.Unlock()
		p.onMessage(dc, func(m webrtc.DataChannelMessage) {
			var c Control
			if decodeControl(m.Data, &c) == nil && !p.handleCommand(c) {
				p.dispatchControl(c, handle)
			}
		})
		dc.OnOpen(func() { p.announceCommands() })
	}
	b.pc.OnDataChannel(func(dc *webrtc.DataChannel) { bind(b, dc) })
	for _, name := range []string{ControlChannel, ClipboardChannel} {
		dc, err := a.pc.CreateDataChannel(name, nil)
		if err != nil {
			t.Fatal(err)
		}
		bind(a, dc)
	}
	offer, err := a.pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(a.pc)
	if err = a.pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gathered:
	case <-time.After(10 * time.Second):
		t.Fatal("offer timeout")
	}
	if err = b.pc.SetRemoteDescription(*a.pc.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	answer, err := b.pc.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered = webrtc.GatheringCompletePromise(b.pc)
	if err = b.pc.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gathered:
	case <-time.After(10 * time.Second):
		t.Fatal("answer timeout")
	}
	if err = a.pc.SetRemoteDescription(*b.pc.LocalDescription()); err != nil {
		t.Fatal(err)
	}
	clipboardEventually(t, func() bool {
		a.clipboardFlow.mu.Lock()
		aw := a.clipboardFlow.window
		a.clipboardFlow.mu.Unlock()
		b.clipboardFlow.mu.Lock()
		bw := b.clipboardFlow.window
		b.clipboardFlow.mu.Unlock()
		return aw == clipboardReceiveWindow && bw == clipboardReceiveWindow && a.ClipboardReady() && b.ClipboardReady()
	})
	return a, b
}
func clipboardEventually(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("等待剪貼簿狀態逾時")
		}
		time.Sleep(time.Millisecond)
	}
}

// 暫停接收消費直到額度用完，證明傳送等待且 ping／優先回覆仍可通過。
// 恢復後兩方向慢速接收，逐筆比對所有內容與順序，不碰系統剪貼簿。
func TestClipboardCreditWebRTCSmoke(t *testing.T) {
	a, b := clipboardTestPeers(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	const count = 160
	payload := func(i int) []byte {
		data := bytes.Repeat([]byte{byte(i)}, MaxClipboardMessage)
		data[0] = 1
		binary.BigEndian.PutUint32(data[1:5], uint32(i))
		return data
	}
	send := func(p *Peer) <-chan error {
		done := make(chan error, 1)
		go func() {
			for i := 0; i < count; i++ {
				if err := p.SendClipboard(ctx, payload(i)); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}()
		return done
	}
	sentA := send(a)
	clipboardEventually(t, func() bool { return len(b.clipboardInbox) == clipboardReceiveWindow })
	select {
	case err := <-sentA:
		t.Fatalf("接收未消費時傳送意外結束：%v", err)
	default:
	}
	pingCtx, pingCancel := context.WithTimeout(ctx, time.Second)
	_, err := a.CallCommand(pingCtx, "ping")
	pingCancel()
	if err != nil {
		t.Fatalf("滿載影響心跳：%v", err)
	}
	// 普通傳送正在等待額度，pull 回覆仍可使用獨立送出資格及額度。
	priority := append([]byte{2}, bytes.Repeat([]byte{43}, 32)...)
	priorityCtx, priorityCancel := context.WithTimeout(ctx, time.Second)
	err = a.SendClipboard(priorityCtx, priority)
	priorityCancel()
	if err != nil {
		t.Fatalf("優先回覆被普通資料阻塞：%v", err)
	}
	clipboardEventually(t, func() bool { return len(b.clipboardInbox) == clipboardReceiveWindow+1 })
	if !a.ClipboardReady() || !b.ClipboardReady() {
		t.Fatal("慢速接收導致剪貼簿斷線")
	}
	// 等待同一條送出 lane 的呼叫也能取消，不會重送或插入不完整資料。
	cancelCtx, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	err = a.SendClipboard(cancelCtx, payload(999))
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("無法取消等待：%v", err)
	}
	receive := func(p *Peer, wantPriority bool) <-chan error {
		done := make(chan error, 1)
		go func() {
			got, seenPriority := 0, false
			for got < count || (wantPriority && !seenPriority) {
				select {
				case <-ctx.Done():
					done <- ctx.Err()
					return
				case data := <-p.ClipboardMessages():
					if data[0] == 2 {
						if !wantPriority || seenPriority || !bytes.Equal(data, priority) {
							done <- fmt.Errorf("pull 回覆錯誤")
							return
						}
						seenPriority = true
					} else {
						if !bytes.Equal(data, payload(got)) {
							done <- fmt.Errorf("第 %d 筆內容或順序錯誤", got)
							return
						}
						got++
					}
					time.Sleep(time.Millisecond)
					p.ClipboardConsumed(data)
				}
			}
			done <- nil
		}()
		return done
	}
	receivedA, receivedB := receive(a, false), receive(b, true)
	sentB := send(b)
	for _, done := range []<-chan error{sentA, sentB, receivedA, receivedB} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if !a.Connected() || !b.Connected() {
		t.Fatal("完成後連線未保持")
	}
}

func TestClipboardCreditsLegacyAndCumulative(t *testing.T) {
	p := &Peer{done: make(chan struct{}), clipboardDone: make(chan struct{})}
	defer p.markClosed()
	// 舊端沒有欄位時不等待永遠不會出現的額度。
	for i := 0; i < 100; i++ {
		if !p.reserveClipboardSlot(0) {
			t.Fatal("舊端遭流控阻塞")
		}
	}
	p.setClipboardWindow(clipboardReceiveWindow)
	p.acceptClipboardCredit(0, 101)
	if p.reserveClipboardSlot(0) {
		t.Fatal("超額確認增加額度")
	}
	p.acceptClipboardCredit(0, 100)
	p.acceptClipboardCredit(0, 90)
	for i := 0; i < clipboardReceiveWindow; i++ {
		if !p.reserveClipboardSlot(0) {
			t.Fatal("確認後無額度")
		}
	}
	p.acceptClipboardCredit(0, 100)
	if p.reserveClipboardSlot(0) {
		t.Fatal("重複確認增加額度")
	}
	if !p.reserveClipboardSlot(1) {
		t.Fatal("兩類資料共用了額度")
	}
}

func TestSlowLegacyClipboardKeepsControlResponsive(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	events := make(chan string, 2)
	a, _ := clipboardTestPeers(t, func(c Control) {
		if c.Type == "clipboard-text" {
			close(entered)
			<-release
		}
		events <- c.Type
	})
	// 即使測試中止，先釋放模擬原生寫入，再清理 Peer。
	var released bool
	defer func() {
		if !released {
			close(release)
		}
	}()
	if err := a.SendControl(Control{Type: "clipboard-text", Clipboard: []byte("text")}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("未進入文字寫入")
	}
	if err := a.SendControl(Control{Type: "key", Key: "v", Down: true}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := a.CallCommand(ctx, "ping"); err != nil {
		t.Fatalf("文字寫入阻塞控制接收：%v", err)
	}
	select {
	case <-events:
		t.Fatal("文字寫入未完成就執行後續按鍵")
	default:
	}
	close(release)
	released = true
	for _, want := range []string{"clipboard-text", "key"} {
		select {
		case got := <-events:
			if got != want {
				t.Fatalf("got %s want %s", got, want)
			}
		case <-ctx.Done():
			t.Fatal("控制工作未恢復")
		}
	}
}
