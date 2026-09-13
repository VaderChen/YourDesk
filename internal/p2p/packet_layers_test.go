package p2p

import (
	"testing"
	"testing/synctest"
	"time"
)

func TestLayerProgressTracksDirections(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tracker := newLayerTracker()
		tracker.update([]LayerProgress{{ID: "L2", Sent: 10, Received: 20, Observed: true}})
		time.Sleep(time.Second)
		tracker.update([]LayerProgress{{ID: "L2", Sent: 15, Received: 20, Observed: true}})
		time.Sleep(time.Second)
		r := tracker.result()[0]
		if r.Sent != 5 || r.Received != 0 || r.TXIdleMS != 1000 || r.RXIdleMS != 2000 {
			t.Fatalf("incorrect directional progress: %+v", r)
		}
	})
}
