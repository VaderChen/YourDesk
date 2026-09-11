//go:build !winpe

package peertransport

import (
	"encoding/binary"
	"errors"
	"net"
	"os"
	"sync"
	"time"
)

// 在 Tailcat UDP 1232-byte 上限內分片。WebRTC 的 DTLS/SCTP 封包不受此 MTU 限制；
// 重組有數量、大小與時間上限，遺失交由原本的 WebRTC 通道語意處理。
const fragmentPayload = 1100
const fragmentHeader = 12
const maxDatagram = 65535

type packetLink struct {
	mu                          sync.Mutex
	writeMu                     sync.Mutex
	conn                        net.Conn
	closed                      bool
	done                        chan struct{}
	inbox                       chan []byte
	deadlineChanged             chan struct{}
	readDeadline, writeDeadline time.Time
	local, remote               *net.UDPAddr
	sequence                    uint64
	offer                       string
	cleanup                     func()
}

func newPacketLink(host bool) *packetLink {
	a, b := "127.0.0.2", "127.0.0.3"
	if !host {
		a, b = b, a
	}
	return &packetLink{done: make(chan struct{}), inbox: make(chan []byte, 32), deadlineChanged: make(chan struct{}),
		local: &net.UDPAddr{IP: net.ParseIP(a), Port: tunnelPort}, remote: &net.UDPAddr{IP: net.ParseIP(b), Port: tunnelPort}}
}
func (p *packetLink) Offer() string       { return p.offer }
func (p *packetLink) LocalAddr() net.Addr { return p.local }
func (p *packetLink) attach(c net.Conn) {
	p.mu.Lock()
	if p.closed || p.conn != nil {
		p.mu.Unlock()
		c.Close()
		return
	}
	if err := c.SetWriteDeadline(p.writeDeadline); err != nil {
		p.mu.Unlock()
		c.Close()
		return
	}
	p.conn = c
	p.mu.Unlock()
	go p.receive(c)
}
func (p *packetLink) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.done)
	c := p.conn
	cleanup := p.cleanup
	p.mu.Unlock()
	if c != nil {
		c.Close()
	}
	if cleanup != nil {
		cleanup()
	}
	return nil
}
func (p *packetLink) ReadFrom(buf []byte) (int, net.Addr, error) {
	for {
		p.mu.Lock()
		until := p.readDeadline
		changed := p.deadlineChanged
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return 0, nil, net.ErrClosed
		}
		var expired <-chan time.Time
		var timer *time.Timer
		if !until.IsZero() {
			if !time.Now().Before(until) {
				return 0, nil, os.ErrDeadlineExceeded
			}
			timer = time.NewTimer(time.Until(until))
			expired = timer.C
		}
		select {
		case b := <-p.inbox:
			if timer != nil {
				timer.Stop()
			}
			return copy(buf, b), p.remote, nil
		case <-p.done:
			if timer != nil {
				timer.Stop()
			}
			return 0, nil, net.ErrClosed
		case <-expired:
			return 0, nil, os.ErrDeadlineExceeded
		case <-changed:
			if timer != nil {
				timer.Stop()
			}
		}
	}
}
func (p *packetLink) WriteTo(buf []byte, addr net.Addr) (int, error) {
	if len(buf) > maxDatagram {
		return 0, errors.New("虛擬傳輸封包過大")
	}
	if addr == nil || addr.String() != p.remote.String() {
		return 0, errors.New("虛擬傳輸目的端不符")
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	p.mu.Lock()
	c := p.conn
	closed := p.closed
	until := p.writeDeadline
	p.mu.Unlock()
	if closed {
		return 0, net.ErrClosed
	}
	if !until.IsZero() && !time.Now().Before(until) {
		return 0, os.ErrDeadlineExceeded
	}
	// Host 尚未收到對端 UDP flow 時等同封包遺失，ICE 自行重送，不阻塞 mux。
	if c == nil {
		return len(buf), nil
	}
	p.sequence++
	count := (len(buf) + fragmentPayload - 1) / fragmentPayload
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		start := i * fragmentPayload
		end := min(start+fragmentPayload, len(buf))
		fragment := make([]byte, fragmentHeader+end-start)
		binary.BigEndian.PutUint64(fragment, p.sequence)
		binary.BigEndian.PutUint16(fragment[8:], uint16(i))
		binary.BigEndian.PutUint16(fragment[10:], uint16(count))
		copy(fragment[fragmentHeader:], buf[start:end])
		if _, err := c.Write(fragment); err != nil {
			return 0, err
		}
	}
	return len(buf), nil
}
func (p *packetLink) SetReadDeadline(t time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return net.ErrClosed
	}
	p.readDeadline = t
	close(p.deadlineChanged)
	p.deadlineChanged = make(chan struct{})
	return nil
}
func (p *packetLink) SetWriteDeadline(t time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return net.ErrClosed
	}
	p.writeDeadline = t
	if p.conn != nil {
		return p.conn.SetWriteDeadline(t)
	}
	return nil
}
func (p *packetLink) SetDeadline(t time.Time) error {
	if err := p.SetReadDeadline(t); err != nil {
		return err
	}
	return p.SetWriteDeadline(t)
}

type partial struct {
	created        time.Time
	parts          [][]byte
	received, size int
}

func (p *packetLink) receive(c net.Conn) {
	defer p.Close()
	pending := make(map[uint64]*partial)
	buf := make([]byte, 1232)
	for {
		n, err := c.Read(buf)
		if err != nil {
			return
		}
		if n < fragmentHeader || n > fragmentHeader+fragmentPayload {
			continue
		}
		id := binary.BigEndian.Uint64(buf)
		index := int(binary.BigEndian.Uint16(buf[8:]))
		count := int(binary.BigEndian.Uint16(buf[10:]))
		if count < 1 || count > 60 || index >= count || (index < count-1 && n != fragmentHeader+fragmentPayload) {
			continue
		}
		now := time.Now()
		for k, v := range pending {
			if now.Sub(v.created) > 2*time.Second {
				delete(pending, k)
			}
		}
		a := pending[id]
		if a == nil {
			if len(pending) >= 32 {
				continue
			}
			a = &partial{created: now, parts: make([][]byte, count)}
			pending[id] = a
		}
		if len(a.parts) != count || a.parts[index] != nil {
			continue
		}
		a.parts[index] = append([]byte{}, buf[fragmentHeader:n]...)
		a.received++
		a.size += n - fragmentHeader
		if a.size > maxDatagram {
			delete(pending, id)
			continue
		}
		if a.received != count {
			continue
		}
		data := make([]byte, 0, a.size)
		for _, part := range a.parts {
			data = append(data, part...)
		}
		delete(pending, id)
		select {
		case p.inbox <- data:
		case <-p.done:
			return
		default:
		}
	}
}
