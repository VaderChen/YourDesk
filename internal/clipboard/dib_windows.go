package clipboard

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math/bits"
)

func decodeDIB(data []byte) (image.Image, error) {
	invalid := fmt.Errorf("不支援的剪貼簿 DIB 圖片")
	if len(data) < 40 {
		return nil, invalid
	}
	header := int(binary.LittleEndian.Uint32(data))
	w := int(int32(binary.LittleEndian.Uint32(data[4:])))
	signedH := int(int32(binary.LittleEndian.Uint32(data[8:])))
	h := signedH
	if h < 0 {
		h = -h
	}
	depth := int(binary.LittleEndian.Uint16(data[14:]))
	compression := binary.LittleEndian.Uint32(data[16:])
	if header < 40 || header > len(data) || w < 1 || h < 1 || int64(w)*int64(h) > MaxPixels || (depth != 24 && depth != 32) || (compression != 0 && compression != 3) || binary.LittleEndian.Uint16(data[12:]) != 1 {
		return nil, invalid
	}
	offset := header
	masks := [4]uint32{0xff0000, 0xff00, 0xff, 0}
	if compression == 3 {
		pos := 40
		if len(data) < pos+12 {
			return nil, invalid
		}
		for i := 0; i < 3; i++ {
			masks[i] = binary.LittleEndian.Uint32(data[pos+i*4:])
			if masks[i] == 0 {
				return nil, invalid
			}
		}
		if header >= 56 {
			masks[3] = binary.LittleEndian.Uint32(data[52:])
		} else if header == 40 {
			offset += 12
		}
	}
	colors := int(binary.LittleEndian.Uint32(data[32:]))
	if colors > 256 {
		return nil, invalid
	}
	offset += colors * 4
	stride := (w*depth + 31) / 32 * 4
	if int64(offset)+int64(stride)*int64(h) > int64(len(data)) {
		return nil, invalid
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	component := func(pixel, mask uint32) uint8 {
		if mask == 0 {
			return 255
		}
		shift := bits.TrailingZeros32(mask)
		return uint8(uint64((pixel&mask)>>shift) * 255 / uint64(mask>>shift))
	}
	for y := 0; y < h; y++ {
		row := y
		if signedH > 0 {
			row = h - 1 - y
		}
		for x := 0; x < w; x++ {
			pos := offset + row*stride + x*(depth/8)
			pixel := uint32(data[pos]) | uint32(data[pos+1])<<8 | uint32(data[pos+2])<<16
			if depth == 32 {
				pixel |= uint32(data[pos+3]) << 24
			}
			img.SetNRGBA(x, y, color.NRGBA{R: component(pixel, masks[0]), G: component(pixel, masks[1]), B: component(pixel, masks[2]), A: component(pixel, masks[3])})
		}
	}
	return img, nil
}
func encodeDIB(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	data := make([]byte, 40+w*h*4)
	binary.LittleEndian.PutUint32(data, 40)
	binary.LittleEndian.PutUint32(data[4:], uint32(w))
	binary.LittleEndian.PutUint32(data[8:], uint32(h))
	binary.LittleEndian.PutUint16(data[12:], 1)
	binary.LittleEndian.PutUint16(data[14:], 32)
	binary.LittleEndian.PutUint32(data[20:], uint32(w*h*4))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			pos := 40 + ((h-1-y)*w+x)*4
			// CF_DIB 的 BI_RGB 不定義 alpha；以白底提供相容表示，PNG 保留透明度。
			a := uint32(c.A)
			data[pos] = byte((uint32(c.B)*a + 255*(255-a)) / 255)
			data[pos+1] = byte((uint32(c.G)*a + 255*(255-a)) / 255)
			data[pos+2] = byte((uint32(c.R)*a + 255*(255-a)) / 255)
		}
	}
	return data
}
