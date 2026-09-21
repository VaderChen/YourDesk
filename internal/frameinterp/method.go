package frameinterp

import (
	"context"
	"fmt"
	"image"
)

// 空值保留舊設定相容性，優先選擇本機支援的 Apple，否則使用 RIFE。
func ResolveMethod(method string) string {
	if method == "apple" || method == "rife" {
		return method
	}
	if AppleSupported() {
		return "apple"
	}
	return "rife"
}
func MethodStatus(method string) (bool, string) {
	if ResolveMethod(method) == "apple" {
		if AppleSupported() {
			return true, ""
		}
		return false, "Apple 補幀需要支援的 Mac 與 macOS 27 以上"
	}
	return Status()
}
func MidpointWithMethod(ctx context.Context, a, b *image.RGBA, method string) (*image.RGBA, error) {
	if ResolveMethod(method) != "apple" {
		return Midpoint(ctx, a, b)
	}
	if !AppleSupported() {
		return nil, fmt.Errorf("Apple 補幀需要支援的 Mac 與 macOS 27 以上")
	}
	// 即時補幀不能排在另一個視窗的推論之後，逾時原生呼叫仍可能執行中。
	if !inference.TryLock() {
		return nil, fmt.Errorf("補幀處理忙碌，顯示真實影格")
	}
	defer inference.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validAppleFramePair(a, b) {
		return nil, fmt.Errorf("補幀影格尺寸不一致")
	}
	out, err := applePredict(a, b)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return out, err
}

// SubImage 的最後一列不一定保留完整 stride；只要求實際會讀取的像素。
func validAppleFramePair(a, b *image.RGBA) bool {
	if a == nil || b == nil || a.Bounds() != b.Bounds() {
		return false
	}
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	if w < 2 || h < 2 || w > 8192 || h > 8192 {
		return false
	}
	for _, f := range []*image.RGBA{a, b} {
		if f.Stride < w*4 || len(f.Pix) < w*4 || (h-1) > (len(f.Pix)-w*4)/f.Stride {
			return false
		}
	}
	return true
}
