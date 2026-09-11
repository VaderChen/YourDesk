package streampipeline

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

func TestOrderedDrainAndSkip(t *testing.T) {
	index, sent := 0, 0
	err := Run(context.Background(), Stages[int, int]{
		Capture: func(context.Context) (int, error) {
			if index == 100 {
				return 0, io.EOF
			}
			index++
			if index%2 == 0 {
				return 0, Skip
			}
			return index, nil
		},
		Encode: func(_ context.Context, n int) (int, error) { return n, nil },
		Send: func(_ context.Context, n int) error {
			if n != sent*2+1 {
				return errors.New("影格順序錯誤")
			}
			sent++
			return nil
		},
	})
	if err != nil || sent != 50 {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
}

func TestSendFailureJoinsWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	want := errors.New("傳送失敗")
	var active atomic.Int32
	err := Run(ctx, Stages[int, int]{
		Capture: func(ctx context.Context) (int, error) {
			active.Add(1)
			defer active.Add(-1)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			default:
				return 1, nil
			}
		},
		Encode: func(_ context.Context, n int) (int, error) { return n, nil },
		Send:   func(context.Context, int) error { return want },
	})
	if !errors.Is(err, want) || active.Load() != 0 {
		t.Fatalf("workers=%d err=%v", active.Load(), err)
	}
}

func TestDoubleBufferBounds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var captured, encoded atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Stages[int, int]{
			Capture: func(context.Context) (int, error) { return int(captured.Add(1)), nil },
			Encode:  func(_ context.Context, n int) (int, error) { encoded.Add(1); return n, nil },
			Send: func(ctx context.Context, _ int) error {
				close(entered)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-release:
					return errors.New("停止")
				}
			},
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("傳送階段未啟動")
	}
	// 慢速傳送時，一個傳送中、一個已編碼，另有最多兩個原始影格。
	time.Sleep(20 * time.Millisecond)
	if captured.Load() > 4 || encoded.Load() > 2 {
		t.Errorf("capture=%d encode=%d", captured.Load(), encoded.Load())
	}
	close(release)
	<-done
}
