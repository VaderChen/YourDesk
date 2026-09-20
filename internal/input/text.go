package input

import (
	"errors"
	"unicode/utf16"
	"unicode/utf8"
)

// MaxTextBytes bounds a single committed IME/paste message before UTF-16 allocation.
const MaxTextBytes = 16 * 1024

// TextController is separate from physical keys: Unicode input must not change
// the clipboard or be interpreted as platform-dependent virtual key names.
type TextController interface {
	Text(string) error
}

func ValidateText(text string) error {
	if len(text) > MaxTextBytes {
		return errors.New("文字輸入超過 16 KiB")
	}
	if !utf8.ValidString(text) {
		return errors.New("文字輸入不是有效的 UTF-8")
	}
	return nil
}

func SendText(controller Controller, text string) error {
	if err := ValidateText(text); err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	c, ok := controller.(TextController)
	if !ok {
		return errors.New("此輸入控制器不支援 Unicode 文字")
	}
	return c.Text(text)
}

func textUTF16(text string) ([]uint16, error) {
	if err := ValidateText(text); err != nil {
		return nil, err
	}
	return utf16.Encode([]rune(text)), nil
}
