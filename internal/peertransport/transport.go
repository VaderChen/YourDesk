// Package peertransport 提供 WebRTC 下方的虛擬封包傳輸層。
// 上層維持同一套 DataChannel；新增 Relay 時只需實作 Backend。
package peertransport

import (
	"context"
	"errors"
	"net"
	"sort"
	"sync"
)

type Mode string

const (
	Native  Mode = ""
	Tailcat Mode = "tailcat"
)

// Link 的 PacketConn 僅供單一 WebRTC 工作階段使用；Close 必須可重複呼叫。
// 原生模式回傳 nil，由 Pion 使用系統 UDP。
type Link interface {
	net.PacketConn
	Offer() string
}

type Backend interface {
	Host(context.Context) (Link, error)
	Viewer(context.Context, string) (Link, error)
}

var backends = struct {
	sync.RWMutex
	values map[Mode]Backend
}{values: initialBackends()}

// Register 只供程式內部於啟動時安裝其他傳輸後端，拒絕覆寫既有模式。
func Register(mode Mode, backend Backend) error {
	if mode == Native || backend == nil {
		return errors.New("無效的傳輸後端")
	}
	backends.Lock()
	defer backends.Unlock()
	if _, exists := backends.values[mode]; exists {
		return errors.New("傳輸後端已存在")
	}
	backends.values[mode] = backend
	return nil
}
func Open(ctx context.Context, mode Mode, host bool, offer string) (Link, error) {
	if mode == Native {
		return nil, nil
	}
	backends.RLock()
	backend := backends.values[mode]
	backends.RUnlock()
	if backend == nil {
		return nil, errors.New("不支援的傳輸模式")
	}
	if host {
		return backend.Host(ctx)
	}
	return backend.Viewer(ctx, offer)
}

// Modes 回報本程式可接受的虛擬傳輸模式，供已驗證的對端協商。
func Modes() []Mode {
	backends.RLock()
	defer backends.RUnlock()
	modes := make([]Mode, 0, len(backends.values))
	for mode := range backends.values {
		modes = append(modes, mode)
	}
	sort.Slice(modes, func(i, j int) bool { return modes[i] < modes[j] })
	return modes
}
func Supports(mode Mode) bool {
	if mode == Native {
		return true
	}
	backends.RLock()
	defer backends.RUnlock()
	return backends.values[mode] != nil
}
