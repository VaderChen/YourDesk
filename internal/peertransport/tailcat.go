//go:build !winpe

package peertransport

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/tailscale/tailcat"
	"tailscale.com/wgengine/filter"
)

const tunnelPort = 47824 // Tailcat 虛擬網路埠，不開啟本機 TCP/UDP 監聽。
type tailcatBackend struct{}

func (tailcatBackend) Host(ctx context.Context) (Link, error) {
	p := newPacketLink(true)
	server := &tailcat.Server{
		Logf:           func(string, ...any) {},
		ServedTCPPorts: []filter.PortRange{},
		ServedUDPPorts: []filter.PortRange{{First: tunnelPort, Last: tunnelPort}},
		OnUDP: func(port uint16) func(tailcat.ConnPacketConn) {
			if port != tunnelPort {
				return nil
			}
			return func(c tailcat.ConnPacketConn) { p.attach(c) }
		},
	}
	// Start 無 context API；逾時後等初始化退出再回收，避免與 Start 競爭。
	ready := make(chan error, 1)
	go func() { ready <- server.Start() }()
	setup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	select {
	case err := <-ready:
		if err != nil {
			p.Close()
			return nil, errors.New("Tailcat 初始化失敗")
		}
	case <-setup.Done():
		p.Close()
		go func() {
			if <-ready == nil {
				server.Close()
			}
		}()
		return nil, errors.New("Tailcat 初始化逾時或已取消")
	}
	p.offer = string(server.TailcatAddr())
	p.cleanup = func() { server.Close() }
	return p, nil
}
func (tailcatBackend) Viewer(ctx context.Context, offer string) (Link, error) {
	if len(offer) == 0 || len(offer) > 8192 {
		return nil, errors.New("Tailcat 配對資料無效")
	}
	if _, err := tailcat.ParseAddr(tailcat.Addr(offer)); err != nil {
		return nil, errors.New("Tailcat 配對資料無效")
	}
	client := tailcat.NewClient(tailcat.Addr(offer))
	client.Logf = func(string, ...any) {}
	setup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c, err := client.DialUDPPort(setup, tunnelPort)
	if err != nil {
		client.Close()
		return nil, errors.New("Tailcat 連線失敗或已取消")
	}
	p := newPacketLink(false)
	p.cleanup = func() { client.Close() }
	p.attach(c)
	return p, nil
}

var _ net.PacketConn = (*packetLink)(nil)
