package pixelconv

import (
	"bytes"
	"testing"
)

func TestSwapOpaqueStrideAndTail(t *testing.T) {
	for _, w := range []int{1, 3, 4, 15, 16, 17, 129} {
		h, ss, ds := 3, w*4+7, w*4+11
		src := make([]byte, ss*h)
		for i := range src {
			src[i] = byte(i*31 + 7)
		}
		dst := bytes.Repeat([]byte{93}, ds*h)
		want := append([]byte(nil), dst...)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i, j := y*ss+x*4, y*ds+x*4
				want[j], want[j+1], want[j+2], want[j+3] = src[i+2], src[i+1], src[i], 255
			}
		}
		if !SwapOpaque(dst, src, w, h, ss, ds) || !bytes.Equal(dst, want) {
			t.Fatalf("寬度 %d 的 stride／尾端結果不符", w)
		}
	}
}

func TestSwapOpaqueRejectsInvalidBuffer(t *testing.T) {
	dst, src := make([]byte, 16), make([]byte, 16)
	if SwapOpaque(dst, src, 4, 2, 16, 16) || SwapOpaque(dst, src, 4, 1, 15, 16) || SwapOpaque(dst, src, 4, 2, int(^uint(0)>>1), 16) {
		t.Fatal("接受不完整或溢位緩衝區")
	}
}
