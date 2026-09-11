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
		return false, "Apple 補幀需要支援的 Mac 與 macOS 26 以上"
	}
	return Status()
}
func MidpointWithMethod(ctx context.Context, a, b *image.RGBA, method string) (*image.RGBA, error) {
	if ResolveMethod(method) != "apple" {
		return Midpoint(ctx, a, b)
	}
	inference.Lock()
	defer inference.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil || b == nil || a.Bounds() != b.Bounds() || a.Bounds().Empty() {
		return nil, fmt.Errorf("補幀影格尺寸不一致")
	}
	out, err := applePredict(a, b)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return out, err
}
