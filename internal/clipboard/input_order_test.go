package clipboard

import (
	"context"
	"reflect"
	"testing"
	"yourdesk/internal/p2p"
	"yourdesk/internal/rawkey"
)

// 在送出工作者尚未執行的情境下，滑鼠仍須等待前面的修飾鍵，不得走旁路。
func TestModifierPointerQueueOrder(t *testing.T) {
	s := &Sync{ctx: context.Background(), peer: &p2p.Peer{}, keys: make(chan keyJob, 4096)}
	events := []p2p.Control{
		{Type: "raw-key", RawKey: &rawkey.Event{Platform: "darwin", Code: 56, Modifiers: 1, Down: true}},
		{Type: "button", Button: 1, Down: true, X: .2, Y: .3},
		{Type: "move", X: .3, Y: .4},
		{Type: "wheel", Delta: 1},
		{Type: "button", Button: 1, Down: false, X: .3, Y: .4},
		{Type: "raw-key", RawKey: &rawkey.Event{Platform: "darwin", Code: 56, Down: false}},
	}
	for _, e := range events {
		if err := s.SendControl(e); err != nil {
			t.Fatal(err)
		}
	}
	for _, expected := range events {
		select {
		case job := <-s.keys:
			if !reflect.DeepEqual(job.control, expected) {
				t.Fatalf("輸入順序或內容變更：%+v", job.control)
			}
		default:
			t.Fatal("滑鼠未經有序交接")
		}
	}
	for i := 0; i < 5000; i++ {
		if err := s.SendControl(p2p.Control{Type: "move", X: .5}); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.keys) > 64 {
		t.Fatal("移動事件持續累積")
	}
	if err := s.SendControl(p2p.Control{Type: "button", Button: 1, Down: false}); err != nil {
		t.Fatal("移動事件排擠滑鼠釋放")
	}
	if err := s.SendControl(p2p.Control{Type: "key", Key: "shift", Down: false}); err != nil {
		t.Fatal("移動事件排擠修飾鍵釋放")
	}
}
