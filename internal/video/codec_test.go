package video

import "testing"

func TestSelectSoftware(t *testing.T) {
	s, err := Select(CodecSoftwareJPEG)
	if err != nil || s.Selected != CodecSoftwareJPEG || s.Hardware {
		t.Fatalf("%+v %v", s, err)
	}
}
func TestSelectAutoHasSelection(t *testing.T) {
	s, err := Select(CodecAuto)
	if err != nil || s.Selected == "" || s.Backend == "" {
		t.Fatalf("%+v %v", s, err)
	}
}
