//go:build darwin && cgo

package frameinterp

import (
	"context"
	"image"
	"image/color"
	"os"
	"sort"
	"testing"
	"time"
)

// 明確開啟才啟動實際 Apple 模型，避免一般測試佔用 GPU。
func TestAppleHardwareSmoke(t *testing.T) {
	if os.Getenv("YOURDESK_APPLE_SMOKE") != "1" {
		t.Skip("設定 YOURDESK_APPLE_SMOKE=1 執行硬體 Smoke")
	}
	if !AppleSupported() {
		t.Fatal("目前系統未提供 Apple 補幀能力")
	}
	for _, size := range []image.Point{{1280, 720}, {720, 1280}, {640, 360}, {1280, 720}} {
		w, h := AppleWorkingSize(size.X, size.Y)
		if w < 2 || h < 2 || w%2 != 0 || h%2 != 0 {
			t.Fatalf("工作尺寸無效：%dx%d", w, h)
		}
		durations := []time.Duration{}
		for n := 0; n < 12; n++ {
			a, b := smokeFrame(w, h, 0), smokeFrame(w, h, 16)
			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			out, err := MidpointWithMethod(ctx, a, b, "apple")
			cancel()
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("%dx%d 第%d次：%v (%s)", w, h, n, err, elapsed)
			}
			if out.Bounds() != a.Bounds() {
				t.Fatal("輸出尺寸錯誤")
			}
			// 高亮物體必須位於兩張輸入之間，而非只重複任一張。
			x := smokeCentroid(out)
			left, right := smokeCentroid(a), smokeCentroid(b)
			if x < left+2 || x > right-2 {
				t.Fatalf("中間幀位置錯誤：%.2f，輸入 %.2f / %.2f", x, left, right)
			}
			if n == 0 {
				t.Logf("%dx%d 初始化／首幀 %s", w, h, elapsed)
			} else {
				durations = append(durations, elapsed)
			}
		}
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		t.Logf("%dx%d 穩態 p50=%s 最大=%s", w, h, durations[len(durations)/2], durations[len(durations)-1])
	}
}
func smokeFrame(w, h, offset int) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := uint8(25 + (x/32+y/32)%2*10)
			im.SetRGBA(x, y, color.RGBA{c, c, c, 255})
			if x >= w/3+offset && x < w/3+w/6+offset && y >= h/3 && y < h*2/3 {
				im.SetRGBA(x, y, color.RGBA{230, 230, 230, 255})
			}
		}
	}
	return im
}
func smokeCentroid(im *image.RGBA) float64 {
	var sum, count float64
	for y := 0; y < im.Bounds().Dy(); y++ {
		for x := 0; x < im.Bounds().Dx(); x++ {
			i := y*im.Stride + x*4
			if im.Pix[i] > 180 && im.Pix[i+1] > 180 && im.Pix[i+2] > 180 {
				sum += float64(x)
				count++
			}
		}
	}
	if count == 0 {
		return -1
	}
	return sum / count
}
