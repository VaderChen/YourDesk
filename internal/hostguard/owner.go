package hostguard

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Owner struct {
	PID     int    `json:"pid"`
	Path    string `json:"path"`
	Created uint64 `json:"created,string"`
}

func lockPath(room string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "YourDesk", fmt.Sprintf("host-%x.lock", sha256.Sum256([]byte(room)))), nil
}
func publishOwner(path string) func() {
	owner, err := processOwner(os.Getpid())
	if err != nil {
		return func() {}
	}
	b, err := json.Marshal(owner)
	if err != nil {
		return func() {}
	}
	if os.WriteFile(path+".owner", b, 0600) != nil {
		return func() {}
	}
	return func() { os.Remove(path + ".owner") }
}

// 只採用仍被鎖住的 Host 所留資料，並核對程序建立時間與路徑。
func FindOwner(room string) *Owner {
	path, err := lockPath(room)
	if err != nil {
		return nil
	}
	release, err := tryLock(path)
	if err == nil {
		release()
		return nil
	}
	if !errors.Is(err, ErrOccupied) {
		return nil
	}
	b, err := os.ReadFile(path + ".owner")
	if err != nil {
		return nil
	}
	var owner Owner
	if json.Unmarshal(b, &owner) != nil || owner.PID <= 0 || owner.PID == os.Getpid() {
		return nil
	}
	actual, err := processOwner(owner.PID)
	if err != nil || actual != owner {
		return nil
	}
	return &owner
}
func StopOwner(room string, expected Owner) error {
	current := FindOwner(room)
	if current == nil || *current != expected {
		return errors.New("原程序已結束或身分已變更，未停止任何程序")
	}
	return stopVerified(expected)
}

// 只比較原程序的建立時間、路徑與 PID，不使用 PID 單獨識別。
func IsSameProcess(owner Owner) bool {
	actual, err := processOwner(owner.PID)
	return err == nil && actual == owner
}
func StopCandidate(owner Owner) error {
	if owner.PID == os.Getpid() {
		return errors.New("不能停止目前 APP")
	}
	return stopVerified(owner)
}
