package p2p

import (
	"bytes"
	"testing"
)

func TestAudioQueueBoundedCopiesAndKeepsNewest(t *testing.T) {
	p := &Peer{audioInbox: make(chan []byte, 8)}
	for i := 1; i <= 30; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 40)
		p.enqueueAudio(data)
		data[0] = 0
	}
	if len(p.audioInbox) != 8 {
		t.Fatal("音訊佇列必須有上限")
	}
	for i := 23; i <= 30; i++ {
		if data := p.ReadAudioPacket(); len(data) != 40 || data[0] != byte(i) {
			t.Fatal("只保留最新封包且不得引用呼叫端記憶體")
		}
	}
	for _, size := range []int{0, 24, 8217} {
		p.enqueueAudio(make([]byte, size))
	}
	if p.ReadAudioPacket() != nil {
		t.Fatal("錯誤大小不得進入佇列")
	}
}
