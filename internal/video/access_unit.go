package video

import (
	"encoding/binary"
	"errors"
	"fmt"
)

var ErrNeedKeyframe = errors.New("等待 IDR 以恢復參考影格")

// IsKeyframe 讀取實際 NAL，不能把全畫面更新等同於 IDR。
func IsKeyframe(codec WireCodec, data []byte) (bool, error) {
	count := 2
	if codec == WireHEVC {
		count = 3
	} else if codec != WireH264 {
		return false, fmt.Errorf("不支援的影片格式")
	}
	if len(data) < 1 || len(data) > 32<<20 || int(data[0]) != count {
		return false, fmt.Errorf("無效影片封包")
	}
	data = data[1:]
	index := 0
	key, picture := false, false
	for len(data) > 0 {
		if len(data) < 4 {
			return false, fmt.Errorf("影片 NAL 截斷")
		}
		n := int(binary.BigEndian.Uint32(data))
		data = data[4:]
		if n < 1 || n > len(data) {
			return false, fmt.Errorf("影片 NAL 長度無效")
		}
		if index >= count {
			if codec == WireH264 {
				kind := data[0] & 31
				key = key || kind == 5
				picture = picture || kind == 1 || kind == 5
			} else {
				if n < 2 {
					return false, fmt.Errorf("HEVC NAL 截斷")
				}
				kind := (data[0] >> 1) & 63
				key = key || (kind >= 16 && kind <= 21)
				picture = picture || kind <= 31
			}
		}
		index++
		data = data[n:]
	}
	if !picture {
		return false, fmt.Errorf("影片缺少影像 NAL")
	}
	return key, nil
}
