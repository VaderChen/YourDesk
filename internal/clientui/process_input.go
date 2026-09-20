package clientui

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

const processInputCapacity = 16
const processInputTimeout = 10 * time.Second

var errProcessInputBusy = errors.New("子程序控制佇列已滿，請稍後重試")

type processInputMessage struct {
	ctx    context.Context
	cancel context.CancelFunc
	data   []byte
	result chan error
}

// 每個子程序只有一個寫入者。Write 只入列，不能在 server.mu 內等待管線；
// Send 供需要確認傳送的呼叫者在鎖外等待。取消正在寫入的訊息時關閉管線，
// 避免半筆 JSON 與後續命令拼接；os/exec 的 StdinPipe.Close 會解除阻塞寫入。
type processInput struct {
	mu      sync.Mutex
	closed  bool
	pipe    io.WriteCloser
	queue   chan processInputMessage
	done    chan struct{}
	stopped chan struct{}
}

func newProcessInput(pipe io.WriteCloser) *processInput {
	w := &processInput{pipe: pipe, queue: make(chan processInputMessage, processInputCapacity), done: make(chan struct{}), stopped: make(chan struct{})}
	go w.run()
	return w
}

func (w *processInput) enqueue(ctx context.Context, data []byte) (<-chan error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(data) > 65536 {
		return nil, errors.New("子程序控制訊息過長")
	}
	ctx, cancel := context.WithTimeout(ctx, processInputTimeout)
	m := processInputMessage{ctx: ctx, cancel: cancel, data: append([]byte(nil), data...), result: make(chan error, 1)}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		cancel()
		return nil, io.ErrClosedPipe
	}
	select {
	case w.queue <- m:
		return m.result, nil
	default:
		cancel()
		return nil, errProcessInputBusy
	}
}

func (w *processInput) Write(data []byte) (int, error) {
	_, err := w.enqueue(context.Background(), data)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func (w *processInput) Send(ctx context.Context, data []byte) error {
	result, err := w.enqueue(ctx, data)
	if err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-w.done:
		return io.ErrClosedPipe
	}
}

func (w *processInput) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	close(w.done)
	w.mu.Unlock()
	return w.pipe.Close()
}

func (w *processInput) run() {
	defer close(w.stopped)
	defer func() {
		for {
			select {
			case m := <-w.queue:
				m.result <- io.ErrClosedPipe
				m.cancel()
			default:
				return
			}
		}
	}()
	for {
		select {
		case <-w.done:
			return
		case m := <-w.queue:
			if err := m.ctx.Err(); err != nil {
				m.result <- err
				m.cancel()
				continue
			}
			cancelled := make(chan struct{})
			stop := context.AfterFunc(m.ctx, func() { _ = w.Close(); close(cancelled) })
			n, err := w.pipe.Write(m.data)
			if !stop() {
				<-cancelled
			}
			if err == nil && n != len(m.data) {
				err = io.ErrShortWrite
			}
			if contextErr := m.ctx.Err(); contextErr != nil {
				err = contextErr
			}
			m.cancel()
			m.result <- err
			if err != nil {
				_ = w.Close()
				return
			}
		}
	}
}
