package p2p

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/transport/v4"
	"github.com/pion/transport/v4/stdnet"
	"yourdesk/internal/authlog"
)

// 只在診斷版觀測 socket 邊界，不改 MTU、不保存封包或端點內容。
var diagnosticSocketSequence atomic.Uint64
var diagnosticUDPSent, diagnosticUDPReceived atomic.Uint64

type diagnosticNet struct{ transport.Net }
type packetCounts struct {
	Packets  uint64 `json:"packets"`
	Bytes    uint64 `json:"bytes"`
	Max      int    `json:"maxPayload"`
	Over1200 uint64 `json:"over1200"`
	Over1400 uint64 `json:"over1400"`
	Over1472 uint64 `json:"over1472"`
	Errors   uint64 `json:"errors"`
	Short    uint64 `json:"shortOrFullBuffer"`
}

func (c *packetCounts) record(n, capacity int, err error, write bool) {
	if err != nil {
		c.Errors++
	}
	if n <= 0 {
		return
	}
	c.Packets++
	c.Bytes += uint64(n)
	if n > c.Max {
		c.Max = n
	}
	if n > 1200 {
		c.Over1200++
	}
	if n > 1400 {
		c.Over1400++
	}
	if n > 1472 {
		c.Over1472++
	}
	if (write && n < capacity) || (!write && n == capacity) {
		c.Short++
	}
}

type diagnosticUDP struct {
	transport.UDPConn
	number uint64
	mu     sync.Mutex
	tx, rx packetCounts
	done   chan struct{}
	once   sync.Once
}

func newDiagnosticNet() (transport.Net, error) {
	n, err := stdnet.NewNet()
	if err != nil {
		return nil, err
	}
	return &diagnosticNet{n}, nil
}
func (n *diagnosticNet) ListenUDP(network string, a *net.UDPAddr) (transport.UDPConn, error) {
	c, err := n.Net.ListenUDP(network, a)
	if err != nil {
		return nil, err
	}
	return wrapDiagnosticUDP(c), nil
}
func (n *diagnosticNet) DialUDP(network string, l, r *net.UDPAddr) (transport.UDPConn, error) {
	c, err := n.Net.DialUDP(network, l, r)
	if err != nil {
		return nil, err
	}
	return wrapDiagnosticUDP(c), nil
}
func wrapDiagnosticUDP(c transport.UDPConn) *diagnosticUDP {
	d := &diagnosticUDP{UDPConn: c, number: diagnosticSocketSequence.Add(1), done: make(chan struct{})}
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-d.done:
				d.report(true)
				return
			case <-t.C:
				d.report(false)
			}
		}
	}()
	return d
}
func (d *diagnosticUDP) report(closed bool) {
	d.mu.Lock()
	tx, rx := d.tx, d.rx
	d.mu.Unlock()
	// 指標跨取樣累計；不記錄 IP、port、密碼或資料內容。
	authlog.Event("udp-socket", map[string]any{"layer": "L6", "socket": d.number, "tx": tx, "rx": rx, "closed": closed})
}
func (d *diagnosticUDP) ReadFrom(b []byte) (int, net.Addr, error) {
	n, a, e := d.UDPConn.ReadFrom(b)
	if n > 0 {
		diagnosticUDPReceived.Add(uint64(n))
	}
	d.mu.Lock()
	d.rx.record(n, len(b), e, false)
	d.mu.Unlock()
	return n, a, e
}
func (d *diagnosticUDP) WriteTo(b []byte, a net.Addr) (int, error) {
	n, e := d.UDPConn.WriteTo(b, a)
	if n > 0 {
		diagnosticUDPSent.Add(uint64(n))
	}
	d.mu.Lock()
	d.tx.record(n, len(b), e, true)
	d.mu.Unlock()
	return n, e
}
func (d *diagnosticUDP) Close() error {
	e := d.UDPConn.Close()
	d.once.Do(func() { close(d.done) })
	return e
}
