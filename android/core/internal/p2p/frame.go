package p2p

import "encoding/binary"

const maxAssembledFrameBytes = 32 * 1024 * 1024

type frameAssembler struct {
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
	if len(b) <= frameHeaderSize || len(b)-frameHeaderSize > chunkSize {
		return Frame{}, false
	}
	display := int(binary.BigEndian.Uint16(b[42:44])) - 1
	seq := binary.BigEndian.Uint64(b[0:8])
	w, h := binary.BigEndian.Uint32(b[8:12]), binary.BigEndian.Uint32(b[12:16])
	idx, total := binary.BigEndian.Uint32(b[16:20]), binary.BigEndian.Uint32(b[20:24])
	x, y := binary.BigEndian.Uint32(b[32:36]), binary.BigEndian.Uint32(b[36:40])
	keyframe := b[40] == 1
	if total == 0 || total > 1200 || idx >= total || w == 0 || h == 0 || w > 8192 || h > 8192 || uint64(w)*uint64(h) > 32*1024*1024 || b[41] > 2 || b[40] > 1 {
		return Frame{}, false
	}
	if seq < a.seq {
		return Frame{}, false
	}
	if seq != a.seq || a.total == 0 {
		a.display, a.codec = display, b[41]
		a.seq, a.width, a.height, a.x, a.y, a.keyframe, a.total = seq, w, h, x, y, keyframe, total
		a.received, a.bytes, a.complete = 0, 0, false
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
	if a.complete || a.display != display || a.width != w || a.height != h || a.total != total || a.x != x || a.y != y || a.keyframe != keyframe || a.codec != b[41] {
		return Frame{}, false
	}
	payload := b[frameHeaderSize:]
	var data []byte
	if total == 1 {
		data = make([]byte, len(payload))
		copy(data, payload)
	} else {
		if a.chunks[idx] != nil {
			return Frame{}, false
		}
		if a.bytes > maxAssembledFrameBytes-len(payload) {
			clear(a.chunks)
			a.bytes, a.received, a.complete = 0, 0, true
			return Frame{}, false
		}
		a.chunks[idx] = make([]byte, len(payload))
		copy(a.chunks[idx], payload)
		a.bytes += len(payload)
		a.received++
		if a.received != total {
			return Frame{}, false
		}
		data = make([]byte, 0, a.bytes)
		for _, c := range a.chunks {
			data = append(data, c...)
		}
		clear(a.chunks)
	}
	a.complete = true
	return Frame{Display: a.display, Codec: a.codec, Sequence: seq, Width: w, Height: h, X: a.x, Y: a.y, Keyframe: a.keyframe, JPEG: data}, true
}
