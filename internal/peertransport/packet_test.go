//go:build !winpe

package peertransport

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

type packetTestConn struct {
	incoming [][]byte
	written  [][]byte
	record   bool
}

func (c *packetTestConn) Read(p []byte) (int, error) {
	if len(c.incoming) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.incoming[0])
	c.incoming = c.incoming[1:]
	return n, nil
}
func (c *packetTestConn) Write(p []byte) (int, error) {
	if c.record {
		c.written = append(c.written, bytes.Clone(p))
	}
	return len(p), nil
}
func (*packetTestConn) Close() error                     { return nil }
func (*packetTestConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (*packetTestConn) RemoteAddr() net.Addr             { return &net.UDPAddr{} }
func (*packetTestConn) SetDeadline(time.Time) error      { return nil }
func (*packetTestConn) SetReadDeadline(time.Time) error  { return nil }
func (*packetTestConn) SetWriteDeadline(time.Time) error { return nil }

func packetTestFragments(t testing.TB, data []byte) [][]byte {
	t.Helper()
	c := &packetTestConn{record: true}
	p := newPacketLink(true)
	p.conn = c
	if n, err := p.WriteTo(data, p.remote); err != nil || n != len(data) {
		t.Fatalf("WriteTo = %d, %v", n, err)
	}
	return c.written
}

func TestPacketFragmentWireAndOwnership(t *testing.T) {
	for _, size := range []int{0, 1, fragmentPayload, fragmentPayload + 1, maxDatagram} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i*31 + i/fragmentPayload)
			}
			fragments := packetTestFragments(t, data)
			count := max(1, (size+fragmentPayload-1)/fragmentPayload)
			if len(fragments) != count {
				t.Fatal("fragment count changed")
			}
			for i, part := range fragments {
				if binary.BigEndian.Uint64(part) != 1 || int(binary.BigEndian.Uint16(part[8:])) != i || int(binary.BigEndian.Uint16(part[10:])) != count || !bytes.Equal(part[fragmentHeader:], data[i*fragmentPayload:min((i+1)*fragmentPayload, size)]) {
					t.Fatal("fragment header or payload corrupted")
				}
			}
			// Reverse order and duplicate the first arriving fragment. The receive
			// buffer is overwritten on every Read; delivered bytes must remain owned.
			incoming := make([][]byte, 0, count+1)
			for i := count - 1; i >= 0; i-- {
				incoming = append(incoming, fragments[i])
				if i == count-1 && count > 1 {
					incoming = append(incoming, fragments[i])
				}
			}
			p := newPacketLink(false)
			p.receive(&packetTestConn{incoming: incoming})
			select {
			case got := <-p.inbox:
				if !bytes.Equal(got, data) {
					t.Fatal("reassembled payload corrupted")
				}
			default:
				t.Fatal("no reassembled packet")
			}
			if len(p.inbox) != 0 {
				t.Fatal("duplicate fragment was delivered twice")
			}
		})
	}
}

func TestPacketReceiveSinglePayloadOwnership(t *testing.T) {
	first := bytes.Repeat([]byte{0x31}, fragmentPayload)
	second := bytes.Repeat([]byte{0x72}, fragmentPayload)
	incoming := append(packetTestFragments(t, first), packetTestFragments(t, second)...)
	p := newPacketLink(false)
	p.receive(&packetTestConn{incoming: incoming})
	for _, want := range [][]byte{first, second} {
		select {
		case got := <-p.inbox:
			if !bytes.Equal(got, want) {
				t.Fatal("later Read overwrote an earlier packet")
			}
		default:
			t.Fatal("missing packet")
		}
	}
}

func BenchmarkPacketWriteTo(b *testing.B) {
	for _, size := range []int{64, 1400, 60000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			p := newPacketLink(true)
			p.conn = &packetTestConn{}
			data := make([]byte, size)
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := p.WriteTo(data, p.remote); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPacketReceiveSingle(b *testing.B) {
	fragments := packetTestFragments(b, make([]byte, fragmentPayload))
	b.ReportAllocs()
	b.SetBytes(fragmentPayload)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := newPacketLink(false)
		p.receive(&packetTestConn{incoming: fragments})
		<-p.inbox
	}
}
