// Package pixelconv 提供保留既有像素結果的紅／藍通道交換。
package pixelconv

// SwapOpaque 支援不同 stride；緩衝需不重疊（或完全同址），alpha 固定 255。
func SwapOpaque(dst, src []byte, w, h, srcStride, dstStride int) bool {
	if w < 1 || h < 1 || w > 8192 || h > 8192 || srcStride < w*4 || dstStride < w*4 {
		return false
	}
	if len(src) < w*4 || len(dst) < w*4 {
		return false
	}
	if h > 1 && (srcStride > (len(src)-w*4)/(h-1) || dstStride > (len(dst)-w*4)/(h-1)) {
		return false
	}
	swap(dst, src, w, h, srcStride, dstStride)
	return true
}
