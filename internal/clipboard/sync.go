package clipboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"yourdesk/internal/p2p"
)

type keyJob struct {
	generation uint64
	paste      bool
	control    p2p.Control
}
type Sync struct {
	pull              *pullState
	remotePull        atomic.Bool
	pasteTransfer     *pasteTransferJob
	remoteDirectories atomic.Bool
	mu                sync.Mutex
	progressMu        sync.Mutex
	progressHandler   func([]Progress)
	progress          map[string]*transferProgress
	nativeMu          sync.Mutex
	transferGate      chan struct{}
	peer              *p2p.Peer
	ctx               context.Context
	clipCtx           context.Context
	generation        uint64
	pasteCancel       context.CancelCauseFunc
	held              map[string]p2p.Control
	remote            atomic.Bool
	legacyRemote      atomic.Bool
	started           time.Time
	observed          int64
	retryPending      bool
	retryAfter        time.Time
	packets           chan []byte
	sent              int64
	sentValid         bool
	keys              chan keyJob
	acks              map[string]chan error
}

func New() *Sync {
	return &Sync{pull: newPullState(), packets: make(chan []byte, 256), observed: nativeRevision(), progress: make(map[string]*transferProgress), transferGate: make(chan struct{}, 1), keys: make(chan keyJob, 4096), acks: make(map[string]chan error), held: make(map[string]p2p.Control)}
}

// Run 僅在工作階段驗證完成後啟動；剪貼簿通道失敗不關閉操作通道。
func (s *Sync) Run(ctx context.Context, peer *p2p.Peer) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	clipCtx, cancelClip := context.WithCancel(ctx)
	defer cancelClip()
	s.mu.Lock()
	s.peer = peer
	s.ctx = ctx
	s.clipCtx = clipCtx
	s.started = time.Now()
	s.mu.Unlock()
	// 以實際建立連線時的剪貼簿作為基準，握手期間的舊內容不主動覆寫對方。
	s.nativeMu.Lock()
	s.observed = nativeRevision()
	s.sentValid = false
	s.nativeMu.Unlock()
	var wg sync.WaitGroup
	for _, worker := range []func(context.Context){s.routePackets, s.receive, s.pullWorker, s.pullWorker, s.pullWorker, s.pullWorker} {
		wg.Add(1)
		go func(worker func(context.Context)) { defer wg.Done(); worker(clipCtx) }(worker)
	}
	for _, worker := range []func(context.Context){s.watch, s.reportProgress, s.keyboard} {
		wg.Add(1)
		go func(worker func(context.Context)) { defer wg.Done(); worker(ctx) }(worker)
	}
	select {
	case <-ctx.Done():
	case <-peer.Done():
	case <-peer.ClipboardDone():
		s.remote.Store(false)
		cancelClip()
		select {
		case <-ctx.Done():
		case <-peer.Done():
		}
	}
	cancel()
	wg.Wait()
	s.closePull()
}
func (s *Sync) watch(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	lastHello := time.Time{}
	lastWarning := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		s.reapPull()
		if time.Since(lastHello) > time.Second {
			if s.peer.ClipboardReady() && s.clipCtx.Err() == nil {
				helloCtx, cancel := context.WithTimeout(ctx, time.Second)
				_ = s.sendPacket(helloCtx, packet{Type: "hello", Version: 1, Directories: true, Pull: nativePullSupported()})
				cancel()
			}
			if !s.newChannelReady() {
				_ = s.peer.SendControl(p2p.Control{Type: "clipboard-capabilities"})
			}
			lastHello = time.Now()
		}
		var err error
		if s.newChannelReady() {
			err = s.transfer(s.clipCtx, false)
		} else if s.legacyRemote.Load() && time.Since(s.started) >= 2*time.Second {
			err = s.transferLegacy(ctx, false)
		}
		if err != nil && ctx.Err() == nil && time.Since(lastWarning) > 5*time.Second {
			lastWarning = time.Now()
			slog.Warn("剪貼簿同步未完成", "error", err)
		}
	}
}

