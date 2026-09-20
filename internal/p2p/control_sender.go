package p2p

import (
	"errors"
	"sync"
)

var errControlQueueFull = errors.New("control channel 等待佇列已滿")

// 開啟狀態、排隊與送出共用同一把鎖。OnOpen 不會略過正在排入的訊息，
// 新訊息也不能超過開啟前已接受的按鍵／命令。滿載不得移除已接受事件。
type orderedControlSender struct {
	mu      sync.Mutex
	pending [][]byte
}

func (s *orderedControlSender) send(open func() bool, write func([]byte) error, b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !open() {
		if len(s.pending) >= 64 {
			return errControlQueueFull
		}
		s.pending = append(s.pending, b)
		return nil
	}
	if err := s.drain(write); err != nil {
		return err
	}
	return write(b)
}

func (s *orderedControlSender) flush(open func() bool, write func([]byte) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !open() {
		return nil
	}
	return s.drain(write)
}

func (s *orderedControlSender) drain(write func([]byte) error) error {
	for len(s.pending) > 0 {
		if err := write(s.pending[0]); err != nil {
			return err
		}
		s.pending[0] = nil
		s.pending = s.pending[1:]
	}
	s.pending = nil
	return nil
}
