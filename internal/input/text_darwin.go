//go:build darwin

package input

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>
static int yd_unicode_text(const UniChar *text, size_t length) {
  // One Unicode scalar per key pair also preserves non-BMP surrogate pairs.
  for (size_t offset = 0; offset < length;) {
    size_t count = text[offset] >= 0xD800 && text[offset] <= 0xDBFF ? 2 : 1;
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
    CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
    if (!down || !up) {
      if (down) CFRelease(down);
      if (up) CFRelease(up);
      return -1;
    }
    CGEventSetFlags(down, 0);
    CGEventSetFlags(up, 0);
    CGEventKeyboardSetUnicodeString(down, count, text + offset);
    CGEventKeyboardSetUnicodeString(up, count, text + offset);
    CGEventPost(kCGHIDEventTap, down);
    CGEventPost(kCGHIDEventTap, up);
    CFRelease(down);
    CFRelease(up);
    offset += count;
  }
  return 0;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

func (Native) Text(text string) error {
	units, err := textUTF16(text)
	if err != nil || len(units) == 0 {
		return err
	}
	if C.yd_unicode_text((*C.UniChar)(unsafe.Pointer(&units[0])), C.size_t(len(units))) != 0 {
		return errors.New("無法建立 Unicode 鍵盤事件")
	}
	return nil
}
