package p2p

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransportSampleTimeoutIsSingleFlight(t *testing.T) {
	var sampler transportSampler
	entered, release := make(chan struct{}), make(chan struct{})
	var count atomic.Int32
	collect := func() transportObservation {
		count.Add(1)
		close(entered)
		<-release
		return transportObservation{DTLS: "connected"}
	}
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	for i := 0; i < 4; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		_, ok := sampler.get(ctx, nil, collect)
		cancel()
		if ok {
			t.Fatal("未完成取樣被當成有效資料")
		}
	}
	<-entered
	if count.Load() != 1 {
		t.Fatalf("逾時產生 %d 個工作", count.Load())
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	obs, ok := sampler.get(ctx, nil, collect)
	if !ok || obs.DTLS != "connected" || count.Load() != 1 {
		t.Fatal("取樣恢復後未取得原工作結果")
	}
	stopped := make(chan struct{})
	close(stopped)
	if _, ok := sampler.get(context.Background(), stopped, collect); ok {
		t.Fatal("已關閉的 Peer 仍等待取樣")
	}
}

func TestMissingTransportSamplePreservesProgress(t *testing.T) {
	tracker := newLayerTracker()
	tracker.update([]LayerProgress{{ID: "L2"}})
	tracker.update([]LayerProgress{{ID: "L2", Observed: true, Sent: 100, Received: 200}})
	if result := tracker.result(); result[0].Sent != 0 || result[0].Received != 0 {
		t.Fatal("第一筆有效取樣把先前累計流量誤認為本批進展")
	}
	tracker.update([]LayerProgress{{ID: "L2", Observed: true, Sent: 120, Received: 230}})
	tracker.update([]LayerProgress{{ID: "L2"}})
	result := tracker.result()
	if result[0].Sent != 20 || result[0].Received != 30 || !result[0].Observed {
		t.Fatal("無效取樣覆寫先前進展")
	}
}
