//go:build darwin && cgo

package clipboard

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"yourdesk/internal/p2p"
	"yourdesk/internal/signaling"
)

// 明確啟用才會使用系統剪貼簿與 Finder WebDAV 掛載。
// 同機共用 pasteboard，手動觸發傳輸而非啟動兩個互相覆寫的 watch。
func TestClipboardLoopbackSmoke(t *testing.T) {
	if os.Getenv("YOURDESK_CLIPBOARD_SMOKE") != "1" {
		t.Skip("set YOURDESK_CLIPBOARD_SMOKE=1")
	}
	// 保存原內容；測試結束前若使用者另有複製，就不覆寫。
	original, originalErr := readContent()
	if original.Kind == "files" {
		for _, path := range original.Paths {
			if isPullPath(path) {
				original = content{}
				break
			}
		}
	}
	var finalRevision int64
	defer func() {
		if finalRevision != 0 && nativeRevision() == finalRevision {
			if originalErr == nil && original.Kind != "" && original.Kind != "unsupported" {
				_ = writeContent(original)
			} else {
				_ = writeContent(content{Kind: "text", Data: []byte{}})
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	secret := bytes.Repeat([]byte{37}, 32)
	hostChan := make(chan *p2p.Peer, 1)
	errChan := make(chan error, 2)
	go func() {
		errChan <- signaling.ListenDirect(ctx, addr, secret, func(c context.Context, s *signaling.Client) {
			peer, e := p2p.NewHost(c, s, nil)
			if e != nil {
				errChan <- e
				return
			}
			hostChan <- peer
			<-ctx.Done()
		})
	}()
	var signal *signaling.Client
	for i := 0; i < 40; i++ {
		signal, err = signaling.DialDirect(ctx, addr, secret)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := p2p.NewViewer(ctx, signal, func(p2p.Frame) {})
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	var host *p2p.Peer
	select {
	case host = <-hostChan:
	case err := <-errChan:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	defer host.Close()
	for !host.ClipboardReady() || !viewer.ClipboardReady() {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	a, b := New(), New()
	var workers sync.WaitGroup
	for s, p := range map[*Sync]*p2p.Peer{a: host, b: viewer} {
		s.peer = p
		s.ctx = ctx
		s.clipCtx = ctx
		s.started = time.Now()
		for _, fn := range []func(context.Context){s.routePackets, s.receive, s.pullWorker, s.pullWorker, s.pullWorker, s.pullWorker} {
			workers.Add(1)
			go func(fn func(context.Context)) { defer workers.Done(); fn(ctx) }(fn)
		}
	}
	defer func() { cancel(); workers.Wait(); a.closePull(); b.closePull() }()
	for _, s := range []*Sync{a, b} {
		if err := s.sendPacket(ctx, packet{Type: "hello", Version: 1, Directories: true, Pull: true}); err != nil {
			t.Fatal(err)
		}
	}
	for !a.newChannelReady() || !b.newChannelReady() {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Log("本機 P2P 剪貼簿通道與雙向協商成功")
	for i, s := range []*Sync{a, b} {
		text := []byte("YourDesk 本機雙向文字 smoke")
		if err := writeContent(content{Kind: "text", Data: text}); err != nil {
			t.Fatal(err)
		}
		s.sentValid = false
		if err := s.transfer(ctx, true); err != nil {
			t.Fatalf("方向 %d 文字: %v", i, err)
		}
		got, err := readContent()
		if err != nil || got.Kind != "text" || !bytes.Equal(got.Data, text) {
			t.Fatalf("文字比對失敗: %v", err)
		}
		t.Logf("方向 %d 文字接收與 native pasteboard 比對成功", i)
		dir := t.TempDir()
		data := bytes.Repeat([]byte("YourDesk-file-smoke\n"), 140000)
		source := filepath.Join(dir, "smoke.txt")
		if err := os.WriteFile(source, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := writeContent(content{Kind: "files", Paths: []string{source}}); err != nil {
			t.Fatal(err)
		}
		s.sentValid = false
		if err := s.transfer(ctx, true); err != nil {
			t.Fatalf("方向 %d 檔案: %v", i, err)
		}
		got, err = readContent()
		if err != nil || got.Kind != "files" || len(got.Paths) != 1 || got.Paths[0] == source {
			t.Fatalf("未公布遠端檔案路徑: kind=%s err=%v", got.Kind, err)
		}
		received, err := os.ReadFile(got.Paths[0])
		if err != nil {
			t.Fatalf("讀取公布的檔案: %v", err)
		}
		if !bytes.Equal(received, data) {
			t.Fatalf("檔案內容不一致: got=%d want=%d", len(received), len(data))
		}
		t.Logf("方向 %d 檔案實際傳輸並比對成功，%d bytes", i, len(received))
		finalRevision = nativeRevision()
	}
}
