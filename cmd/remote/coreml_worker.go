package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/draw"
	"log/slog"
	"sync/atomic"
	"time"
	"yourdesk/internal/superres"
)

var viewerSuperResolution atomic.Int32

// 單一背景推論、最多一張待處理影格；不累積延遲，也不在 UI 執行 Core ML。
type mlJob struct {
	model               int
	frame               *image.RGBA
	display, generation int
}
type mlResult struct {
	frame                              *image.RGBA
	display, generation, width, height int
	err                                error
}
type mlWorker struct {
	frame                  *image.RGBA
	revision               uint64
	initialized            bool
	jobs                   chan mlJob
	results                chan mlResult
	done                   chan struct{}
	generation             int
	texture                *ebiten.Image
	width, height, display int
	reason                 string
	retryAt                time.Time
	validUntil             time.Time
}

func newMLWorker() *mlWorker {
	w := &mlWorker{jobs: make(chan mlJob, 1), results: make(chan mlResult, 1), done: make(chan struct{})}
	go func() {
		for {
			select {
			case <-w.done:
				return
			case job := <-w.jobs:
				result, err := superres.Enhance(job.model, job.frame)
				r := mlResult{result, job.display, job.generation, job.frame.Bounds().Dx(), job.frame.Bounds().Dy(), err}
				select {
				case <-w.results:
				default:
				}
				select {
				case w.results <- r:
				case <-w.done:
					return
				}
			}
		}
	}()
	return w
}
func (w *mlWorker) reset() {
	w.generation++
	w.initialized = false
	w.frame = nil
	if w.texture != nil {
		w.texture.Dispose()
		w.texture = nil
	}
	select {
	case <-w.jobs:
	default:
	}
	w.reason = ""
	w.retryAt = time.Time{}
}
func (w *mlWorker) Close() { close(w.done); w.reset() }
func (w *mlWorker) submit(frame *image.RGBA, display, model int) {
	if w.width != frame.Bounds().Dx() || w.height != frame.Bounds().Dy() || w.display != display {
		w.reset()
		w.width, w.height, w.display = frame.Bounds().Dx(), frame.Bounds().Dy(), display
	}
	if !superres.Ready(model) || time.Now().Before(w.retryAt) {
		return
	}
	w.initialized = true
	// 以最新快照替換尚未開始的工作。
	select {
	case <-w.jobs:
	default:
	}
	copy := image.NewRGBA(image.Rect(0, 0, w.width, w.height))
	draw.Draw(copy, copy.Bounds(), frame, frame.Bounds().Min, draw.Src)
	select {
	case w.jobs <- mlJob{model, copy, display, w.generation}:
	default:
	}
}
func (w *mlWorker) current() *ebiten.Image {
	select {
	case result := <-w.results:
		if result.generation == w.generation && result.display == w.display && result.width == w.width && result.height == w.height {
			if result.err != nil {
				slog.Warn("Core ML 超解析度回退", "error", result.err)
				w.generation++
				select {
				case <-w.jobs:
				default:
				}
				w.reason = "Core ML 推論失敗，回退 FSR 1"
				w.retryAt = time.Now().Add(10 * time.Second)
				if w.texture != nil {
					w.texture.Dispose()
					w.texture = nil
				}
			} else {
				if w.texture == nil || w.texture.Bounds() != result.frame.Bounds() {
					if w.texture != nil {
						w.texture.Dispose()
					}
					w.texture = ebiten.NewImage(result.frame.Bounds().Dx(), result.frame.Bounds().Dy())
				}
				w.frame = result.frame
				w.texture.WritePixels(result.frame.Pix)
				w.revision++
				w.reason = ""
				w.validUntil = time.Now().Add(2 * time.Second)
			}
		}
	default:
	}
	if time.Now().After(w.validUntil) {
		return nil
	}
	return w.texture
}
