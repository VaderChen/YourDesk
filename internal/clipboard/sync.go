package clipboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"yourdesk/internal/eventqueue"
	"yourdesk/internal/p2p"
)

type keyJob struct {
	generation uint64
	copyIntent bool
	paste      bool
	reset      bool
	batch      []p2p.Control
	result     chan error
	context    context.Context
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
	inputInFlight     bool
	inputEpoch        chan struct{}
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
	keys              *eventqueue.Queue[keyJob]
	acks              map[string]chan error
}

func New() *Sync {
	return &Sync{pull: newPullState(), packets: make(chan []byte, 256), observed: nativeRevision(), progress: make(map[string]*transferProgress), transferGate: make(chan struct{}, 1), keys: newInputQueue(), acks: make(map[string]chan error), held: make(map[string]p2p.Control)}
}

func newInputQueue() *eventqueue.Queue[keyJob] {
	return eventqueue.New(4096, func(previous, next keyJob) bool {
		return !previous.paste && !next.paste && !previous.reset && !next.reset && previous.generation == next.generation && p2p.CanCoalesceMoves(previous.control, next.control)
	})
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
	if !s.keys.Push(keyJob{paste: true, generation: s.generation}, 0) {
		slog.Warn("剪貼簿貼上佇列已滿")
	}
}

// CancelKeys 在失焦、暫停控制、切換螢幕時取消尚未送出的貼上與輸入。
func (s *Sync) CancelKeys(reasons ...string) {
	reason := "輸入狀態重設"
	if len(reasons) > 0 {
		reason = reasons[0]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetKeysLocked(reason, false)
}

// ResetKeys 明確要求遠端重設，即使目前沒有本機持鍵狀態也送出屏障。
func (s *Sync) ResetKeys(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetKeysLocked(reason, true)
}

// 只排入有序屏障，不在 UI 執行緒等待傳輸。傳送中的按下事件返回後，
// 工作者仍記錄它，再由屏障釋放，避免失焦恰逢傳送時留下卡鍵。
func (s *Sync) resetKeysLocked(reason string, force bool) {
	if !force && len(s.held) == 0 && s.keys.Len() == 0 && s.pasteCancel == nil && !s.inputInFlight {
		return
	}
	s.generation++
	if s.inputEpoch != nil {
		close(s.inputEpoch)
		s.inputEpoch = nil
	}
	if s.pasteCancel != nil {
		s.pasteCancel(errors.New(reason))
		s.pasteCancel = nil
	}
	s.keys.Reset(keyJob{generation: s.generation, reset: true})
}
func (s *Sync) SendControl(c p2p.Control) error { return s.enqueueControl(c, false) }

// SendCopyControl 在送出遠端複製前記下本機版本，快速接續的貼上不以本機舊資料覆蓋遠端。
func (s *Sync) SendCopyControl(c p2p.Control) error { return s.enqueueControl(c, true) }
func (s *Sync) enqueueControl(c p2p.Control, copyIntent bool) error {
	s.mu.Lock()
	if s.peer == nil {
		s.mu.Unlock()
		return fmt.Errorf("操作連線尚未就緒")
	}
	if c.Type != "key" && c.Type != "raw-key" && c.Type != "button" && c.Type != "move" && c.Type != "wheel" && c.Type != "raw-key-reset" {
		peer := s.peer
		s.mu.Unlock()
		return peer.SendControl(c)
	}
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	if c.Type == "raw-key-reset" {
		s.resetKeysLocked("原始鍵盤狀態重設", true)
		return nil
	}
	// 滑鼠與鍵盤共用順序；只更新尾端連續移動，保留可靠事件空間。
	limit := 0
	if c.Type == "move" {
		limit = 64
	}
	if !s.keys.Push(keyJob{control: c, copyIntent: copyIntent, generation: s.generation}, limit) && c.Type != "move" {
		return fmt.Errorf("輸入佇列已滿")
	}
	return nil
}
func (s *Sync) keyboard(ctx context.Context) {
	s.keyboardWithSender(ctx, s.peer.SendControl)
}

func (s *Sync) keyboardWithSender(ctx context.Context, send func(p2p.Control) error) {
	dropPaste := false
	var copied bool
	var copyRevision int64
	var generation uint64
	for {
		job, ok := s.keys.Pop(ctx.Done())
		if !ok {
			return
		}
		s.mu.Lock()
		if job.generation != s.generation {
			s.mu.Unlock()
			continue
		}
		if generation != job.generation {
			dropPaste = false
			generation = job.generation
			copied = false
		}
		if job.batch != nil {
			s.inputInFlight = true
			s.mu.Unlock()
			err := s.sendInputBatch(ctx, job, send)
			s.mu.Lock()
			job.result <- err
			if err != nil {
				s.resetKeysLocked("批次輸入取消或失敗", true)
			}
			s.inputInFlight = false
			s.mu.Unlock()
			continue
		}
		if job.reset {
			releases := make([]p2p.Control, 0, len(s.held)+1)
			for _, c := range s.held {
				if c.RawKey != nil {
					copy := *c.RawKey
					copy.Down, copy.Repeat, copy.Modifiers = false, false, 0
					c.RawKey = &copy
				} else {
					c.Down = false
				}
				releases = append(releases, c)
			}
			clear(s.held)
			s.inputInFlight = true
			s.mu.Unlock()
			for _, c := range append(releases, p2p.Control{Type: "raw-key-reset"}) {
				_ = send(c)
			}
			s.mu.Lock()
			s.inputInFlight = false
			s.mu.Unlock()
			continue
		}
		if job.paste {

			timeoutCtx, timeoutCancel := context.WithTimeout(ctx, TransferTimeout)
			pasteCtx, cancel := context.WithCancelCause(timeoutCtx)
			s.pasteCancel = cancel
			s.mu.Unlock()
			var err error
			if !copied || nativeRevision() != copyRevision {
				err = s.waitPasteTransfer(pasteCtx)
			}

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
		s.inputInFlight = true
		s.mu.Unlock()
		if job.copyIntent {
			copyRevision = nativeRevision()
		}
		err := send(c)
		if job.copyIntent {
			copied = err == nil
		}
		s.mu.Lock()
		s.inputInFlight = false
		if err == nil {
			s.rememberControlLocked(c)
		} else if ctx.Err() == nil {
			slog.Debug("鍵盤傳送失敗", "error", err)
		}
		s.mu.Unlock()
	}
}

func (s *Sync) rememberControlLocked(c p2p.Control) {
	id, down := c.Key, c.Down
	if c.RawKey != nil {
		id = fmt.Sprintf("%s/%d", c.RawKey.Platform, c.RawKey.Code)
		down = c.RawKey.Down
	}
	if c.Type == "button" {
		id = fmt.Sprintf("mouse/%d", c.Button)
	}
	if c.Type == "key" || c.Type == "raw-key" || c.Type == "button" {
		if down {
			if s.held == nil {
				s.held = make(map[string]p2p.Control)
			}
			s.held[id] = c
		} else {
			delete(s.held, id)
		}
	}
}

func (s *Sync) sendInputBatch(ctx context.Context, job keyJob, send func(p2p.Control) error) error {
	for _, c := range job.batch {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := job.context.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		current := job.generation == s.generation
		s.mu.Unlock()
		if !current {
			return context.Canceled
		}
		if err := send(c); err != nil {
			return err
		}
		s.mu.Lock()
		s.rememberControlLocked(c)
		s.mu.Unlock()
	}
	return nil
}
