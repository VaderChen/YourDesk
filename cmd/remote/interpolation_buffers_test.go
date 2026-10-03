package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
	"yourdesk/internal/frameinterp"
)

func TestInterpolationSnapshotPixels(t *testing.T) {
	for _, method := range []string{"rife", "apple"} {
		for _, size := range []image.Point{{1, 1}, {63, 47}, {1920, 1080}, {1080, 1920}} {
			t.Run(fmt.Sprintf("%s/%dx%d", method, size.X, size.Y), func(t *testing.T) {
				parent := image.NewRGBA(image.Rect(0, 0, size.X+7, size.Y+5))
				for i := range parent.Pix {
					parent.Pix[i] = byte(i*31 + i/101)
				}
				source := parent.SubImage(image.Rect(3, 2, size.X+3, size.Y+2)).(*image.RGBA)
				want := image.NewRGBA(image.Rectangle{Max: size})
				w, h := frameinterp.AppleWorkingSize(size.X, size.Y)
				scaled := method == "apple" && w >= 2 && h >= 2
				if scaled {
					want = image.NewRGBA(image.Rect(0, 0, w, h))
				}
				var previous *image.RGBA
				for pass := 0; pass < 3; pass++ {
					source.Pix[0] = byte(pass * 37)
					if scaled {
						xdraw.ApproxBiLinear.Scale(want, want.Bounds(), source, source.Bounds(), draw.Src, nil)
					} else {
						draw.Draw(want, want.Bounds(), source, source.Bounds().Min, draw.Src)
					}
					got := interpolationSnapshot(source, method, previous)
					if got == source || got.Rect != want.Rect || got.Stride != want.Stride || !bytes.Equal(got.Pix, want.Pix) {
						t.Fatal("快照尺寸、stride 或像素改變")
					}
					if previous != nil && got != previous {
						t.Fatal("相同尺寸的待處理快照未重用")
					}
					source.Pix[0]++
					if !bytes.Equal(got.Pix, want.Pix) {
						t.Fatal("快照與來源共用像素")
					}
					previous = got
				}
			})
		}
	}
}

func TestInterpolationSnapshotRejectsIncompatibleBuffer(t *testing.T) {
	bounds := image.Rect(0, 0, 5, 4)
	source := image.NewRGBA(bounds)
	source.Pix[0] = 37
	for _, old := range []*image.RGBA{
		nil, source,
		image.NewRGBA(image.Rect(0, 0, 6, 4)),
		image.NewRGBA(image.Rect(1, 2, 6, 6)),
		{Rect: bounds, Stride: 24, Pix: make([]byte, 96)},
		{Rect: bounds, Stride: 20, Pix: make([]byte, 79)},
	} {
		got := interpolationSnapshot(source, "rife", old)
		if got == old || got.Rect != bounds || got.Stride != 20 || !bytes.Equal(got.Pix, source.Pix) {
			t.Fatal("不相容的緩衝未替換為獨立且完整的快照")
		}
	}
}

func TestInterpolationQueuedSnapshotOwnership(t *testing.T) {
	if !frameinterp.AppleSupported() {
		t.Skip("需要 Apple 排程路徑；不執行原生推論")
	}
	oldMethod := viewerInterpolationMethod.Swap(1)
	defer viewerInterpolationMethod.Store(oldMethod)
	source := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for i := range source.Pix {
		source.Pix[i] = 37
	}
	active := interpolationSnapshot(source, "apple", nil)
	expected := bytes.Clone(active.Pix)
	p := frameInterpolator{method: "apple", sourceBounds: source.Bounds(), previous: active, shown: active, target: active, deadline: time.Now().Add(time.Hour)}
	p.busy.Store(true)
	defer p.reset()
	done := make(chan bool, 1)
	go func() {
		unchanged := true
		for i := 0; i < 2000; i++ {
			unchanged = bytes.Equal(active.Pix, expected) && unchanged
		}
		done <- unchanged
	}()
	var queued *image.RGBA
	for i := 0; i < 200; i++ {
		source.Pix[0] = byte(i)
		out, changed := p.frame(source, 0, true, true)
		if out != active || changed || p.queued == active || queued != nil && queued != p.queued {
			t.Fatal("替換待處理影格改變了顯示／推論中的影格")
		}
		queued = p.queued
	}
	if !<-done || !bytes.Equal(active.Pix, expected) {
		t.Fatal("推論中的像素被改寫")
	}
	p.deadline = time.Time{}
	p.frame(source, 0, false, true)
	if p.queued != nil || p.previous != queued || p.shown != queued {
		t.Fatal("待處理影格未依既有順序交付")
	}
	retained := bytes.Clone(queued.Pix)
	source.Pix[0]++
	p.frame(source, 0, true, true)
	if p.previous == queued || !bytes.Equal(queued.Pix, retained) {
		t.Fatal("已交付的影格被後續快照覆寫")
	}
	p.frame(source, 0, false, false)
	if p.queued != nil || p.previous != nil || p.target != nil || p.shown != nil || p.results != nil {
		t.Fatal("停用後仍保留影格")
	}
}

func BenchmarkInterpolationSnapshot(b *testing.B) {
	for _, method := range []string{"rife", "apple"} {
		for _, size := range []image.Point{{1920, 1080}, {3840, 2160}} {
			for _, reuse := range []bool{false, true} {
				b.Run(fmt.Sprintf("%s/%dx%d/reuse=%t", method, size.X, size.Y, reuse), func(b *testing.B) {
					source := image.NewRGBA(image.Rectangle{Max: size})
					snapshot := interpolationSnapshot(source, method, nil)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if !reuse {
							snapshot = nil
						}
						snapshot = interpolationSnapshot(source, method, snapshot)
					}
				})
			}
		}
	}
}
