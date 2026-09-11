package main

import (
	_ "embed"
	"github.com/hajimehoshi/ebiten/v2"
	"time"
)

//go:embed fsr_easu.kage
var fsrEASU []byte

//go:embed fsr_rcas.kage
var fsrRCAS []byte

// FSR 1 的 EASU／RCAS 移植；僅在新影格或尺寸穩定後重算。
type fsrScaler struct {
	easu, rcas            *ebiten.Shader
	intermediate, output  *ebiten.Image
	width, height, sw, sh int
	requested             [2]int
	changed               time.Time
	pending               bool
}

func newFSRScaler() (*fsrScaler, error) {
	a, err := ebiten.NewShader(fsrEASU)
	if err != nil {
		return nil, err
	}
	b, err := ebiten.NewShader(fsrRCAS)
	if err != nil {
		a.Dispose()
		return nil, err
	}
	return &fsrScaler{easu: a, rcas: b}, nil
}
func (s *fsrScaler) reset() {
	if s.intermediate != nil {
		s.intermediate.Dispose()
		s.output.Dispose()
	}
	s.intermediate, s.output = nil, nil
	s.width, s.height = 0, 0
	s.pending = true
}
func (s *fsrScaler) Close() { s.reset(); s.easu.Dispose(); s.rcas.Dispose() }
func (s *fsrScaler) Scale(src *ebiten.Image, w, h int, dirty bool) *ebiten.Image {
	s.pending = s.pending || dirty
	if w < 1 || h < 1 || w > 8192 || h > 8192 {
		return src
	}
	if s.requested != [2]int{w, h} {
		s.requested = [2]int{w, h}
		s.changed = time.Now()
	}
	if time.Since(s.changed) < 150*time.Millisecond {
		return src
	}
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if s.width != w || s.height != h || s.sw != sw || s.sh != sh {
		s.reset()
		s.width, s.height, s.sw, s.sh = w, h, sw, sh
		s.intermediate = ebiten.NewImage(w, h)
		s.output = ebiten.NewImage(w, h)
	}
	if s.pending {
		vertices := []ebiten.Vertex{
			{DstX: 0, DstY: 0}, {DstX: float32(w), DstY: 0},
			{DstX: 0, DstY: float32(h)}, {DstX: float32(w), DstY: float32(h)},
		}
		a := &ebiten.DrawTrianglesShaderOptions{Uniforms: map[string]any{"OutputSize": []float32{float32(w), float32(h)}}}
		a.Images[0] = src
		s.intermediate.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, s.easu, a)
		b := &ebiten.DrawRectShaderOptions{}
		b.Images[0] = s.intermediate
		s.output.DrawRectShader(w, h, s.rcas, b)
		s.pending = false
	}
	return s.output
}
