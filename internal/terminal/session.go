// Package terminal 提供經 P2P 驗證、隨連線回收的互動終端機。
package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
	"yourdesk/internal/p2p"
)

type console interface {
	io.ReadWriteCloser
	Resize(int, int) error
	Wait() error
}
type request struct {
	Columns  int    `json:"columns"`
	Rows     int    `json:"rows"`
	Data     []byte `json:"data"`
	Sequence uint64 `json:"sequence"`
	Ack      uint64 `json:"ack"`
}
type session struct {
	mu                sync.Mutex
	space             *sync.Cond
	stopped           bool
	tty               console
	buffer, pending   []byte
	sequence, written uint64
	ended             bool
	failure           string
	input             chan []byte
	done              chan struct{}
	once              sync.Once
}

func (s *session) close() {
	s.once.Do(func() {
		s.mu.Lock()
		s.stopped = true
		s.ended = true
		s.space.Broadcast()
		s.mu.Unlock()
		close(s.done)
		s.tty.Close()
	})
}
func (s *session) read() {
	b := make([]byte, 4096)
	for {
		n, e := s.tty.Read(b)
		s.mu.Lock()
		for len(s.buffer)+n > 256*1024 && !s.stopped {
			s.space.Wait()
		}
		if s.stopped {
			s.mu.Unlock()
			return
		}
		s.buffer = append(s.buffer, b[:n]...)
		if e != nil {
			s.ended = true
		}
		s.mu.Unlock()
		if e != nil {
			s.close()
			return
		}
	}
}
func Available() bool { return os.Geteuid() != 0 && supported() }

func Register(peer *p2p.Peer, authorized func() bool, active func(bool)) {
	if !Available() {
		return
	}
	var mu sync.Mutex
	var current *session
	closed := false
	go func() {
		<-peer.Done()
		mu.Lock()
		closed = true
		if current != nil {
			current.close()
		}
		mu.Unlock()
	}()
	for _, action := range []string{"open", "read", "write", "resize", "close"} {
		action := action
		_ = peer.RegisterCommandParams("terminal."+action, func(ctx context.Context, raw json.RawMessage) (any, error) {
			if !authorized() || ctx.Err() != nil {
				return nil, errors.New("遠端連線已結束")
			}
			var in request
			if len(raw) > 0 && json.Unmarshal(raw, &in) != nil {
				return nil, errors.New("終端機參數無效")
			}
			mu.Lock()
			defer mu.Unlock()
			if closed || !authorized() || ctx.Err() != nil {
				return nil, errors.New("遠端連線已結束")
			}
			if action == "open" {
				if current != nil {
					return map[string]bool{"ok": true}, nil
				}
				if in.Columns < 20 || in.Columns > 500 || in.Rows < 5 || in.Rows > 300 {
					return nil, errors.New("終端機大小無效")
				}
				tty, e := start(in.Columns, in.Rows)
				if e != nil {
					return nil, e
				}
				current = &session{tty: tty, input: make(chan []byte, 16), done: make(chan struct{})}
				s := current
				s.space = sync.NewCond(&s.mu)
				active(true)
				go s.read()
				go func() {
					if err := tty.Wait(); err != nil {
						s.mu.Lock()
						if !s.stopped {
							s.failure = "命令列無法繼續執行，請確認遠端 Shell 狀態。"
						}
						s.mu.Unlock()
					}
				}()
				go func() {
					for {
						select {
						case <-s.done:
							return
						case b := <-s.input:
							if _, e := tty.Write(b); e != nil {
								s.mu.Lock()
								s.failure = "終端機輸入已中斷。"
								s.ended = true
								s.mu.Unlock()
								s.close()
								return
							}
						}
					}
				}()
				return map[string]bool{"ok": true}, nil
			}
			s := current
			if s == nil {
				return nil, errors.New("請先開啟終端機")
			}
			if action == "close" {
				s.close()
				return map[string]bool{"ok": true}, nil
			}
			if action == "resize" {
				if in.Columns < 20 || in.Columns > 500 || in.Rows < 5 || in.Rows > 300 {
					return nil, errors.New("終端機大小無效")
				}
				return map[string]bool{"ok": true}, s.tty.Resize(in.Columns, in.Rows)
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if action == "write" {
				if len(in.Data) > 2048 || in.Sequence == 0 || in.Sequence > s.written+1 {
					return nil, errors.New("終端機輸入序號無效")
				}
				if in.Sequence <= s.written {
					return map[string]bool{"ok": true}, nil
				}
				if s.ended {
					return nil, errors.New("Shell 已結束")
				}
				select {
				case s.input <- append([]byte(nil), in.Data...):
					s.written = in.Sequence
					return map[string]bool{"ok": true}, nil
				default:
					return nil, errors.New("輸入過快，請重新連線。")
				}
			}
			if in.Ack > s.sequence {
				return nil, errors.New("終端機輸出序號無效")
			}
			if in.Ack == s.sequence {
				s.pending = nil
			}
			if len(s.pending) == 0 && len(s.buffer) > 0 {
				n := min(len(s.buffer), 4096)
				s.pending = append([]byte(nil), s.buffer[:n]...)
				s.buffer = s.buffer[n:]
				s.space.Broadcast()
				s.sequence++
			}
			return map[string]any{"sequence": s.sequence, "data": s.pending, "ended": s.ended && len(s.buffer) == 0, "error": s.failure}, nil
		})
	}
}
