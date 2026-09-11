//go:build !darwin || !cgo

package superres

import (
	"errors"
	"image"
)

func loadModel(int) error                           { return errors.New("僅支援 Apple Silicon Mac") }
func predict(int, *image.RGBA) (*image.RGBA, error) { return nil, errors.New("Core ML 不可用") }
