package video

import "fmt"

// AV1 使用含長度的 low-overhead OBU；每個封包附帶 sequence header，
// 讓重連與 GOP 恢復能驗證真正的 key frame，而不是信任傳送端旗標。
type av1OBU struct {
	kind      byte
	raw, body []byte
}

// walkAV1OBUs visits views into data; callers must copy anything they retain.
func walkAV1OBUs(data []byte, visit func(av1OBU)) error {
	if len(data) == 0 || len(data) > 32<<20 {
		return fmt.Errorf("AV1 封包長度無效")
	}
	count := 0
	for len(data) > 0 {
		start := data
		header := data[0]
		data = data[1:]
		if header&0x81 != 0 || header&2 == 0 {
			return fmt.Errorf("AV1 OBU 標頭無效或未包含長度")
		}
		if header&4 != 0 {
			if len(data) == 0 || data[0]&7 != 0 {
				return fmt.Errorf("AV1 extension 無效")
			}
			data = data[1:]
		}
		var size uint64
		done := false
		for i := 0; i < 8 && len(data) > 0; i++ {
			b := data[0]
			data = data[1:]
			size |= uint64(b&127) << uint(7*i)
			if b&128 == 0 {
				done = true
				break
			}
		}
		if !done || size > uint64(len(data)) {
			return fmt.Errorf("AV1 OBU 截斷")
		}
		n := int(size)
		count++
		if count > 4096 {
			return fmt.Errorf("AV1 OBU 過多")
		}
		visit(av1OBU{(header >> 3) & 15, start[:len(start)-len(data)+n], data[:n]})
		data = data[n:]
	}
	return nil
}
func av1Keyframe(data []byte) (bool, error) {
	sequence, reduced, picture, key := false, false, false, false
	var invalid string
	err := walkAV1OBUs(data, func(obu av1OBU) {
		// Finish syntax validation even after a semantic failure so malformed
		// trailing OBUs retain the original error precedence.
		if invalid != "" {
			return
		}
		switch obu.kind {
		case 1:
			if len(obu.body) == 0 || picture {
				invalid = "AV1 sequence header 無效"
				return
			}
			sequence = true
			reduced = obu.body[0]&8 != 0
		case 3, 6:
			if !sequence || len(obu.body) == 0 || picture {
				invalid = "AV1 需單一影格及 sequence header"
				return
			}
			picture = true
			key = reduced || (obu.body[0]&0x80 == 0 && (obu.body[0]>>5)&3 == 0)
		}
	})
	if err != nil {
		return false, err
	}
	if invalid != "" {
		return false, fmt.Errorf("%s", invalid)
	}
	if !picture {
		return false, fmt.Errorf("AV1 封包缺少影格")
	}
	return key, nil
}
func packAV1(data, config []byte, sequence *[]byte) ([]byte, error) {
	var header []byte
	err := walkAV1OBUs(data, func(obu av1OBU) {
		if obu.kind == 1 {
			header = obu.raw
		}
	})
	if err != nil {
		return nil, err
	}
	found := header != nil
	if found {
		*sequence = append((*sequence)[:0], header...)
	}
	if !found && len(*sequence) == 0 && len(config) > 4 && config[0] == 0x81 {
		if e := walkAV1OBUs(config[4:], func(obu av1OBU) {
			if obu.kind == 1 {
				header = obu.raw
			}
		}); e == nil && header != nil {
			*sequence = append([]byte(nil), header...)
		}
	}
	payload := data
	if !found {
		if len(*sequence) > (32<<20)-len(data) {
			return nil, fmt.Errorf("AV1 封包長度無效")
		}
		// data is nonempty. Capping the prefix forces a new backing array while
		// append copies both parts without first zeroing the complete payload.
		payload = append((*sequence)[:len(*sequence):len(*sequence)], data...)
	}
	if _, err = av1Keyframe(payload); err != nil {
		return nil, err
	}
	return payload, nil
}
