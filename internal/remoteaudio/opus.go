//go:build cgo && opus

package remoteaudio

/*
#cgo LDFLAGS: -lopus -lm
#include <opus.h>
static int yd_opus_configure(OpusEncoder *encoder, int bitrate) {
    int result = opus_encoder_ctl(encoder, OPUS_SET_BITRATE(bitrate));
    if (result != OPUS_OK) return result;
    result = opus_encoder_ctl(encoder, OPUS_SET_VBR_CONSTRAINT(1));
    if (result != OPUS_OK) return result;
    return opus_encoder_ctl(encoder, OPUS_SET_COMPLEXITY(5));
}
*/
import "C"

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"
)

const opusAvailable = true

type opusDevice struct {
	encoder *C.OpusEncoder
	decoder *C.OpusDecoder
}

func opusError(code C.int) error {
	return fmt.Errorf("Opus：%s", C.GoString(C.opus_strerror(code)))
}

func openOpus(mode, bitrate, preference int) (device, error) {
	if preference == 2 {
		return nil, errors.New("Opus 使用軟體編解碼")
	}
	d := &opusDevice{}
	var code C.int
	switch mode {
	case 1:
		d.encoder = C.opus_encoder_create(SampleRate, Channels, C.OPUS_APPLICATION_AUDIO, &code)
		if code == C.OPUS_OK && d.encoder != nil {
			code = C.yd_opus_configure(d.encoder, C.int(bitrate))
		}
	case 2:
		d.decoder = C.opus_decoder_create(SampleRate, Channels, &code)
	default:
		return nil, errors.New("Opus 模式無效")
	}
	if code != C.OPUS_OK {
		d.close()
		return nil, opusError(code)
	}
	if d.encoder == nil && d.decoder == nil {
		return nil, errors.New("無法建立 Opus 編解碼器")
	}
	return d, nil
}

func (d *opusDevice) hardware() bool { return false }
func (d *opusDevice) close() {
	if d.encoder != nil {
		C.opus_encoder_destroy(d.encoder)
		d.encoder = nil
	}
	if d.decoder != nil {
		C.opus_decoder_destroy(d.decoder)
		d.decoder = nil
	}
}

func (d *opusDevice) process(data []byte) ([]byte, error) {
	pcm := make([]C.opus_int16, OpusFrameSamples*Channels)
	if d.encoder != nil {
		if len(data) != len(pcm)*2 {
			return nil, errors.New("Opus 輸入須為 20 ms 雙聲道 PCM")
		}
		for i := range pcm {
			pcm[i] = C.opus_int16(int16(binary.LittleEndian.Uint16(data[i*2:])))
		}
		out := make([]byte, MaxOpusPacket)
		n := C.opus_encode(d.encoder, &pcm[0], OpusFrameSamples, (*C.uchar)(unsafe.Pointer(&out[0])), C.opus_int32(len(out)))
		if n < 0 {
			return nil, opusError(n)
		}
		return out[:int(n)], nil
	}
	if d.decoder == nil || len(data) == 0 || len(data) > MaxOpusPacket {
		return nil, errors.New("Opus 解碼資料無效")
	}
	packet := (*C.uchar)(unsafe.Pointer(&data[0]))
	// 協定固定 20 ms；先檢查取樣數，防止超長封包放大播放佇列。
	if C.opus_packet_get_nb_samples(packet, C.opus_int32(len(data)), SampleRate) != OpusFrameSamples {
		return nil, errors.New("Opus 封包長度須為 20 ms")
	}
	n := C.opus_decode(d.decoder, packet, C.opus_int32(len(data)), &pcm[0], OpusFrameSamples, 0)
	if n < 0 {
		return nil, opusError(n)
	}
	out := make([]byte, int(n)*Channels*2)
	for i := 0; i < len(out)/2; i++ {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(pcm[i]))
	}
	return out, nil
}
