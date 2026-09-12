//go:build !cgo

package pixelconv

func swap(dst, src []byte, w, h, ss, ds int) {
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i, j := y*ss+x*4, y*ds+x*4
			r, g, b := src[i], src[i+1], src[i+2]
			dst[j], dst[j+1], dst[j+2], dst[j+3] = b, g, r, 255
		}
	}
}
