package signaling

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"yourdeskandroid/core/internal/security"
)

var ErrMessageTime = errors.New("signaling 訊息時間超出允許範圍")

var ErrAuthentication = errors.New("signaling HMAC 驗證失敗")

type Role string

const (
	RoleHost   Role = "host"
	RoleViewer Role = "viewer"
)

type Kind string

const (
	KindJoin   Kind = "join"
	KindOffer  Kind = "offer"
	KindAnswer Kind = "answer"
	KindError  Kind = "error"
)

type Envelope struct {
	Binding   string          `json:"binding,omitempty"`
	Room      string          `json:"room"`
	Role      Role            `json:"role"`
	Kind      Kind            `json:"kind"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Signature string          `json:"signature"`
}

type signedFields struct {
	Binding   string          `json:"binding,omitempty"`
	Room      string          `json:"room"`
	Role      Role            `json:"role"`
	Kind      Kind            `json:"kind"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

func NewEnvelope(room string, role Role, kind Kind, payload any, secret []byte) (Envelope, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return Envelope{}, err
		}
		raw = b
	}
	e := Envelope{Room: room, Role: role, Kind: kind, Timestamp: time.Now().Unix(), Payload: raw}
	b, err := e.signingBytes()
	if err != nil {
		return Envelope{}, err
	}
	e.Signature = security.Sign(secret, b)
	return e, nil
}

func (e Envelope) Verify(secret []byte) error {
	if e.Room == "" || (e.Role != RoleHost && e.Role != RoleViewer) {
		return errors.New("無效的 signaling envelope")
	}
	if delta := time.Now().Unix() - e.Timestamp; delta > 300 || delta < -300 {
		return ErrMessageTime
	}
	b, err := e.signingBytes()
	if err != nil {
		return err
	}
	if !security.Verify(secret, b, e.Signature) {
		return ErrAuthentication
	}
	return nil
}

func (e Envelope) DecodePayload(v any) error {
	if len(e.Payload) == 0 {
		return errors.New("signaling payload 為空")
	}
	if err := json.Unmarshal(e.Payload, v); err != nil {
		return fmt.Errorf("解析 signaling payload: %w", err)
	}
	return nil
}

func (e Envelope) signingBytes() ([]byte, error) {
	return json.Marshal(signedFields{
		Binding: e.Binding, Room: e.Room, Role: e.Role, Kind: e.Kind, Timestamp: e.Timestamp, Payload: e.Payload,
	})
}
