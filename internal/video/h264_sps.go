package video

import "fmt"

type h264Bits struct {
	data    []byte
	offset  int
	invalid bool
}

func (b *h264Bits) read(n int) uint32 {
	if n < 0 || n > 32 || b.offset+n > len(b.data)*8 {
		b.invalid = true
		return 0
	}
	var value uint32
	for i := 0; i < n; i++ {
		value = value<<1 | uint32((b.data[b.offset/8]>>uint(7-b.offset%8))&1)
		b.offset++
	}
	return value
}
func (b *h264Bits) ue() uint32 {
	zeros := 0
	for !b.invalid && b.read(1) == 0 {
		zeros++
		if zeros > 30 {
			b.invalid = true
			return 0
		}
	}
	return (1 << uint(zeros)) - 1 + b.read(zeros)
}
func (b *h264Bits) se() int {
	value := b.ue()
	if value&1 != 0 {
		return int(value+1) / 2
	}
	return -int(value) / 2
}

// 僅解析初始化解碼器所需的尺寸，不依賴被控端是否也有編碼能力。
func h264Dimensions(annexB []byte) (int, int, error) {
	nals, err := h264AnnexBNALs(annexB)
	if err != nil {
		return 0, 0, err
	}
	var sps []byte
	for _, nal := range nals {
		if nal[0]&31 == 7 {
			sps = nal[1:]
			break
		}
	}
	if len(sps) == 0 || len(sps) > 65536 {
		return 0, 0, fmt.Errorf("H.264 SPS 無效")
	}
	rbsp := make([]byte, 0, len(sps))
	zeros := 0
	for _, v := range sps {
		if zeros == 2 && v == 3 {
			zeros = 0
			continue
		}
		rbsp = append(rbsp, v)
		if v == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	b := h264Bits{data: rbsp}
	profile := b.read(8)
	b.read(8)
	b.read(8)
	b.ue()
	chroma := uint32(1)
	switch profile {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		chroma = b.ue()
		if chroma > 3 {
			return 0, 0, fmt.Errorf("H.264 色度格式無效")
		}
		if chroma == 3 && b.read(1) != 0 {
			chroma = 0
		}
		if b.ue() != 0 || b.ue() != 0 {
			return 0, 0, fmt.Errorf("目前僅支援 8-bit H.264")
		}
		b.read(1)
		if b.read(1) != 0 {
			count := 8
			if chroma == 3 {
				count = 12
			}
			for i := 0; i < count; i++ {
				if b.read(1) == 0 {
					continue
				}
				last, next := 8, 8
				size := 16
				if i >= 6 {
					size = 64
				}
				for j := 0; j < size; j++ {
					if next != 0 {
						next = (last + b.se() + 256) & 255
					}
					if next != 0 {
						last = next
					}
				}
			}
		}
	}
	b.ue()
	poc := b.ue()
	switch poc {
	case 0:
		b.ue()
	case 1:
		b.read(1)
		b.se()
		b.se()
		count := b.ue()
		if count > 255 {
			return 0, 0, fmt.Errorf("H.264 POC 無效")
		}
		for i := uint32(0); i < count; i++ {
			b.se()
		}
	case 2:
	default:
		b.invalid = true
	}
	b.ue()
	b.read(1)
	mw, mh := b.ue()+1, b.ue()+1
	frameOnly := b.read(1)
	if frameOnly == 0 {
		b.read(1)
	}
	b.read(1)
	var left, right, top, bottom uint32
	if b.read(1) != 0 {
		left, right, top, bottom = b.ue(), b.ue(), b.ue(), b.ue()
	}
	cropX, cropY := uint64(1), uint64(2-frameOnly)
	if chroma == 1 {
		cropX, cropY = 2, 2*cropY
	} else if chroma == 2 {
		cropX = 2
	}
	width, height := uint64(mw)*16, uint64(mh)*16*uint64(2-frameOnly)
	cropW, cropH := (uint64(left)+uint64(right))*cropX, (uint64(top)+uint64(bottom))*cropY
	if b.invalid || width <= cropW || height <= cropH || width > 8192 || height > 8192 {
		return 0, 0, fmt.Errorf("H.264 SPS 尺寸無效")
	}
	return int(width - cropW), int(height - cropH), nil
}
