package deviceid

import "testing"

func TestEncodeStableAndOpaque(t *testing.T) {
	a := encode(SourceCPU, " abc-123 ")
	b := encode(SourceCPU, "ABC-123")
	if a != b {
		t.Fatalf("UID 不穩定: %q != %q", a, b)
	}
	if len(a) != 27 || a[:3] != "YD-" {
		t.Fatalf("UID 格式錯誤: %q", a)
	}
	if a == encode(SourceMainboard, "ABC-123") {
		t.Fatal("不同來源必須有 domain separation")
	}
}

func TestUsable(t *testing.T) {
	for _, v := range []string{"", "unknown", "Default String", "0000000000000000"} {
		if usable(v) {
			t.Fatalf("應拒絕 %q", v)
		}
	}
	if !usable("real-id") {
		t.Fatal("應接受有效識別")
	}
}
