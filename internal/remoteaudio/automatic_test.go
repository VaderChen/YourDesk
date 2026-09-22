package remoteaudio

import "testing"

func TestAutomaticCodecPreferenceAndFallback(t *testing.T) {
	source := []Capability{{Codec: "aac", Encode: true, HardwareEncode: true}, {Codec: "opus", Encode: true}, {Codec: "pcm", Encode: true}}
	receiver := []Capability{{Codec: "aac", Decode: true}, {Codec: "opus", Decode: true}, {Codec: "pcm", Decode: true}}
	check := func(want string) {
		t.Helper()
		codec, err := AutomaticCodec(source, receiver)
		if err != nil || codec != want {
			t.Fatalf("got %q %v, want %q", codec, err, want)
		}
	}
	check("aac")
	source[0].HardwareEncode = false
	check("opus")
	source[1].Encode = false
	check("aac-software")
	receiver[0].Decode = false
	check("pcm")
	receiver[2].Decode = false
	if _, err := AutomaticCodec(source, receiver); err == nil {
		t.Fatal("兩端沒有共同可用格式時不應假裝可用")
	}
	if err := (Settings{Codec: "auto"}).Normalized().Validate(); err != nil {
		t.Fatal(err)
	}
	s := &Source{}
	if _, err := s.Configure(Settings{Enabled: true, Codec: "auto", Profile: "standard", Generation: 1}); err == nil {
		t.Fatal("未協商的 auto 不可被當作實際 AAC 編碼")
	}
}
