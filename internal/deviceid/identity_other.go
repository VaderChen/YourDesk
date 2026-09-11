//go:build !windows

package deviceid

import "errors"

func platformIdentity() (Identity, error) {
	candidates := []struct {
		source Source
		read   func() (string, error)
	}{
		{SourceCPU, cpuID},
		{SourceMainboard, mainboardID},
		{SourceMAC, physicalMAC},
	}
	for _, candidate := range candidates {
		raw, err := candidate.read()
		if err == nil && usable(raw) {
			return Identity{UID: encode(candidate.source, raw), Source: candidate.source}, nil
		}
	}
	return Identity{}, errors.New("無法取得 CPU ID、主機板 ID 或實體 MAC 位址")
}
