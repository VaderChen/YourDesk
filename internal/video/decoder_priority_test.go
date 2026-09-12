package video

import (
	"bytes"
	"reflect"
	"runtime"
	"testing"
)

func TestEncoderFirstWithHardwareDecodePreference(t *testing.T) {
	for _, tc := range []struct {
		mask uint32
		want []Codec
	}{
		{0, []Codec{CodecHardwareHEVC, CodecHardwareH264}},
		{1 << WireH264, []Codec{CodecHardwareH264, CodecHardwareHEVC}},
		{1 << WireHEVC, []Codec{CodecHardwareHEVC, CodecHardwareH264}},
		{1<<WireHEVC | 1<<WireH264, []Codec{CodecHardwareHEVC, CodecHardwareH264}},
	} {
		if runtime.GOOS == "windows" {
			tc.want = append(tc.want, CodecHardwareAV1)
		}
		if got := PreferredHardwareEncoders(tc.mask); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("優先順序錯誤：%v", got)
		}
	}
}
func TestHEVCSoftwareInputSmoke(t *testing.T) {
	data, err := unpackHEVC(probeHEVC)
	if err != nil || len(data) == 0 {
		t.Fatalf("HEVC fixture 無法轉換：%v", err)
	}
	if !bytes.Equal(data[:4], []byte{0, 0, 0, 1}) || ((data[4]>>1)&63) != 32 {
		t.Fatal("Annex B VPS 不正確")
	}
	for _, bad := range [][]byte{nil, probeHEVC[:3], probeHEVC[:len(probeHEVC)-1]} {
		if _, err := unpackHEVC(bad); err == nil {
			t.Fatal("截斷封包未拒絕")
		}
	}
	bad := append([]byte(nil), probeHEVC...)
	bad[5] = 0
	if _, err := unpackHEVC(bad); err == nil {
		t.Fatal("無效 VPS 未拒絕")
	}
}