// Poll(true) 只排入貼上屏障，不在 UI 執行緒讀圖、讀檔或等待網路。
func (s *Sync) Poll(force bool) {
	if !force {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case s.keys <- keyJob{paste: true, generation: s.generation}:
	default:
		slog.Warn("剪貼簿貼上佇列已滿")
	}
}

// CancelKeys 在失焦、暫停控制、切換螢幕時取消尚未送出的貼上與按鍵。
func (s *Sync) CancelKeys(reasons ...string) {
	reason := "輸入狀態重設"
	if len(reasons) > 0 {
		reason = reasons[0]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.held) == 0 && len(s.keys) == 0 && s.pasteCancel == nil {
		return
	}
	s.generation++
	if s.pasteCancel != nil {
		s.pasteCancel(errors.New(reason))
		s.pasteCancel = nil
	}
	if s.peer == nil {
		return
	}
	for _, c := range s.held {
		if c.RawKey != nil {
			copy := *c.RawKey
			copy.Down = false
			copy.Repeat = false
			c.RawKey = &copy
		} else {
			c.Down = false
		}
		_ = s.peer.SendControl(c)
	}
	clear(s.held)
	_ = s.peer.SendControl(p2p.Control{Type: "raw-key-reset"})
}
func (s *Sync) SendControl(c p2p.Control) error {
	if c.Type == "raw-key-reset" {
		s.CancelKeys("原始鍵盤狀態重設")
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.peer != nil {
			return s.peer.SendControl(c)
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peer == nil {
		return fmt.Errorf("操作連線尚未就緒")
	}
	if c.Type != "key" && c.Type != "raw-key" {
		return s.peer.SendControl(c)
	}
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	select {
	case s.keys <- keyJob{control: c, generation: s.generation}:
		return nil
	default:
		return fmt.Errorf("鍵盤佇列已滿")
	}
}
func (s *Sync) keyboard(ctx context.Context) {
	dropPaste := false
	var generation uint64
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.keys:
			s.mu.Lock()
			if job.generation != s.generation {
				s.mu.Unlock()
				continue
			}
			if generation != job.generation {
				dropPaste = false
				generation = job.generation
			}
			if job.paste {

				timeoutCtx, timeoutCancel := context.WithTimeout(ctx, TransferTimeout)
				pasteCtx, cancel := context.WithCancelCause(timeoutCtx)
				s.pasteCancel = cancel
				s.mu.Unlock()
				err := s.waitPasteTransfer(pasteCtx)
				cause := context.Cause(pasteCtx)
				cancel(nil)
				timeoutCancel()
				dropPaste = err != nil
				s.mu.Lock()
				if job.generation == s.generation {
					s.pasteCancel = nil
				}
				s.mu.Unlock()
				if err != nil && ctx.Err() == nil {
					if errors.Is(err, context.Canceled) {
						slog.Info("貼上按鍵已取消，背景資料傳輸不受影響", "原因", cause)
					} else {
						slog.Warn("貼上未執行，剪貼簿尚未就緒", "error", err)
					}
				}
				continue
			}
			c := job.control
			down := c.Down
			if c.RawKey != nil {
				down = c.RawKey.Down
			}
			if dropPaste && isPasteKey(c) {
				if !down {
					dropPaste = false
				}
				s.mu.Unlock()
				continue
			}
			if err := s.peer.SendControl(c); err == nil {
				id := c.Key
				if c.RawKey != nil {
					id = fmt.Sprintf("%s/%d", c.RawKey.Platform, c.RawKey.Code)
				}
				if down {
					s.held[id] = c
				} else {
					delete(s.held, id)
				}
			} else if ctx.Err() == nil {
				slog.Debug("鍵盤傳送失敗", "error", err)
			}
			s.mu.Unlock()
		}
	}
}
