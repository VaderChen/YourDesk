package p2p

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestLivenessRecoveryAndFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var calls, closed atomic.Int32
		go monitorLiveness(ctx, func() bool { return true }, func(context.Context) error {
			if calls.Add(1) == 3 {
				return nil
			}
			return errors.New("no response")
		}, func() { closed.Add(1) })
		time.Sleep(25 * time.Second)
		synctest.Wait()
		if closed.Load() != 0 || calls.Load() != 5 {
			t.Fatalf("未重設連續失敗 %d %d", calls.Load(), closed.Load())
		}
		time.Sleep(25 * time.Second)
		synctest.Wait()
		if closed.Load() != 1 || calls.Load() != 10 {
			t.Fatalf("未關閉失效連線 %d %d", calls.Load(), closed.Load())
		}
	})
}
func TestLivenessLegacyAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		go monitorLiveness(ctx, func() bool { return false }, func(context.Context) error { calls++; return nil }, func() { t.Error("舊版不可被誤關閉") })
		time.Sleep(time.Minute)
		synctest.Wait()
		cancel()
		synctest.Wait()
		if calls != 0 {
			t.Fatal("對舊版送出不支援的命令")
		}
	})
}

func TestLivenessUsesReceiveProgress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var flowing, closed atomic.Bool
		flowing.Store(true)
		go monitorLiveness(ctx, func() bool { return true }, func(context.Context) error { return errors.New("ping delayed") }, func() { closed.Store(true) }, func() bool { return flowing.Load() })
		time.Sleep(time.Minute)
		synctest.Wait()
		if closed.Load() {
			t.Fatal("closed while application data still arrived")
		}
		flowing.Store(false)
		time.Sleep(25 * time.Second)
		synctest.Wait()
		if closed.Load() {
			t.Fatal("closed before receive grace")
		}
		time.Sleep(5 * time.Second)
		synctest.Wait()
		if !closed.Load() {
			t.Fatal("silent dead connection not closed")
		}
	})
}

// 持續收到零星資料，也不能讓失效控制通道永遠維持 connected。
func TestLivenessReceiveProgressHasLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var closed atomic.Bool
		go monitorLiveness(ctx, func() bool { return true }, func(context.Context) error { return errors.New("no heartbeat") }, func() { closed.Store(true) }, func() bool { return true })
		time.Sleep(94 * time.Second)
		synctest.Wait()
		if closed.Load() {
			t.Fatal("closed before heartbeat grace expired")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if !closed.Load() {
			t.Fatal("receive traffic masked dead control channel")
		}
	})
}

func TestLivenessHealthyHeartbeatWithoutTraffic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go monitorLiveness(ctx, func() bool { return true }, func(context.Context) error { return nil }, func() { t.Error("idle healthy session closed") }, func() bool { return false })
		time.Sleep(time.Hour)
		synctest.Wait()
	})
}

func TestLivenessBlockedSendStillTimesOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		release := make(chan struct{})
		var calls atomic.Int32
		var closed atomic.Bool
		go monitorLiveness(ctx, func() bool { return true }, func(context.Context) error {
			calls.Add(1)
			<-release // 模擬底層 Send 忽略 context。
			return nil
		}, func() { closed.Store(true) })
		time.Sleep(45 * time.Second)
		synctest.Wait()
		if !closed.Load() || calls.Load() != 1 {
			t.Fatalf("closed=%v calls=%d", closed.Load(), calls.Load())
		}
		close(release)
		synctest.Wait()
	})
}
