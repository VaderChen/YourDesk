package serviceupdate

import (
	"errors"
	"fmt"
)

// Operations 只管理受保護的服務副本，絕不以 SYSTEM 執行一般使用者的安裝器。
type Operations interface {
	Stop() error
	Backup() error
	Install() error
	Start() error
	Verify(updated bool) error
	Restore() error
}

type Reply struct {
	Phase string `json:"phase"`
	Error string `json:"error,omitempty"`
}

// Transaction 由 worker 序列執行；逾時與客戶端放棄也呼叫 Rollback。
type Transaction struct {
	Phase                   string
	ops                     Operations
	stopped, saved, changed bool
}

func NewTransaction(ops Operations) *Transaction { return &Transaction{Phase: "ready", ops: ops} }

func (t *Transaction) Command(operation string) Reply {
	var err error
	mutating := false
	switch operation {
	case "status":
	case "begin":
		if t.Phase == "stopped" {
			break
		}
		if t.Phase != "ready" {
			err = errors.New("服務更新尚未就緒")
			break
		}
		t.stopped = true // Stop 即使回報逾時，也可能已停止服務。
		mutating = true
		if err = t.ops.Stop(); err == nil {
			err = t.ops.Backup()
			if err == nil {
				t.saved = true
				t.Phase = "stopped"
			}
		}
	case "commit":
		if t.Phase == "applied" {
			break
		}
		if t.Phase != "stopped" {
			err = errors.New("服務更新尚未停止")
			break
		}
		t.changed = true
		mutating = true
		if err = t.ops.Install(); err == nil {
			err = t.ops.Start()
		}
		if err == nil {
			err = t.ops.Verify(true)
		}
		if err == nil {
			t.Phase = "applied"
		}
	case "rollback":
		err = t.Rollback()
	case "finish":
		if t.Phase == "complete" {
			break
		}
		if t.Phase != "applied" {
			err = errors.New("服務更新尚未完成驗證")
			break
		}
		t.Phase = "complete"
	default:
		err = errors.New("未知服務更新操作")
	}
	if err != nil && mutating {
		if rollbackErr := t.Rollback(); rollbackErr != nil {
			err = fmt.Errorf("%w；回復失敗：%v", err, rollbackErr)
		}
	}
	reply := Reply{Phase: t.Phase}
	if err != nil {
		reply.Error = err.Error()
	}
	return reply
}

func (t *Transaction) Rollback() error {
	if t.Phase == "complete" {
		return errors.New("更新已完成，不能回復已確認的版本")
	}
	if t.Phase == "rolled-back" {
		return nil
	}
	if !t.stopped {
		t.Phase = "rolled-back"
		return nil
	}
	// 安裝前失敗不需要覆寫舊檔案；備份／還原不碰配對資料。
	if t.changed {
		if err := t.ops.Stop(); err != nil {
			t.Phase = "failed"
			return err
		}
		if !t.saved {
			t.Phase = "failed"
			return errors.New("服務備份不完整")
		}
		if err := t.ops.Restore(); err != nil {
			t.Phase = "failed"
			return err
		}
	}
	if err := t.ops.Start(); err != nil {
		t.Phase = "failed"
		return err
	}
	if err := t.ops.Verify(false); err != nil {
		t.Phase = "failed"
		return err
	}
	t.Phase = "rolled-back"
	return nil
}
