package filetransfer

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"os"
)

// cancel authenticates ownership independently of resume's filesystem checks.
// A moved destination or changed stage must not strand a detached reservation.
// The caller already holds the hub lock and passed the session authorization
// check in command; releaseUpload only unlinks the originally owned inode.
func (s *session) cancel(raw json.RawMessage) (any, error) {
	var in struct {
		ID    string  `json:"id"`
		Token *string `json:"token,omitempty"`
		Path  *string `json:"path,omitempty"`
		Size  *int64  `json:"size,omitempty"`
	}
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	if len(in.ID) != 32 {
		return nil, errors.New("傳輸識別碼無效")
	}
	u, c := s.uploads[in.ID], s.receipts[in.ID]
	var identity *transferIdentity
	if u != nil {
		identity = &u.transferIdentity
	} else if c != nil {
		identity = &c.transferIdentity
	}
	withToken := in.Token != nil || in.Path != nil || in.Size != nil
	if withToken {
		if in.Token == nil || in.Path == nil || in.Size == nil || !validResumeToken(*in.Token) || in.ID != (*in.Token)[:32] || *in.Size < 0 || *in.Size > MaxFileSize {
			return nil, errors.New("取消憑證或參數無效")
		}
		name, err := relative(*in.Path)
		if err != nil || name == "." {
			return nil, errors.New("取消憑證或參數無效")
		}
		if identity != nil {
			hash := sha256.Sum256([]byte(*in.Token))
			if subtle.ConstantTimeCompare(identity.tokenHash[:], hash[:]) != 1 || identity.path != name || identity.size != *in.Size || !os.SameFile(identity.home, s.homeInfo) {
				return nil, errors.New("取消憑證不符")
			}
			if identity.owner != s && ownerLive(identity.owner) {
				return nil, errors.New("此傳輸仍由另一個已連線工作階段使用")
			}
		}
	} else if identity != nil && identity.owner != s {
		// Legacy ID-only cancellation remains scoped to its current owner.
		return nil, errors.New("請提供續傳憑證以取消此傳輸")
	}
	if c != nil {
		// Keep the bounded completion receipt so a lost cancel reply followed by
		// a retry cannot misreport a committed upload as cancelled. Cancellation
		// never reads, changes, or removes the completed user file.
		c.owner = s
		return map[string]any{"ok": true, "state": "complete"}, nil
	}
	if u != nil {
		s.removeUpload(in.ID)
	}
	// Missing IDs are idempotent, including lost begin replies and expired
	// reservations. No other transfer can be affected by this branch.
	return map[string]any{"ok": true, "state": "cancelled"}, nil
}
