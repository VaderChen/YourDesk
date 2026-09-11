package streampipeline

import "context"

// Consumer 將回呼來源與單一處理 worker 分離，以兩個槽位交接所有權。
// 槽位包含正在處理的項目；來源自行組裝中的資料不計入交接槽位。
// 不覆寫或丟棄已排隊的相依資料，滿載時由 Submit 反壓來源。
type Consumer[T any] struct {
	ctx    context.Context
	cancel context.CancelFunc
	free   chan *T
	ready  chan *T
	done   chan struct{}
}

func NewConsumer[T any](parent context.Context, consume func(T)) *Consumer[T] {
	ctx, cancel := context.WithCancel(parent)
	c := &Consumer[T]{ctx: ctx, cancel: cancel, free: make(chan *T, 2), ready: make(chan *T, 2), done: make(chan struct{})}
	for i := 0; i < 2; i++ {
		c.free <- new(T)
	}
	go func() {
		defer close(c.done)
		for {
			select {
			case <-ctx.Done():
				return
			case slot := <-c.ready:
				if ctx.Err() == nil {
					consume(*slot)
				}
				var zero T
				*slot = zero
				c.free <- slot
			}
		}
	}()
	return c
}

// Submit 成功後呼叫端不得修改 value 引用的記憶體。
// 同一來源須依序呼叫，以維持資料順序；取消時解除反壓。
func (c *Consumer[T]) Submit(value T) bool {
	if c.ctx.Err() != nil {
		return false
	}
	var slot *T
	select {
	case <-c.ctx.Done():
		return false
	case slot = <-c.free:
	}
	*slot = value
	select {
	case <-c.ctx.Done():
		var zero T
		*slot = zero
		c.free <- slot
		return false
	case c.ready <- slot:
		return true
	}
}

// Close 停止接收並等待當前處理完成，之後才可釋放原生解碼資源。
// 不關閉輸入 channel，避免與尚未返回的來源回呼競爭。可重複呼叫。
func (c *Consumer[T]) Close() {
	c.cancel()
	<-c.done
}
