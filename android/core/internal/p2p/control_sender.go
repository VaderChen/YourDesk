package p2p

import (
	"errors"
	"sync"
)

var errControlQueueFull = errors.New("control channel 等待佇列已滿")

// State check, enqueue and writes share a lock: OnOpen cannot miss an
// enqueue, and a new key-up cannot overtake an accepted key-down.
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
