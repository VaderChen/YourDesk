package signaling

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// SealTransport 將傳輸位址中的 PSK 加密；不得直接放入 SDP、LOG 或站台設定。
// AAD 綁定 room、直連 TLS 工作階段、角色與完整 SDP（含每次新生的 ICE 憑證）。
func (c *Client) transportCipher(sdp string) (cipher.AEAD, []byte, error) {
	h := hmac.New(sha256.New, c.secret)
	h.Write([]byte("YourDesk transport offer v1"))
	block, err := aes.NewCipher(h.Sum(nil))
	if err != nil {
		return nil, nil, err
	}
	aead, err := cipher.NewGCM(block)
	aad, _ := json.Marshal([]string{"YourDesk transport offer v1", c.room, c.binding, string(RoleHost), sdp})
	return aead, aad, err
}
func (c *Client) SealTransport(sdp, value string) (string, error) {
	aead, aad, err := c.transportCipher(sdp)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(value), aad)), nil
}
func (c *Client) OpenTransport(sdp, value string) (string, error) {
	if len(value) > 16384 {
		return "", errors.New("傳輸配對資料過大")
	}
	aead, aad, err := c.transportCipher(sdp)
	if err != nil {
		return "", err
	}
	data, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || len(data) < aead.NonceSize() {
		return "", errors.New("傳輸配對資料無效")
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], aad)
	if err != nil {
		return "", errors.New("傳輸配對驗證失敗")
	}
	return string(plain), nil
}
