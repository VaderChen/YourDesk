package video

import (
	"encoding/binary"
	"fmt"
)

// 將既有 VPS／SPS／PPS 封包轉成 Media Foundation 使用的 Annex B。
func unpackHEVC(data []byte) ([]byte, error) {
	if _, err := IsKeyframe(WireHEVC, data); err != nil {
		return nil, err
	}
	data = data[1:]
	out := make([]byte, 0, len(data))
	index := 0
	for len(data) > 0 {
		n := int(binary.BigEndian.Uint32(data))
		data = data[4:]
		if n < 2 {
			return nil, fmt.Errorf("HEVC NAL 截斷")
		}
		kind := (data[0] >> 1) & 63
		if index < 3 && kind != byte(32+index) {
			return nil, fmt.Errorf("HEVC 參數集無效")
		}
		out = append(out, 0, 0, 0, 1)
		out = append(out, data[:n]...)
		data = data[n:]
		index++
	}
	return out, nil
}
