package video

import (
	"image"
	"sync"
)

// IntraDecoder 讓平台保留解碼工作階段，避免每張影格重新建立 COM／D3D 資源。
type IntraDecoder interface {
	Decode([]byte) (image.Image, error)
	Close() error
}
type BackendReporter interface{ Backend() string }
type DecodeSession struct {
	mu      sync.Mutex
	decoder IntraDecoder
	codec   WireCodec
}

func (s *DecodeSession) Decode(codec WireCodec, payload []byte) (image.Image, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, err := IsKeyframe(codec, payload)
	if err != nil {
		return nil, err
	}
	if (s.decoder == nil || s.codec != codec) && !key {
		return nil, ErrNeedKeyframe
	}
	if s.decoder == nil || s.codec != codec {
		if s.decoder != nil {
			s.decoder.Close()
			s.decoder = nil
		}
		var err error
		s.decoder, err = newIntraDecoder(codec)
		if err != nil {
			return nil, err
		}
		s.codec = codec
	}
	img, err := s.decoder.Decode(payload)
	if err != nil {
		s.decoder.Close()
		s.decoder = nil
	}
	return img, err
}
func (s *DecodeSession) Backend() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value, ok := s.decoder.(BackendReporter); ok {
		return value.Backend()
	}
	return "VideoToolbox 自動選擇解碼"
}
func (s *DecodeSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.decoder != nil {
		s.decoder.Close()
		s.decoder = nil
	}
}

// 解碼模式只取自目前成功使用的工作階段。
func (s *DecodeSession) DecodingMode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.decoder.(interface{ DecodingMode() string }); ok {
		return d.DecodingMode()
	}
	return "unknown"
}
