// Package streampipeline 以明確所有權串接擷取、編碼、傳送三個階段。
package streampipeline

import (
	"context"
	"errors"
	"io"
	"sync"
)

// Skip 表示本輪沒有可送出的影格；EOF 則結束擷取並送完已接收的工作。
var Skip = errors.New("本輪沒有影格")

type Stages[Raw, Encoded any] struct {
	Capture func(context.Context) (Raw, error)
	Encode  func(context.Context, Raw) (Encoded, error)
	Send    func(context.Context, Encoded) error
}

// Run 每個交接點僅有兩個槽位，包含正在處理的工作，沒有額外無界佇列。
// Raw 在 Encode 返回前不得被擷取端重用；Encoded 在 Send 返回前不得重用。
// 各階段只有一個 worker，保留編碼參考狀態及傳送順序。背壓只暫停上游，
// 不任意丟棄已編碼的相依影格。返回前等待全部 worker，才能安全釋放原生資源。
func Run[Raw, Encoded any](parent context.Context, s Stages[Raw, Encoded]) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	rawFree, rawReady := make(chan *Raw, 2), make(chan *Raw, 2)
	encodedFree, encodedReady := make(chan *Encoded, 2), make(chan *Encoded, 2)
	for i := 0; i < 2; i++ {
		rawFree <- new(Raw)
		encodedFree <- new(Encoded)
	}
	var wg sync.WaitGroup
	var once sync.Once
	var result error
	fail := func(err error) { once.Do(func() { result = err; cancel() }) }
	wg.Add(3)
	go func() {
		defer wg.Done()
		defer close(rawReady)
		for {
			var slot *Raw
			select {
			case <-ctx.Done():
				return
			case slot = <-rawFree:
			}
			value, err := s.Capture(ctx)
			if errors.Is(err, io.EOF) {
				return
			}
			if errors.Is(err, Skip) {
				rawFree <- slot
				continue
			}
			if err != nil {
				fail(err)
				return
			}
			*slot = value
			select {
			case <-ctx.Done():
				return
			case rawReady <- slot:
			}
		}
	}()
	go func() {
		defer wg.Done()
		defer close(encodedReady)
		for {
			var raw *Raw
			var ok bool
			select {
			case <-ctx.Done():
				return
			case raw, ok = <-rawReady:
				if !ok {
					return
				}
			}
			var slot *Encoded
			select {
			case <-ctx.Done():
				return
			case slot = <-encodedFree:
			}
			value, err := s.Encode(ctx, *raw)
			var zero Raw
			*raw = zero
			rawFree <- raw
			if errors.Is(err, Skip) {
				encodedFree <- slot
				continue
			}
			if err != nil {
				fail(err)
				return
			}
			*slot = value
			select {
			case <-ctx.Done():
				return
			case encodedReady <- slot:
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			var slot *Encoded
			var ok bool
			select {
			case <-ctx.Done():
				return
			case slot, ok = <-encodedReady:
				if !ok {
					return
				}
			}
			err := s.Send(ctx, *slot)
			var zero Encoded
			*slot = zero
			encodedFree <- slot
			if err != nil {
				fail(err)
				return
			}
		}
	}()
	wg.Wait()
	if result != nil {
		return result
	}
	return parent.Err()
}
