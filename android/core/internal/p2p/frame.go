package p2p

import "encoding/binary"

type frameAssembler struct {
	display       int
	seq           uint64
	width, height uint32
	total         uint32
	x, y          uint32
	keyframe      bool
	codec         byte
	chunks        map[uint32][]byte
}

func (a *frameAssembler) add(b []byte) (Frame, bool) {
	if len(b) < frameHeaderSize {
		return Frame{}, false
	}
	display := int(binary.BigEndian.Uint16(b[42:44])) - 1
	seq := binary.BigEndian.Uint64(b[0:8])
	w := binary.BigEndian.Uint32(b[8:12])
	h := binary.BigEndian.Uint32(b[12:16])
	idx := binary.BigEndian.Uint32(b[16:20])
	total := binary.BigEndian.Uint32(b[20:24])
	x := binary.BigEndian.Uint32(b[32:36])
	y := binary.BigEndian.Uint32(b[36:40])
	keyframe := b[40] == 1
	if total == 0 || total > 1200 || idx >= total || w == 0 || h == 0 || w > 8192 || h > 8192 || uint64(w)*uint64(h) > 32*1024*1024 || b[41] > 2 {
		return Frame{}, false
	}
	if seq < a.seq {
		return Frame{}, false
	}
	if seq != a.seq {
		a.display = display
		a.codec = b[41]
		a.seq, a.width, a.height, a.x, a.y, a.keyframe, a.total, a.chunks = seq, w, h, x, y, keyframe, total, make(map[uint32][]byte, total)
	}
	if a.chunks == nil {
		a.display = display
		a.codec = b[41]
		a.seq, a.width, a.height, a.x, a.y, a.keyframe, a.total, a.chunks = seq, w, h, x, y, keyframe, total, make(map[uint32][]byte, total)
	}
	if a.display != display || a.width != w || a.height != h || a.total != total || a.x != x || a.y != y || a.keyframe != keyframe || a.codec != b[41] {
		return Frame{}, false
	}
	a.chunks[idx] = append([]byte(nil), b[frameHeaderSize:]...)
	if uint32(len(a.chunks)) != total {
		return Frame{}, false
	}
	jpeg := make([]byte, 0, int(total)*chunkSize)
	for i := uint32(0); i < total; i++ {
		c, ok := a.chunks[i]
		if !ok {
			return Frame{}, false
		}
		jpeg = append(jpeg, c...)
	}
	a.chunks = nil
	return Frame{Display: a.display, Codec: a.codec, Sequence: seq, Width: w, Height: h, X: a.x, Y: a.y, Keyframe: a.keyframe, JPEG: jpeg}, true
}
