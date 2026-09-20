package anet

import (
	"errors"
	"net"
)

// This stateless resolver is injectable so network handover and offline
// recovery can be verified on the build host, without Android netlink access.
func discoverRouteAddrs(dial func(string, string) (net.Conn, error)) ([]net.Addr, error) {
	var addrs []net.Addr
	for _, route := range []struct {
		network      string
		destinations []string
	}{
		{"udp4", []string{"8.8.8.8:53", "1.1.1.1:53"}},
		{"udp6", []string{"[2001:4860:4860::8888]:53", "[2606:4700:4700::1111]:53"}},
	} {
		for _, destination := range route.destinations {
			conn, err := dial(route.network, destination)
			if err != nil {
				continue
			}
			local, ok := conn.LocalAddr().(*net.UDPAddr)
			_ = conn.Close()
			if !ok || local == nil || local.IP == nil || local.IP.IsUnspecified() || local.IP.IsLoopback() || local.IP.IsLinkLocalUnicast() {
				continue
			}
			bits, ip := 128, local.IP.To16()
			if route.network == "udp4" {
				bits, ip = 32, local.IP.To4()
			}
			if ip == nil {
				continue
			}
			addrs = append(addrs, &net.IPNet{IP: append(net.IP(nil), ip...), Mask: net.CIDRMask(bits, bits)})
			break
		}
	}
	if len(addrs) == 0 {
		return nil, errors.New("找不到 Android 可用的 IPv4／IPv6 路由位址")
	}
	return addrs, nil
}
