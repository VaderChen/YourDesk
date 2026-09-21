package prelogin

import "testing"

func TestSASPolicyPreservesExistingPermission(t *testing.T) {
	for current, want := range map[uint64]uint32{0: 1, 1: 1, 2: 3, 3: 3} {
		got, err := sasPolicyWithServices(current)
		if err != nil || got != want {
			t.Fatalf("%d: got %d, %v", current, got, err)
		}
	}
	for _, unknown := range []uint64{4, 255, 1 << 32} {
		if _, err := sasPolicyWithServices(unknown); err == nil {
			t.Fatal("未知原則不可覆寫")
		}
	}
}
