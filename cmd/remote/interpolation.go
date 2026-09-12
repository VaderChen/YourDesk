package main

import (
	"context"
	"fmt"
	"image"
	"image/draw"

	xdraw "golang.org/x/image/draw"
	"sync/atomic"
	"time"
	"yourdesk/internal/frameinterp"
)

var viewerInterpolation atomic.Bool
var viewerInterpolationMethod atomic.Int32

type interpolationResult struct {
	frame      *image.RGBA
	generation uint64
	reason     string
}
type frameInterpolator struct {
	method                  string
	sourceBounds            image.Rectangle
	generated               bool
	previous, shown, target *image.RGBA
	queued                  *image.RGBA
	queuedAt                time.Time
	period                  time.Duration
	synthesizing            bool
	last, deadline          time.Time
	display                 int
	generation              uint64
	cancel                  context.CancelFunc
	results                 chan interpolationResult
	busy                    atomic.Bool
	status                  string
}

func (p *frameInterpolator) reset() {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.generation++
	p.sourceBounds = image.Rectangle{}
	p.previous = nil
	p.shown = nil
	p.target = nil
	p.queued = nil
	p.synthesizing = false
	p.last = time.Time{}
	p.generated = false
	p.status = "等待影格"
}
func snapshotRGBA(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

// Apple 的真實幀、生成幀與回退共用工作尺寸，最後統一交給顯示縮放。
func interpolationSnapshot(src *image.RGBA, method string) *image.RGBA {
	if method != "apple" {
		return snapshotRGBA(src)
	}
	w, h := frameinterp.AppleWorkingSize(src.Bounds().Dx(), src.Bounds().Dy())
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

// 先顯示中間幀，再於半個影格週期後顯示新真實幀；逾期結果不採用。
// 使用接收端時間，不改動來源 FPS 與舊版線上協定。
func (p *frameInterpolator) frame(src *image.RGBA, display int, dirty, enabled bool) (*image.RGBA, bool) {
	// 未啟用補幀時不在每次繪圖查詢原生／模型能力。
	if !enabled || src == nil {
		if p.previous != nil || p.target != nil || p.queued != nil {
			p.reset()
			dirty = true
		}
		p.status = "尚未啟用"
		return src, dirty
	}
	method := frameinterp.ResolveMethod([]string{"", "apple", "rife"}[viewerInterpolationMethod.Load()])
	if method != p.method {
		p.reset()
		p.method = method
		dirty = true
	}
	if ok, reason := frameinterp.MethodStatus(method); !ok {
		if p.previous != nil {
			p.reset()
			dirty = true
		}
		p.status = reason
		return src, dirty
	}
	if p.results == nil {
		p.results = make(chan interpolationResult, 1)
	}
	now := time.Now()
	old := p.shown
	if dirty || p.previous == nil {
		if p.previous != nil && (p.sourceBounds != src.Bounds() || display != p.display) {
			p.reset()
		}
		p.display = display
		p.sourceBounds = src.Bounds()
		p.queued = interpolationSnapshot(src, method)
		p.queuedAt = now
	}
	// 先接收既有工作的結果，不能被同一次 Draw 到達的新來源幀取消。
	select {
	case r := <-p.results:
		if r.generation == p.generation && p.target != nil && p.synthesizing && now.Before(p.deadline) {
			p.synthesizing = false
			if r.frame != nil {
				p.shown = r.frame
				p.generated = true
				p.deadline = now.Add(p.period / 2)
				p.status = interpolationLabel(method)
			} else {
				p.shown = p.target
				p.target = nil
				p.generated = false
				p.status = "補幀回退，顯示真實影格"
				if r.reason != "" {
					p.status += "：" + r.reason
				}
			}
		}
	default:
	}
	if p.target != nil && !now.Before(p.deadline) {
		p.shown = p.target
		p.target = nil
		if p.synthesizing {
			p.generation++
			if p.cancel != nil {
				p.cancel()
			}
			p.generated = false
			p.status = "補幀逾時，顯示真實影格"
		}
		p.synthesizing = false
	}
	if p.target == nil && p.queued != nil {
		next, arrival := p.queued, p.queuedAt
		p.queued = nil
		interval := arrival.Sub(p.last)
		p.last = arrival
		if p.previous == nil || interval < 20*time.Millisecond || interval > 150*time.Millisecond || p.busy.Load() {
			p.shown = next
			p.status = "等待穩定影格"
		} else {
			p.generation++
			p.target = next
			p.period = interval
			p.synthesizing = true
			// 計算期限和中間幀呈現時間分開；最多等待一個來源週期、上限 50 ms。
			budget := min(interval, 50*time.Millisecond)
			p.deadline = now.Add(budget)
			ctx, cancel := context.WithDeadline(context.Background(), p.deadline)
			p.cancel = cancel
			a, b, id := p.previous, next, p.generation
			p.busy.Store(true)
			if !p.generated {
				p.status = "準備補幀"
			} else {
				p.status = interpolationLabel(method)
			}
			go func() {
				defer p.busy.Store(false)
				defer cancel()
				mid, err := motionMidpoint(ctx, a, b, method)
				if ctx.Err() != nil {
					mid = nil
					err = ctx.Err()
				}
				select {
				case p.results <- interpolationResult{frame: mid, generation: id, reason: interpolationError(err)}:
				default:
				}
			}()
		}
		p.previous = next
	}
	if p.target == nil && now.Sub(p.last) > 150*time.Millisecond {
		p.status = "等待穩定影格"
	}
	return p.shown, p.shown != old
}

// 場景跳變直接回退；中間光流與遮擋由 RIFE 估計。
func motionMidpoint(ctx context.Context, a, b *image.RGBA, method string) (*image.RGBA, error) {
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	delta, count := 0, 0
	var histogramA, histogramB [48]int
	for y := 0; y < h; y += 32 {
		for x := 0; x < w; x += 32 {
			ai, bi := y*a.Stride+x*4, y*b.Stride+x*4
			for c := 0; c < 3; c++ {
				delta += absInt(int(a.Pix[ai+c]) - int(b.Pix[bi+c]))
				histogramA[c*16+int(a.Pix[ai+c])/16]++
				histogramB[c*16+int(b.Pix[bi+c])/16]++
				count++
			}
		}
	}
	histogramDistance := 0
	for i := range histogramA {
		histogramDistance += absInt(histogramA[i] - histogramB[i])
	}
	// 大幅平移會增加逐像素差異，但整體色彩分布通常維持；兩者都變化才判定切場。
	if count == 0 || (delta/count > 45 && float64(histogramDistance) > 1.3*float64(count)) {
		return nil, fmt.Errorf("畫面變化過大")
	}
	out, err := frameinterp.MidpointWithMethod(ctx, a, b, method)
	return out, err
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func interpolationLabel(method string) string {
	if method == "apple" {
		return "2× 補幀（Apple 低延遲）"
	}
	return "2× 補幀（RIFE 4.25 Lite）"
}

func interpolationError(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
