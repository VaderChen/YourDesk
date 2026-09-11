package clipboard

import (
	"context"
	"fmt"
	"yourdesk/internal/p2p"
)

// PasteAgentText 沿用既有剪貼簿傳输並等待完成；會取代本機與遠端剪貼簿文字。
func (s *Sync) PasteAgentText(ctx context.Context, text string) error {
	if len(text) == 0 || len(text) > 16384 || !validText([]byte(text)) {
		return fmt.Errorf("文字必須是有效 UTF-8，且不超過 16 KB")
	}
	s.nativeMu.Lock()
	err := writeContent(content{Kind: "text", Data: []byte(text)})
	s.nativeMu.Unlock()
	if err != nil {
		return err
	}
	if err = s.transferForPaste(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peer == nil {
		return fmt.Errorf("操作連線尚未就緒")
	}
	// 指定 Windows 按鍵語意，由 Host 的既有跨平台映射轉換貼上組合鍵。
	for _, c := range []p2p.Control{{Type: "key", Key: "control", Down: true}, {Type: "key", Key: "v", Down: true}, {Type: "key", Key: "v", Down: false}, {Type: "key", Key: "control", Down: false}} {
		c.Platform = "windows"
		if err = s.peer.SendControl(c); err != nil {
			_ = s.peer.SendControl(p2p.Control{Type: "raw-key-reset"})
			return err
		}
	}
	return nil
}
