package hostsession

import (
	"context"
	"fmt"
	"sync/atomic"

	"yourdesk/internal/filetransfer"
	"yourdesk/internal/p2p"
)

// Lifecycle admission/state notifications are shared with Stream, but command
// registration is separate: file peers never acquire shell, desktop or input.
func filesSession(ctx context.Context, peer *p2p.Peer) error {
	var authorized atomic.Bool
	authorized.Store(true)
	defer authorized.Store(false)
	filetransfer.Register(peer, authorized.Load)
	_ = peer.RegisterCommand("session.capabilities", func(context.Context) (any, error) {
		return map[string]any{"schema": 1, "desktop": false, "terminal": false, "clipboard": false, "files": true}, nil
	})
	fmt.Println(`YOURDESK_UI_EVENT {"event":"host-ready"}`)
	fmt.Println(`YOURDESK_UI_EVENT {"event":"authenticated"}`)
	select {
	case <-ctx.Done():
	case <-peer.Done():
	}
	return nil
}
