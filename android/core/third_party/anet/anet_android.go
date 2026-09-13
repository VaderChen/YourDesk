//go:build android

// Package anet supplies the small network-interface surface used by Pion.
// Android 沙盒通常拒絕 RTM_GETADDR 的 netlink 查詢；這裡以不送出封包的
// UDP connect 取得系統實際選用的本機位址，供 Pion 產生 Host candidate。
package anet

import (
	"errors"
	"net"
	"sync"
)

var (
	androidInterfaceOnce sync.Once
	androidInterface     net.Interface
	androidAddr          net.Addr
	androidInterfaceErr  error
)

func discover() {
	androidInterface = net.Interface{Index: 1, Name: "android-default", MTU: 1500, Flags: net.FlagUp | net.FlagRunning}
	for _, destination := range []string{"8.8.8.8:53", "1.1.1.1:53", "208.67.222.222:53"} {
		conn, err := net.Dial("udp4", destination)
		if err != nil {
			continue
		}
		local, ok := conn.LocalAddr().(*net.UDPAddr)
		_ = conn.Close()
		if ok && local != nil && local.IP != nil && !local.IP.IsUnspecified() {
			androidAddr = &net.IPNet{IP: append(net.IP(nil), local.IP...), Mask: net.CIDRMask(32, 32)}
			return
		}
	}
	androidInterfaceErr = errors.New("找不到 Android 可用的 IPv4 路由位址")
}

func ensure() error { androidInterfaceOnce.Do(discover); return androidInterfaceErr }
func Interfaces() ([]net.Interface, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	return []net.Interface{androidInterface}, nil
}
func InterfaceAddrs() ([]net.Addr, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	return []net.Addr{androidAddr}, nil
}
func InterfaceAddrsByInterface(ifi *net.Interface) ([]net.Addr, error) {
	if ifi == nil || ifi.Index != androidInterface.Index {
		return nil, errors.New("無效的 Android 網路介面")
	}
	if err := ensure(); err != nil {
		return nil, err
	}
	return []net.Addr{androidAddr}, nil
}
func InterfaceByIndex(index int) (*net.Interface, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	if index != androidInterface.Index {
		return nil, errors.New("找不到 Android 網路介面")
	}
	copy := androidInterface
	return &copy, nil
}
func InterfaceByName(name string) (*net.Interface, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	if name != androidInterface.Name {
		return nil, errors.New("找不到 Android 網路介面")
	}
	copy := androidInterface
	return &copy, nil
}
func SetAndroidVersion(uint) {}
