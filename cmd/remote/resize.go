package main

import (
	_ "embed"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed resize.kage
var resizeShader []byte

// 重用 GPU 縮圖資源；只在來源像素或視窗尺寸改變時重新取樣。
type imageScaler struct {
	requestedWidth, requestedHeight          int
	changedAt                                time.Time
	pendingDirty                             bool
	shader                                   *ebiten.Shader
	stages                                   []*ebiten.Image
	sourceWidth, sourceHeight, width, height int
}

func newImageScaler() (*imageScaler, error) {
	shader, err := ebiten.NewShader(resizeShader)
	if err != nil {
		return nil, err
	}
	return &imageScaler{shader: shader}, nil
}

func (s *imageScaler) clearStages() {
	for _, stage := range s.stages {
		stage.Dispose()
	}
	s.stages = nil
}

func (s *imageScaler) Close() {
	s.clearStages()
	s.shader.Dispose()
}

// 尺寸連續變動時使用線性預覽；穩定後才配置高品質縮圖。
// 保留尚未取樣的更新，避免切回縮小模式時顯示舊影格。
func (s *imageScaler) ready(src *ebiten.Image, width, height int, dirty bool) bool {
	s.pendingDirty = s.pendingDirty || dirty
	if nativeFullscreenTransitioning() {
		s.changedAt = time.Now()
		return false
	}
	if width != s.requestedWidth || height != s.requestedHeight {
		s.requestedWidth, s.requestedHeight = width, height
		s.changedAt = time.Now()
	}
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	return width > 0 && height > 0 && width <= sw && height <= sh &&
		((width == sw && height == sh) || time.Since(s.changedAt) >= 150*time.Millisecond)
}

func (s *imageScaler) Scale(src *ebiten.Image, width, height int, dirty bool) *ebiten.Image {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	// 縮圖器只接受有效的縮小尺寸，避免非法 GPU 尺寸或不收斂的階段配置。
	if width < 1 || height < 1 || width > sw || height > sh {
		s.pendingDirty = s.pendingDirty || dirty
		return src
	}
	dirty = dirty || s.pendingDirty
	s.pendingDirty = false
	if width == sw && height == sh {
		s.pendingDirty = dirty
		return src
	}
	if sw != s.sourceWidth || sh != s.sourceHeight || width != s.width || height != s.height {
		s.clearStages()
		s.sourceWidth, s.sourceHeight, s.width, s.height = sw, sh, width, height
		// 分離水平、垂直取樣；大幅縮小時分段處理，避免截斷取樣範圍。
		for w, h := sw, sh; w != width || h != height; {
			if w != width {
				w = max(width, (w+3)/4)
				s.stages = append(s.stages, ebiten.NewImage(w, h))
			}
			if h != height {
				h = max(height, (h+3)/4)
				s.stages = append(s.stages, ebiten.NewImage(w, h))
			}
		}
		dirty = true
	}
	current := src
	for _, stage := range s.stages {
		if dirty {
			sw, sh := current.Bounds().Dx(), current.Bounds().Dy()
			dw, dh := stage.Bounds().Dx(), stage.Bounds().Dy()
			axis, ratio := []float32{1, 0}, float32(sw)/float32(dw)
			if sw == dw {
				axis, ratio = []float32{0, 1}, float32(sh)/float32(dh)
			}
			vertices := []ebiten.Vertex{
				{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0},
				{DstX: float32(dw), DstY: 0, SrcX: float32(sw), SrcY: 0},
				{DstX: 0, DstY: float32(dh), SrcX: 0, SrcY: float32(sh)},
				{DstX: float32(dw), DstY: float32(dh), SrcX: float32(sw), SrcY: float32(sh)},
			}
			options := &ebiten.DrawTrianglesShaderOptions{Blend: ebiten.BlendCopy,
				Uniforms: map[string]any{"Axis": axis, "Ratio": ratio}}
			options.Images[0] = current
			stage.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, s.shader, options)
		}
		current = stage
	}
	return current
}
