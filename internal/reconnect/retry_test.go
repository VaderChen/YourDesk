package reconnect

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestThreeFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		count := 0
		failure := errors.New("offline")
		err := Run(context.Background(), func(context.Context) error { count++; return failure })
		if count != 3 || !errors.Is(err, failure) {
			t.Fatalf("attempts=%d err=%v", count, err)
		}
	})
}
func TestSuccessStopsRetries(t *testing.T) {
	for success := 1; success <= 3; success++ {
		synctest.Test(t, func(t *testing.T) {
			count := 0
			err := Run(context.Background(), func(context.Context) error {
				count++
				if count == success {
					return nil
				}
				return errors.New("offline")
			})
			if err != nil || count != success {
				t.Fatalf("attempts=%d err=%v", count, err)
			}
		})
	}
}
func TestCancelDuringDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(time.Second, cancel)
		err := Run(ctx, func(context.Context) error { t.Fatal("attempt after cancellation"); return nil })
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}
func TestAttemptTimeoutAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		count := 0
		start := time.Now()
		err := Run(context.Background(), func(ctx context.Context) error { count++; <-ctx.Done(); return ctx.Err() })
		if count != 3 || !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 105*time.Second {
			t.Fatalf("attempts=%d elapsed=%v err=%v", count, time.Since(start), err)
		}
	})
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		count := 0
		time.AfterFunc(3*time.Second, cancel)
		err := Run(ctx, func(ctx context.Context) error { count++; <-ctx.Done(); return ctx.Err() })
		if count != 1 || !errors.Is(err, context.Canceled) {
			t.Fatalf("attempts=%d err=%v", count, err)
		}
	})
}
