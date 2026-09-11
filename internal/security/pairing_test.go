package security

import "testing"

func TestSignVerify(t *testing.T) {
	s, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	key, err := DecodeSecret(s)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("offer")
	if !Verify(key, msg, Sign(key, msg)) {
		t.Fatal("signature should verify")
	}
	if Verify(key, []byte("answer"), Sign(key, msg)) {
		t.Fatal("different payload must fail")
	}
}

func TestGeneratedSecretHasAtLeast80BitsOfEntropy(t *testing.T) {
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) != generatedSecretLen || generatedSecretLen < 16 {
		t.Fatalf("generated secret length = %d, want at least 16 Base32 characters", len(secret))
	}
}
