//go:build !darwin && !windows

package input

import "errors"

func (Native) Text(text string) error {
	if err := ValidateText(text); err != nil {
		return err
	}
	return errors.New("此平台尚未提供 Unicode 文字注入")
}
