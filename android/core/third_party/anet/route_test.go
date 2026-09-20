package anet

import (
	"errors"
	"net"
	"testing"
)

type routeConn struct {
	net.Conn
	addr   net.Addr
	closed *int
}

func (c *routeConn) LocalAddr() net.Addr { return c.addr }
func (c *routeConn) Close() error        { *c.closed++; return nil }

func TestRoutesRecoverFromOfflineAndRefreshWiFi(t *testing.T) {
	current := ""
	closed := 0
	dial := func(network, destination string) (net.Conn, error) {
		if current == "" || network != "udp4" {
			return nil, errors.New("no route")
		}
		return &routeConn{addr: &net.UDPAddr{IP: net.ParseIP(current)}, closed: &closed}, nil
	}
	if _, err := discoverRouteAddrs(dial); err == nil {
		t.Fatal("offline discovery unexpectedly succeeded")
	}
	for _, ip := range []string{"192.168.1.2", "10.0.0.8", "172.16.2.5"} {
		current = ip
		addrs, err := discoverRouteAddrs(dial)
		if err != nil || len(addrs) != 1 || addrs[0].(*net.IPNet).IP.String() != ip {
			t.Fatalf("cached route: %v %v", addrs, err)
		}
	}
	if closed != 3 {
		t.Fatalf("probe leaked socket: closed=%d", closed)
	}
}

func TestRoutesIPv6OnlyAndInvalidAddresses(t *testing.T) {
	closed := 0
	dial := func(network, destination string) (net.Conn, error) {
		if network != "udp6" {
			return nil, errors.New("IPv4 unavailable")
		}
		return &routeConn{addr: &net.UDPAddr{IP: net.ParseIP("2001:db8::42")}, closed: &closed}, nil
	}
	addrs, err := discoverRouteAddrs(dial)
	if err != nil || len(addrs) != 1 || addrs[0].String() != "2001:db8::42/128" || closed != 1 {
		t.Fatalf("IPv6-only discovery failed: %v %v", addrs, err)
	}
	for _, ip := range []string{"0.0.0.0", "::", "127.0.0.1", "fe80::1"} {
		dial := func(string, string) (net.Conn, error) {
			return &routeConn{addr: &net.UDPAddr{IP: net.ParseIP(ip)}, closed: &closed}, nil
		}
		if _, err := discoverRouteAddrs(dial); err == nil {
			t.Fatalf("unusable address accepted: %s", ip)
		}
	}
}
