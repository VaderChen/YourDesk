//go:build windows

package input

import "encoding/binary"

func (Native) Text(text string) error {
	units, err := textUTF16(text)
	if err != nil || len(units) == 0 {
		return err
	}
	return onInputDesktop(func() error {
		for _, unit := range units {
			// KEYEVENTF_UNICODE requires wVk=0 and the UTF-16 unit in wScan.
			var data [32]byte
			binary.LittleEndian.PutUint16(data[2:4], unit)
			binary.LittleEndian.PutUint32(data[4:8], 0x0004)
			if err := sendNative(input{typ: inputKeyboard, data: data}); err != nil {
				return err
			}
			binary.LittleEndian.PutUint32(data[4:8], 0x0004|keyUp)
			if err := sendNative(input{typ: inputKeyboard, data: data}); err != nil {
				return err
			}
		}
		return nil
	})
}
