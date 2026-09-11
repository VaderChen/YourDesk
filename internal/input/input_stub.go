//go:build !darwin && !windows

package input

import "errors"
import "yourdesk/internal/rawkey"

func EnsurePermissions() error { return errors.New("此平台尚未提供輸入注入") }

func (Native) Move(float64, float64) error { return errors.New("此平台尚未提供輸入注入") }
func (Native) Button(int, bool) error      { return errors.New("此平台尚未提供輸入注入") }
func (Native) ButtonAt(int, bool, float64, float64) error {
	return errors.New("此平台尚未提供輸入注入")
}
func (Native) Key(string, bool) error { return errors.New("此平台尚未提供輸入注入") }
func (Native) Wheel(float64) error    { return errors.New("此平台尚未提供輸入注入") }

func (Native) RawKey(rawkey.Event) error {
	return errors.New("此平台尚未提供原始按鍵注入")
}
