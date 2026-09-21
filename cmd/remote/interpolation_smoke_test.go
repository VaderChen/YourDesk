//go:build darwin && cgo

package main

import (
	"image"
	"image/color"
	"image/draw"
	"os"
	"testing"
	"time"
	"yourdesk/internal/frameinterp"
)

func TestApplePresentationSmoke(t *testing.T) {
	if os.Getenv("YOURDESK_APPLE_SMOKE") != "1" {
		t.Skip("設定 YOURDESK_APPLE_SMOKE=1 執行硬體呈現 Smoke")
	}
	if !frameinterp.AppleSupported() {
		t.Fatal("Apple 補幀不可用")
	}
	old := viewerInterpolationMethod.Swap(1)
	defer viewerInterpolationMethod.Store(old)
	var p frameInterpolator
	defer p.reset()
	source := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	next := time.Now()
	stop := next.Add(5 * time.Second)
	var real, generated int
	var longest time.Duration
	for frame := 0; time.Now().Before(stop); frame++ {
		dirty := !time.Now().Before(next)
		if dirty {
			draw.Draw(source, source.Bounds(), image.NewUniform(color.RGBA{30, 30, 30, 255}), image.Point{}, draw.Src)
			x := 300 + frame%100
			draw.Draw(source, image.Rect(x, 200, x+200, 450), image.NewUniform(color.RGBA{230, 230, 230, 255}), image.Point{}, draw.Src)
			next = time.Now().Add(50 * time.Millisecond)
		}
		start := time.Now()
		out, changed := p.frame(source, 0, dirty, true)
		elapsed := time.Since(start)
		if elapsed > longest {
			longest = elapsed
		}
		if out == nil {
			t.Fatal("暖機／推論期間沒有真實影格回退")
		}
		if changed {
			if p.generated {
				generated++
			} else {
				real++
			}
		}
		time.Sleep(8 * time.Millisecond)
	}
	if generated < 5 || real < 5 {
		t.Fatalf("未形成雙倍呈現：真實=%d 生成=%d 狀態=%s", real, generated, p.status)
	}
	out, _ := p.frame(source, 0, false, false)
	if out != source || p.previous != nil || p.target != nil {
		t.Fatal("關閉補幀未恢复來源影格")
	}
	t.Logf("5 秒呈現：真實=%d 生成=%d，最慢 frame 呼叫=%s", real, generated, longest)
}

func TestAppleSwitchingSmoke(t *testing.T) {
	if os.Getenv("YOURDESK_APPLE_SMOKE") != "1" {
		t.Skip("設定 YOURDESK_APPLE_SMOKE=1 執行硬體切換 Smoke")
	}
	if !frameinterp.AppleSupported() {
		t.Fatal("Apple 補幀不可用")
	}
	old := viewerInterpolationMethod.Swap(1)
	defer viewerInterpolationMethod.Store(old)
	var windows [2]frameInterpolator
	defer func() {
		for i := range windows {
			windows[i].reset()
		}
	}()
	start := time.Now()
	next := start
	var frames, generated, resets int
	var longest time.Duration
	for time.Since(start) < 20*time.Second {
		elapsed := time.Since(start)
		dirty := !time.Now().Before(next)
		if dirty {
			next = time.Now().Add(50 * time.Millisecond)
			frames++
		}
		for i := range windows {
			// 每四秒切換橫直向，每三秒停用半秒；不同尺寸共用原生推論鎖。
			w, h := 640, 360
			if (int(elapsed.Seconds())/4+i)%2 != 0 {
				w, h = h, w
			}
			source := image.NewRGBA(image.Rect(0, 0, w, h))
			draw.Draw(source, source.Bounds(), image.NewUniform(color.RGBA{30, 30, 30, 255}), image.Point{}, draw.Src)
			x := w/4 + frames%24
			draw.Draw(source, image.Rect(x, h/3, x+64, h*2/3), image.NewUniform(color.RGBA{230, 230, 230, 255}), image.Point{}, draw.Src)
			enabled := elapsed%(3*time.Second) < 2500*time.Millisecond
			before := time.Now()
			out, changed := windows[i].frame(source, i, dirty, enabled)
			duration := time.Since(before)
			if duration > longest {
				longest = duration
			}
			if out == nil {
				t.Fatal("切換中遺失回退畫面")
			}
			if !enabled {
				resets++
				if out != source || windows[i].results != nil {
					t.Fatal("停用未釋放結果通道")
				}
			}
			if changed && windows[i].generated {
				generated++
			}
		}
		time.Sleep(8 * time.Millisecond)
	}
	if generated < 10 || resets == 0 {
		t.Fatalf("切換後無法恢復：生成=%d 停用=%d", generated, resets)
	}
	t.Logf("雙視窗 20 秒切換：生成=%d，停用檢查=%d，最慢排程=%s", generated, resets, longest)
}
