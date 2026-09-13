package p2p

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestDiagnosticUDPRoundTrip(t *testing.T) {
	n, err := newDiagnosticNet()
	if err != nil {
		t.Fatal(err)
	}
	a, err := n.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := n.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	b.SetReadDeadline(time.Now().Add(2 * time.Second))
	payload := bytes.Repeat([]byte{42}, 1500)
	if _, err = a.WriteTo(payload, b.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	got, _, err := b.ReadFrom(buf)
	if err != nil || !bytes.Equal(buf[:got], payload) {
		t.Fatalf("資料不一致：%d %v", got, err)
	}
	tx := a.(*diagnosticUDP)
	rx := b.(*diagnosticUDP)
	tx.mu.Lock()
	c := tx.tx
	tx.mu.Unlock()
	if c.Packets != 1 || c.Bytes != 1500 || c.Over1472 != 1 || c.Errors != 0 {
		t.Fatal(c)
	}
	rx.mu.Lock()
	c = rx.rx
	rx.mu.Unlock()
	if c.Packets != 1 || c.Max != 1500 || c.Short != 0 {
		t.Fatal(c)
	}
}
