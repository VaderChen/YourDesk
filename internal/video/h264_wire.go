package video

import (
	"encoding/binary"
	"fmt"
)

// 維持既有跨平台格式：參數集數量、長度＋SPS/PPS，再接四位元組長度的 NAL。
// Media Foundation 的 Annex B／avcC 輸出在這裡統一轉換；容許 GOP 中的參考影格。
// 長度前綴必須能完整解析整份資料，不能只以開頭幾個位元組猜測。
func h264LengthNALs(data []byte) ([][]byte, error) {
	var result [][]byte
	for len(data) > 0 {
		if len(data) < 4 {
			return nil, fmt.Errorf("H.264 NAL 長度不足")
		}
		n := uint64(binary.BigEndian.Uint32(data))
		data = data[4:]
		if n < 1 || n > uint64(len(data)) {
			return nil, fmt.Errorf("H.264 NAL 長度無效")
		}
		nal := data[:int(n)]
		if nal[0]&0x80 != 0 || nal[0]&31 == 0 {
			return nil, fmt.Errorf("H.264 NAL 標頭無效")
		}
		result = append(result, nal)
		data = data[int(n):]
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("H.264 沒有 NAL")
	}
	return result, nil
}
func h264NALs(data []byte) ([][]byte, error) {
	if nals, err := h264LengthNALs(data); err == nil {
		return nals, nil
	}
	return h264AnnexBNALs(data)
}
func h264AnnexBNALs(data []byte) ([][]byte, error) {
	var result [][]byte
	start := func(offset int) (int, int) {
		for i := offset; i+3 <= len(data); i++ {
			if data[i] == 0 && data[i+1] == 0 {
				if data[i+2] == 1 {
					return i, 3
				}
				if i+4 <= len(data) && data[i+2] == 0 && data[i+3] == 1 {
					return i, 4
				}
			}
		}
		return -1, 0
	}
	first, prefix := start(0)
	if first < 0 {
		return nil, fmt.Errorf("H.264 缺少 Annex B 起始碼")
	}
	for _, v := range data[:first] {
		if v != 0 {
			return nil, fmt.Errorf("H.264 起始碼前資料無效")
		}
	}
	for at, n := first, prefix; at >= 0; {
		next, np := start(at + n)
		end := next
		if end < 0 {
			end = len(data)
		}
		for end > at+n && data[end-1] == 0 {
			end--
		}
		if end <= at+n {
			return nil, fmt.Errorf("H.264 空 NAL")
		}
		nal := data[at+n : end]
		if nal[0]&0x80 != 0 || nal[0]&31 == 0 {
			return nil, fmt.Errorf("H.264 NAL 標頭無效")
		}
		result = append(result, nal)
		at, n = next, np
	}
	return result, nil
}
func h264Config(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] != 1 {
		return h264NALs(data)
	}
	if len(data) < 7 || data[4]&3 != 3 {
		return nil, fmt.Errorf("不支援的 H.264 avcC")
	}
	var sets [][]byte
	count := int(data[5] & 31)
	data = data[6:]
	for group := 0; group < 2; group++ {
		for i := 0; i < count; i++ {
			if len(data) < 2 {
				return nil, fmt.Errorf("H.264 參數集不足")
			}
			n := int(binary.BigEndian.Uint16(data))
			data = data[2:]
			if n < 1 || n > len(data) {
				return nil, fmt.Errorf("H.264 參數集長度無效")
			}
			sets = append(sets, data[:n])
			data = data[n:]
		}
		if group == 0 {
			if len(data) < 1 {
				return nil, fmt.Errorf("H.264 缺少 PPS")
			}
			count = int(data[0])
			data = data[1:]
		}
	}
	return sets, nil
}
func packH264(data, config []byte) ([]byte, error) {
	if len(data) > 32<<20 || len(config) > 65536 {
		return nil, fmt.Errorf("H.264 影格過大")
	}
	nals, err := h264NALs(data)
	if len(config) > 0 && config[0] == 1 {
		nals, err = h264LengthNALs(data)
	}
	if err != nil {
		return nil, err
	}
	sets, err := h264Config(config)
	if err != nil {
		return nil, err
	}
	var sps, pps []byte
	picture := false
	for _, nal := range append(sets, nals...) {
		switch nal[0] & 31 {
		case 7:
			sps = nal
		case 8:
			pps = nal
		}
	}
	for _, nal := range nals {
		switch nal[0] & 31 {
		case 5:
			picture = true
		case 1:
			picture = true
		}
	}
	if len(sps) == 0 || len(pps) == 0 || !picture {
		return nil, fmt.Errorf("硬體編碼器未提供完整 SPS／PPS／影像")
	}
	out := []byte{2}
	appendNAL := func(nal []byte) {
		out = binary.BigEndian.AppendUint32(out, uint32(len(nal)))
		out = append(out, nal...)
	}
	appendNAL(sps)
	appendNAL(pps)
	for _, nal := range nals {
		appendNAL(nal)
	}
	if len(out) > 32<<20 {
		return nil, fmt.Errorf("H.264 封包過大")
	}
	return out, nil
}
func unpackH264(data []byte) ([]byte, error) {
	if len(data) < 1 || len(data) > 32<<20 || data[0] != 2 {
		return nil, fmt.Errorf("無效的 H.264 封包")
	}
	data = data[1:]
	out := make([]byte, 0, len(data))
	count := 0
	picture := false
	for len(data) > 0 {
		if len(data) < 4 {
			return nil, fmt.Errorf("H.264 封包截斷")
		}
		n := int(binary.BigEndian.Uint32(data))
		data = data[4:]
		if n < 1 || n > len(data) {
			return nil, fmt.Errorf("H.264 封包長度無效")
		}
		kind := data[0] & 31
		if (count == 0 && kind != 7) || (count == 1 && kind != 8) {
			return nil, fmt.Errorf("H.264 參數集無效")
		}
		picture = picture || kind == 5 || kind == 1
		count++
		out = append(out, 0, 0, 0, 1)
		out = append(out, data[:n]...)
		data = data[n:]
	}
	if count < 3 || !picture {
		return nil, fmt.Errorf("H.264 缺少影像")
	}
	return out, nil
}
