package deviceid

import (
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"net"
	"strings"
)

type Source string

const (
	SourceWindowsMachine Source = "windows-machine-guid"
	SourceSystemUUID     Source = "windows-system-uuid"
	SourceCPU            Source = "cpu"
	SourceMainboard      Source = "mainboard"
	SourceMAC            Source = "mac"
)

type Identity struct {
	UID    string
	Source Source
}

// Current returns the platform-specific, hashed device identity.
func Current() (Identity, error) { return platformIdentity() }

func encode(source Source, raw string) string {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	sum := sha256.Sum256([]byte("yourdesk-device-v1\x00" + string(source) + "\x00" + normalized))
	// 12 bytes encode to exactly 20 Base32 characters.
	value := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:12])
	return fmt.Sprintf("YD-%s-%s-%s-%s-%s", value[0:4], value[4:8], value[8:12], value[12:16], value[16:20])
}

func usable(value string) bool {
	v := strings.ToUpper(strings.TrimSpace(value))
	return v != "" && v != "UNKNOWN" && v != "NONE" && v != "DEFAULT STRING" && v != "TO BE FILLED BY O.E.M." && v != "0000000000000000"
}

func physicalMAC() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 || len(iface.HardwareAddr) != 6 {
			continue
		}
		mac := iface.HardwareAddr
		if mac[0]&2 != 0 || allZero(mac) {
			continue
		} // skip locally administered/virtual MACs
		return mac.String(), nil
	}
	return "", errors.New("找不到有效的實體 MAC")
}

func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
