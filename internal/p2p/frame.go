package p2p

import "encoding/binary"

type frameAssembler struct {
	viewID        uint64
	display       int
	seq           uint64
	width, height uint32
	total         uint32
	x, y          uint32
	keyframe      bool
	codec         byte
	chunks        [][]byte
	received      uint32
	bytes         int
	complete      bool
}

func (a *frameAssembler) add(b []byte) (Frame, bool) {
	if len(b) < frameHeaderSize {
		return Frame{}, false
	}
	headerSize := frameHeaderSize
	var viewID uint64
	if b[40]&2 != 0 {
		headerSize += 8
		if len(b) < headerSize {
			return Frame{}, false
		}
		viewID = binary.BigEndian.Uint64(b[44:52])
	}
	display := int(binary.BigEndian.Uint16(b[42:44])) - 1
	seq := binary.BigEndian.Uint64(b[0:8])
	w := binary.BigEndian.Uint32(b[8:12])
	h := binary.BigEndian.Uint32(b[12:16])
	idx := binary.BigEndian.Uint32(b[16:20])
	total := binary.BigEndian.Uint32(b[20:24])
	x := binary.BigEndian.Uint32(b[32:36])
	y := binary.BigEndian.Uint32(b[36:40])
	keyframe := b[40]&1 != 0
	if total == 0 || total > 1200 || idx >= total || len(b)-headerSize > chunkSize || w == 0 || h == 0 || w > 8192 || h > 8192 || uint64(w)*uint64(h) > 32*1024*1024 || b[41] > 3 || b[40]&^byte(3) != 0 {
		return Frame{}, false
	}
	if seq < a.seq {
		return Frame{}, false
	}
	if seq != a.seq || (a.total == 0) {
		a.viewID = viewID
		a.display = display
		a.codec = b[41]
		a.seq, a.width, a.height, a.x, a.y, a.keyframe, a.total = seq, w, h, x, y, keyframe, total
		a.received, a.bytes, a.complete = 0, 0, false
		// 只重用分片索引，不重用交給解碼器的資料。清掉未完成影格的引用。
		clear(a.chunks)
		if total > 1 {
			if cap(a.chunks) < int(total) {
				a.chunks = make([][]byte, total)
			} else {
				a.chunks = a.chunks[:total]
			}
		} else {
			a.chunks = a.chunks[:0]
		}
	}
	if a.viewID != viewID || a.display != display || a.width != w || a.height != h || a.total != total || a.x != x || a.y != y || a.keyframe != keyframe || a.codec != b[41] {
		return Frame{}, false
	}
	if a.complete {
		return Frame{}, false
	}
	var jpeg []byte
	if total == 1 {
		// 常見的小影格只複製一次，不配置 map 或整個 chunkSize 的輸出。
		jpeg = append([]byte(nil), b[headerSize:]...)
	} else {
		if a.chunks[idx] != nil {
			return Frame{}, false
		}
		chunk := make([]byte, len(b)-headerSize)
		copy(chunk, b[headerSize:])
		a.chunks[idx] = chunk
		a.bytes += len(chunk)
		a.received++
		if a.received != total {
			return Frame{}, false
		}
		// 可變長度分片仍相容；以實際長度配置，不按最大分片尺寸預留。
		jpeg = make([]byte, 0, a.bytes)
		for _, c := range a.chunks {
			jpeg = append(jpeg, c...)
		}
		clear(a.chunks)
	}
	a.complete = true
	return Frame{ViewID: a.viewID, Display: a.display, Codec: a.codec, Sequence: seq, Width: w, Height: h, X: a.x, Y: a.y, Keyframe: a.keyframe, JPEG: jpeg}, true
}
