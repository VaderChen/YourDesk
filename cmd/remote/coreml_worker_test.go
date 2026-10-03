package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"testing"
)

func TestMLQueueReplacementReusesSnapshot(t *testing.T) {
	parent := image.NewRGBA(image.Rect(0, 0, 12, 10))
	source := parent.SubImage(image.Rect(3, 2, 8, 6)).(*image.RGBA)
	for y := source.Rect.Min.Y; y < source.Rect.Max.Y; y++ {
		for x := source.Rect.Min.X; x < source.Rect.Max.X; x++ {
			source.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 30, 255})
		}
	}
	w := &mlWorker{jobs: make(chan mlJob, 1), generation: 4}
	w.enqueue(source, 1, 0)
	first := <-w.jobs
	if first.frame == source || first.frame.Rect != image.Rect(0, 0, 5, 4) || first.frame.Stride != 20 || len(first.frame.Pix) != 80 {
		t.Fatal("快照必須使用獨立、零起點且緊密排列的 RGBA")
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 5; x++ {
			if first.frame.RGBAAt(x, y) != source.RGBAAt(x+3, y+2) {
				t.Fatal("快照未正確複製非零起點及額外 stride 的來源")
			}
		}
	}
	w.jobs <- first
	w.generation = 5
	source.SetRGBA(3, 2, color.RGBA{90, 80, 70, 255})
	w.enqueue(source, 2, 1)
	latest := <-w.jobs
	if latest.frame != first.frame || latest.display != 2 || latest.model != 1 || latest.generation != 5 {
		t.Fatal("取回的待處理快照未重用，或仍帶有舊工作資訊")
	}
	want := source.RGBAAt(3, 2)
	source.SetRGBA(3, 2, color.RGBA{})
	if latest.frame.RGBAAt(0, 0) != want {
		t.Fatal("快照仍與來源共用像素")
	}
}

func TestMLQueueReplacementRejectsIncompatibleSnapshot(t *testing.T) {
	bounds := image.Rect(0, 0, 5, 4)
	source := image.NewRGBA(bounds)
	source.SetRGBA(0, 0, color.RGBA{12, 34, 56, 255})
	tests := []struct {
		name  string
		frame *image.RGBA
	}{
		{"nil", nil},
		{"size", image.NewRGBA(image.Rect(0, 0, 6, 4))},
		{"origin", image.NewRGBA(image.Rect(1, 2, 6, 6))},
		{"stride", &image.RGBA{Pix: make([]byte, 96), Stride: 24, Rect: bounds}},
		{"short", &image.RGBA{Pix: make([]byte, 79), Stride: 20, Rect: bounds}},
		{"source", source},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := &mlWorker{jobs: make(chan mlJob, 1)}
			w.jobs <- mlJob{frame: test.frame}
			w.enqueue(source, 0, 0)
			got := (<-w.jobs).frame
			if got == test.frame || got.Rect != bounds || got.Stride != 20 || len(got.Pix) != 80 || !bytes.Equal(got.Pix, source.Pix) {
				t.Fatal("不相容的待處理快照必須配置正確且獨立的像素")
			}
		})
	}
}

func TestMLQueueSnapshotOwnershipDuringConcurrentReplacement(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for i := range source.Pix {
		source.Pix[i] = 37
	}
	w := &mlWorker{jobs: make(chan mlJob, 1)}
	w.enqueue(source, 0, 0)
	started := make(chan *image.RGBA)
	finished := make(chan bool, 1)
	go func() {
		// 假推論持續讀取已取走的快照，與主執行緒覆蓋待處理工作同時進行。
		active := (<-w.jobs).frame
		want := bytes.Clone(active.Pix)
		started <- active
		unchanged := true
		for i := 0; i < 2000; i++ {
			unchanged = bytes.Equal(active.Pix, want) && unchanged
		}
		finished <- unchanged
	}()
	active := <-started
	for i := 0; i < 2000; i++ {
		source.Pix[0] = uint8(i)
		w.enqueue(source, i, i%2)
	}
	latest := <-w.jobs
	if !<-finished || latest.frame == active || active.Pix[0] != 37 {
		t.Fatal("覆蓋待處理工作改動了推論中的影格")
	}
	if latest.display != 1999 || latest.frame.Pix[0] != source.Pix[0] {
		t.Fatal("佇列未保留最新影格")
	}
}

func BenchmarkMLQueuedSnapshot(b *testing.B) {
	for _, size := range []image.Point{{640, 360}, {1920, 1080}, {3840, 2160}} {
		for _, accepted := range []bool{false, true} {
			name := "replacement"
			if accepted {
				name = "accepted"
			}
			b.Run(fmt.Sprintf("%dx%d/%s", size.X, size.Y, name), func(b *testing.B) {
				source := image.NewRGBA(image.Rectangle{Max: size})
				w := &mlWorker{jobs: make(chan mlJob, 1), width: size.X, height: size.Y}
				if !accepted {
					w.enqueue(source, 0, 0)
				}
				b.SetBytes(int64(len(source.Pix)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					w.enqueue(source, 0, 0)
					if accepted {
						<-w.jobs
					}
				}
			})
		}
	}
}
