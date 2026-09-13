package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"unicode/utf8"
)

const (
	secretBytes        = 24
	generatedSecretLen = 16 // 80 bits of Base32 entropy; suitable for unattended hosts.
)

func NewSecret() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])[:generatedSecretLen], nil
}

func DecodeSecret(value string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(b) < 16 {
		if utf8.RuneCountInString(value) < 6 || len(value) > 1000 {
			return nil, errors.New("連線密碼至少需 6 個字元，且不可超過 1000 位元組")
		}
		sum := sha256.Sum256([]byte("yourdesk-password-v1:" + value))
		return sum[:], nil
	}
	return b, nil
}

func Sign(secret, payload []byte) string {
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write(payload)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func Verify(secret, payload []byte, signature string) bool {
	want, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write(payload)
	return hmac.Equal(want, m.Sum(nil))
}
