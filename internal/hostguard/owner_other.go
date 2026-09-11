//go:build !windows

package hostguard

import "errors"

func processOwner(int) (Owner, error) {
	return Owner{}, errors.New("此平台未提供可驗證的程序資訊")
}
func stopVerified(Owner) error { return errors.New("此平台不支援停止衝突程序") }
