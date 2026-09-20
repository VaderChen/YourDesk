//go:build android

// Package anet supplies the small network-interface surface used by Pion.
// UDP connect only asks the kernel to select a local route; it sends no packet.
package anet

import (
	"errors"
	"net"
)

func androidDefaultInterface() net.Interface {
	return net.Interface{Index: 1, Name: "android-default", MTU: 1500, Flags: net.FlagUp | net.FlagRunning}
}

// Discover on every enumeration: never permanently cache a temporary offline
// failure, or an address that belonged to a previous Wi-Fi / cellular network.
func Interfaces() ([]net.Interface, error) {
	if _, err := discoverRouteAddrs(net.Dial); err != nil {
		return nil, err
	}
	return []net.Interface{androidDefaultInterface()}, nil
}
func InterfaceAddrs() ([]net.Addr, error) { return discoverRouteAddrs(net.Dial) }
func InterfaceAddrsByInterface(ifi *net.Interface) ([]net.Addr, error) {
	if ifi == nil || ifi.Index != 1 {
		return nil, errors.New("無效的 Android 網路介面")
	}
	return discoverRouteAddrs(net.Dial)
}
func InterfaceByIndex(index int) (*net.Interface, error) {
	if index != 1 {
		return nil, errors.New("找不到 Android 網路介面")
	}
	if _, err := discoverRouteAddrs(net.Dial); err != nil {
		return nil, err
	}
	ifi := androidDefaultInterface()
	return &ifi, nil
}
func InterfaceByName(name string) (*net.Interface, error) {
	if name != "android-default" {
		return nil, errors.New("找不到 Android 網路介面")
	}
	return InterfaceByIndex(1)
}
func SetAndroidVersion(uint) {}
