package video

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestAV1WireSmoke(t *testing.T) {
	key, err := IsKeyframe(WireAV1, probeAV1)
	if err != nil || !key {
		t.Fatalf("AV1 固定影格無法識別：%v %v", key, err)
	}
	var sequence []byte
	if _, err := packAV1(probeAV1, nil, &sequence); err != nil || len(sequence) == 0 {
		t.Fatalf("sequence header 未保存：%v", err)
	}
	// 非 key frame 必須保有 sequence，且不能因全畫面更新誤判為 key frame。
	delta := []byte{0x32, 1, 0x20}
	payload, err := packAV1(delta, nil, &sequence)
	if err != nil {
		t.Fatal(err)
	}
	if key, err := IsKeyframe(WireAV1, payload); err != nil || key {
		t.Fatalf("參考影格誤判：%v %v", key, err)
	}
	for _, bad := range [][]byte{nil, probeAV1[:len(probeAV1)-1], {0x30, 0}, {0x32, 0xff}, {0xb2, 1, 0}, delta} {
		if _, err := IsKeyframe(WireAV1, bad); err == nil {
			t.Fatalf("未拒絕無效 OBU：%x", bad)
		}
	}
}

func TestAV1ActualGOPFraming(t *testing.T) {
	data, err := os.ReadFile("../softwarevideo/testdata/av1.frames")
	if err != nil {
		t.Fatal(err)
	}
	var sequence []byte
	frame := 0
	for len(data) > 0 {
		n := int(binary.BigEndian.Uint32(data))
		data = data[4:]
		payload, err := packAV1(data[:n], nil, &sequence)
		if err != nil {
			t.Fatal(err)
		}
		key, err := IsKeyframe(WireAV1, payload)
		if err != nil || key != (frame == 0) {
			t.Fatalf("AV1 frame %d: key=%v err=%v", frame, key, err)
		}
		data = data[n:]
		frame++
	}
	if frame != 3 {
		t.Fatal(frame)
	}
}
