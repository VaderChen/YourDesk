package clipboard

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"yourdesk/internal/p2p"
)

const MaxLegacyTextBytes = 32 * 1024

func (s *Sync) newChannelReady() bool {
	return s.remote.Load() && s.clipCtx != nil && s.clipCtx.Err() == nil && s.peer.ClipboardReady()
}

// 舊協定沒有接收 ack；文字寫入與後續按鍵同在有序 control 回呼內完成。
func (s *Sync) Handle(c p2p.Control) bool {
	if c.Type != "clipboard-capabilities" && c.Type != "clipboard-text" {
		return false
	}
	if c.Type == "clipboard-capabilities" {
		s.legacyRemote.Store(true)
		return true
	}
	if s.remote.Load() || !s.legacyRemote.Load() || len(c.Clipboard) > MaxLegacyTextBytes || !validText(c.Clipboard) {
		return true
	}
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	if err := writeContent(content{Kind: "text", Data: c.Clipboard}); err != nil {
		slog.Warn("舊版剪貼簿文字寫入失敗", "error", err)
		return true
	}
	s.observed = nativeRevision()
	s.sent = s.observed
	s.sentValid = true
	return true
}
func (s *Sync) transferForPaste(ctx context.Context) error {
	// 給新通道能力協商短暫時間，避免連線剛建立就誤用舊協定。
	for !s.newChannelReady() && time.Since(s.started) < 2*time.Second {
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if s.newChannelReady() {
		return s.transfer(ctx, true)
	}
	if s.legacyRemote.Load() {
		err := s.transferLegacy(ctx, true)
		if err == nil && s.newChannelReady() {
			return s.transfer(ctx, true)
		}
		return err
	}
	return nil
}
func (s *Sync) transferLegacy(ctx context.Context, force bool) error {
	select {
	case s.transferGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.transferGate }()
	if s.newChannelReady() {
		return nil
	}
	s.nativeMu.Lock()
	revision := nativeRevision()
	if !force && (s.observed == revision || (s.sentValid && s.sent == revision)) {
		s.nativeMu.Unlock()
		return nil
	}
	s.nativeMu.Unlock()
	value, readRevision, err := readStableContent(readLegacyText)
	s.nativeMu.Lock()
	defer s.nativeMu.Unlock()
	revision = readRevision
	if err != nil {
		return err
	}
	if revision != nativeRevision() {
		return errors.New("剪貼簿讀取期間已變更")
	}
	if s.sentValid && s.sent == revision {
		return nil
	}
	s.observed = revision
	if value.Kind != "text" {
		if force && value.Kind != "" {
			return errors.New("舊版備援只支援文字剪貼簿")
		}
		return nil
	}
	if len(value.Data) > MaxLegacyTextBytes || !validText(value.Data) {
		return errors.New("舊版文字剪貼簿上限為 32 KiB")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = s.peer.SendControl(p2p.Control{Type: "clipboard-text", Clipboard: value.Data}); err != nil {
		return err
	}
	s.sent = revision
	s.sentValid = true
	return nil
}
