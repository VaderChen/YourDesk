package p2p

import "context"

// SendControl 的底層 Send 不接受 context。以單一工作者隔離呼叫，
// 讓監視器仍能逾時；若 Send 卡住，不再堆出新的送出 goroutine。
func boundedLivenessPing(parent context.Context, ping func(context.Context) error) func(context.Context) error {
	type request struct {
		ctx    context.Context
		result chan error
	}
	requests := make(chan request)
	go func() {
		for {
			select {
			case <-parent.Done():
				return
			case r := <-requests:
				err := r.ctx.Err()
				if err == nil {
					err = ping(r.ctx)
				}
				r.result <- err
			}
		}
	}()
	return func(ctx context.Context) error {
		r := request{ctx, make(chan error, 1)}
		select {
		case <-parent.Done():
			return parent.Err()
		case <-ctx.Done():
			return ctx.Err()
		case requests <- r:
		}
		select {
		case <-parent.Done():
			return parent.Err()
		case <-ctx.Done():
			return ctx.Err()
		case err := <-r.result:
			return err
		}
	}
}
