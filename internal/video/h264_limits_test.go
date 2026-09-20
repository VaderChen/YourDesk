package video

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

func TestH264NALCountLimit(t *testing.T) {
	for _, n := range []int{maxVideoNALs, maxVideoNALs + 1} {
		annex := bytes.Repeat([]byte{0, 0, 0, 1, 0x0c, 0x80}, n)
		lengths := bytes.Repeat([]byte{0, 0, 0, 2, 0x0c, 0x80}, n)
		_, annexErr := h264AnnexBNALs(annex)
		_, lengthsErr := h264LengthNALs(lengths)
		wantError := n > maxVideoNALs
		if (annexErr != nil) != wantError || (lengthsErr != nil) != wantError {
			t.Fatalf("NAL count %d: annex=%v lengths=%v", n, annexErr, lengthsErr)
		}
	}
}

func wireWithNALCount(t *testing.T, codec WireCodec, count int) []byte {
	t.Helper()
	fixture := "probes/h264.bin"
	if codec == WireHEVC {
		fixture = "probes/hevc.bin"
	}
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	actual := 0
	for rest := data[1:]; len(rest) != 0; actual++ {
		n := int(binary.BigEndian.Uint32(rest))
		rest = rest[4+n:]
	}
	for ; actual < count; actual++ {
		if codec == WireHEVC {
			data = append(data, 0, 0, 0, 3, 38<<1, 1, 0x80)
		} else {
			data = append(data, 0, 0, 0, 2, 0x0c, 0x80)
		}
	}
	return data
}

func TestWireRejectsExcessiveNALsBeforeConversion(t *testing.T) {
	for _, codec := range []WireCodec{WireH264, WireHEVC} {
		for _, count := range []int{maxVideoNALs, maxVideoNALs + 1} {
			data := wireWithNALCount(t, codec, count)
			_, wireErr := IsKeyframe(codec, data)
			var err error
			if codec == WireH264 {
				_, err = unpackH264(data)
			} else {
				_, err = unpackHEVC(data)
			}
			wantError := count > maxVideoNALs
			if (wireErr != nil) != wantError || (err != nil) != wantError {
				t.Fatalf("codec=%d count=%d: wire=%v conversion=%v", codec, count, wireErr, err)
			}
		}
	}
}

func TestH264DimensionsDoesNotAllocateNALTable(t *testing.T) {
	annex, err := unpackH264(wireWithNALCount(t, WireH264, maxVideoNALs))
	if err != nil {
		t.Fatal(err)
	}
	// SPS 與 NAL 數無關；只容許 SPS 去除 escape bytes 的固定少量配置。
	allocs := testing.AllocsPerRun(20, func() {
		w, h, err := h264Dimensions(annex)
		if err != nil || w != 128 || h != 128 {
			t.Fatalf("dimensions=%dx%d err=%v", w, h, err)
		}
	})
	if allocs > 2 {
		t.Fatalf("dimensions allocated a NAL table: %.1f allocations", allocs)
	}
	// 大量極小 NAL 在固定上限即終止，不再依封包大小建立 slice table。
	oversized := bytes.Repeat([]byte{0, 0, 0, 1, 0x0c, 0x80}, (4<<20)/6)
	if _, _, err := h264Dimensions(oversized); err == nil {
		t.Fatal("accepted excessive NALs")
	}
	allocs = testing.AllocsPerRun(20, func() { _, _, _ = h264Dimensions(oversized) })
	if allocs > 3 {
		t.Fatalf("rejected packet allocated %.1f objects", allocs)
	}
}

func TestH264DimensionsStillValidatesAfterSPS(t *testing.T) {
	fixture, err := os.ReadFile("probes/h264.bin")
	if err != nil {
		t.Fatal(err)
	}
	annex, err := unpackH264(fixture)
	if err != nil {
		t.Fatal(err)
	}
	annex = append(annex, 0, 0, 0, 1, 0x80)
	if _, _, err := h264Dimensions(annex); err == nil {
		t.Fatal("invalid NAL following a valid SPS was ignored")
	}
}

func BenchmarkH264DimensionsManyNALs(b *testing.B) {
	fixture, err := os.ReadFile("probes/h264.bin")
	if err != nil {
		b.Fatal(err)
	}
	annex, err := unpackH264(fixture)
	if err != nil {
		b.Fatal(err)
	}
	annex = append(annex, bytes.Repeat([]byte{0, 0, 0, 1, 0x0c, 0x80}, maxVideoNALs-32)...)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := h264Dimensions(annex); err != nil {
			b.Fatal(err)
		}
	}
}
