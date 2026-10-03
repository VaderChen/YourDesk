package video

import (
	"bytes"
	"testing"
)

func TestAV1ValidationOrderAndLimits(t *testing.T) {
	frame := []byte{0x0a, 1, 0, 0x32, 1, 0}
	limit := append(bytes.Clone(frame), bytes.Repeat([]byte{0x7a, 0}, 4094)...)
	if key, err := av1Keyframe(limit); err != nil || !key {
		t.Fatalf("4096 OBUs: key=%v, err=%v", key, err)
	}
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"count", append(bytes.Clone(limit), 0x7a, 0), "AV1 OBU 過多"},
		{"syntax after semantic error", []byte{0x32, 1, 0, 0xb2, 0}, "AV1 OBU 標頭無效或未包含長度"},
		{"first semantic error", []byte{0x0a, 0, 0x32, 0}, "AV1 sequence header 無效"},
		{"second picture", append(bytes.Clone(frame), 0x32, 1, 0), "AV1 需單一影格及 sequence header"},
		{"late sequence", append(bytes.Clone(frame), 0x0a, 1, 0), "AV1 sequence header 無效"},
		{"extension", []byte{0x0e, 1, 0}, "AV1 extension 無效"},
		{"unterminated length", append([]byte{0x0a}, bytes.Repeat([]byte{0x80}, 8)...), "AV1 OBU 截斷"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, err := av1Keyframe(tc.data)
			if key || err == nil || err.Error() != tc.want {
				t.Fatalf("key=%v, err=%v; want %q", key, err, tc.want)
			}
		})
	}
}

func TestPackAV1CacheValidationAndOwnership(t *testing.T) {
	header := []byte{0x0a, 1, 0}
	delta := []byte{0x32, 1, 0x20}
	sequence := bytes.Clone(header)
	if _, err := packAV1([]byte{0x0a, 1, 8, 0xb2}, nil, &sequence); err == nil || !bytes.Equal(sequence, header) {
		t.Fatal("invalid trailing OBU must not alter the cached sequence")
	}
	packed, err := packAV1(delta, nil, &sequence)
	if err != nil || !bytes.Equal(packed, append(bytes.Clone(header), delta...)) {
		t.Fatalf("delta framing: %x %v", packed, err)
	}
	delta[2] = 0
	sequence[2] = 8
	if packed[2] != 0 || packed[5] != 0x20 {
		t.Fatal("packed frame aliases input or cached sequence")
	}
	config := append([]byte{0x81, 0, 0, 0}, header...)
	sequence = nil
	if _, err := packAV1(delta, append(bytes.Clone(config), 0xb2), &sequence); err == nil || sequence != nil {
		t.Fatal("invalid configuration populated the sequence cache")
	}
	if _, err := packAV1(delta, config, &sequence); err != nil {
		t.Fatal(err)
	}
	config[len(config)-1] = 8
	if !bytes.Equal(sequence, header) {
		t.Fatal("sequence cache aliases decoder configuration")
	}
}

func BenchmarkAV1Keyframe(b *testing.B) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"Typical", probeAV1},
		{"4096OBUs", append([]byte{0x0a, 1, 0, 0x32, 1, 0}, bytes.Repeat([]byte{0x7a, 0}, 4094)...)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if key, err := av1Keyframe(tc.data); err != nil || !key {
					b.Fatal(key, err)
				}
			}
		})
	}
}

func BenchmarkPackAV1Delta(b *testing.B) {
	data := append([]byte{0x32, 0x80, 0x80, 0x04}, make([]byte, 65536)...)
	data[4] = 0x20
	sequence := []byte{0x0a, 1, 0}
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		if _, err := packAV1(data, nil, &sequence); err != nil {
			b.Fatal(err)
		}
	}
}
