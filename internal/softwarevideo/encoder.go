//go:build cgo && ffmpeg

package softwarevideo

/*
#include <libavcodec/avcodec.h>
#include <libavutil/opt.h>
#include <libswscale/swscale.h>
#include <stdlib.h>
typedef struct { AVCodecContext *codec; AVFrame *frame; struct SwsContext *scale; int64_t pts; } yd_av1_encoder;
static void yd_av1_encoder_close(yd_av1_encoder *e) {
 if (!e) return;
 avcodec_free_context(&e->codec); av_frame_free(&e->frame); sws_freeContext(e->scale); free(e);
}
static yd_av1_encoder *yd_av1_encoder_open(int w,int h,int rate,int fps,int gop,int quality) {
 const AVCodec *codec=avcodec_find_encoder_by_name("libaom-av1");
 if (!codec) return NULL;
 yd_av1_encoder *e=calloc(1,sizeof(*e)); if (!e) return NULL;
 e->codec=avcodec_alloc_context3(codec); e->frame=av_frame_alloc();
 if (!e->codec || !e->frame) { yd_av1_encoder_close(e);return NULL; }
 AVCodecContext *c=e->codec;
 c->width=w;c->height=h;c->pix_fmt=AV_PIX_FMT_YUV420P;
 c->time_base=(AVRational){1,fps};c->framerate=(AVRational){fps,1};
 c->gop_size=gop;c->max_b_frames=0;c->thread_count=4;c->bit_rate=rate;
 c->color_range=AVCOL_RANGE_MPEG;c->colorspace=AVCOL_SPC_BT709;
 c->color_primaries=AVCOL_PRI_BT709;c->color_trc=AVCOL_TRC_BT709;
 AVDictionary *opts=NULL;
 av_dict_set(&opts,"usage","realtime",0);av_dict_set(&opts,"cpu-used","8",0);
 av_dict_set(&opts,"lag-in-frames","0",0);av_dict_set(&opts,"row-mt","1",0);
 if (!rate) av_dict_set_int(&opts,"crf",63-(quality*59/100),0);
 int status=avcodec_open2(c,codec,&opts);av_dict_free(&opts);
 if (status<0) { yd_av1_encoder_close(e);return NULL; }
 e->frame->format=c->pix_fmt;e->frame->width=w;e->frame->height=h;
 e->frame->color_range=c->color_range;e->frame->colorspace=c->colorspace;
 if (av_frame_get_buffer(e->frame,32)<0) { yd_av1_encoder_close(e);return NULL; }
 e->scale=sws_getContext(w,h,AV_PIX_FMT_RGBA,w,h,AV_PIX_FMT_YUV420P,SWS_BILINEAR,NULL,NULL,NULL);
 if (!e->scale) { yd_av1_encoder_close(e);return NULL; }
 const int *coeff=sws_getCoefficients(SWS_CS_ITU709);
 if (sws_setColorspaceDetails(e->scale,coeff,1,coeff,0,0,1<<16,1<<16)<0) { yd_av1_encoder_close(e);return NULL; }
 return e;
}
static int yd_av1_encoder_encode(yd_av1_encoder *e,const unsigned char *rgba,int stride,AVPacket *packet) {
 int status=av_frame_make_writable(e->frame);if (status<0)return status;
 const unsigned char *planes[4]={rgba,NULL,NULL,NULL};int strides[4]={stride,0,0,0};
 if (sws_scale(e->scale,planes,strides,0,e->codec->height,e->frame->data,e->frame->linesize)!=e->codec->height)return AVERROR(EINVAL);
 e->frame->pts=e->pts++;
 status=avcodec_send_frame(e->codec,e->frame);if(status<0)return status;
 return avcodec_receive_packet(e->codec,packet);
}
*/
import "C"
import (
	"fmt"
	"image"
	"sync"
	"unsafe"
)

type Encoder struct {
	mu                                     sync.Mutex
	native                                 *C.yd_av1_encoder
	width, height, rate, fps, gop, quality int
	closed                                 bool
}

func NewAV1Encoder() (*Encoder, error) {
	name := C.CString("libaom-av1")
	defer C.free(unsafe.Pointer(name))
	if C.avcodec_find_encoder_by_name(name) == nil {
		return nil, fmt.Errorf("FFmpeg AV1 軟體編碼器未啟用")
	}
	return &Encoder{}, nil
}
func (e *Encoder) Encode(src *image.RGBA, rate, fps, gop, quality int) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || src == nil {
		return nil, fmt.Errorf("AV1 編碼器已關閉或影格為空")
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if w <= 0 || h <= 0 || w > 8192 || h > 8192 || w*h > 32<<20 || w%2 != 0 || h%2 != 0 || src.Stride < w*4 || len(src.Pix) < (h-1)*src.Stride+w*4 {
		return nil, fmt.Errorf("AV1 影格尺寸或像素資料無效")
	}
	rate = max(0, rate)
	fps = max(1, min(240, fps))
	gop = max(1, min(300, gop))
	quality = max(1, min(100, quality))
	if e.native == nil || e.width != w || e.height != h || e.rate != rate || e.fps != fps || e.gop != gop || (rate == 0 && e.quality != quality) {
		C.yd_av1_encoder_close(e.native)
		e.native = nil
		e.native = C.yd_av1_encoder_open(C.int(w), C.int(h), C.int(rate), C.int(fps), C.int(gop), C.int(quality))
		if e.native == nil {
			return nil, fmt.Errorf("FFmpeg AV1 編碼器建立失敗")
		}
		e.width, e.height, e.rate, e.fps, e.gop, e.quality = w, h, rate, fps, gop, quality
	}
	packet := C.av_packet_alloc()
	if packet == nil {
		return nil, fmt.Errorf("AV1 封包配置失敗")
	}
	defer C.av_packet_free(&packet)
	status := C.yd_av1_encoder_encode(e.native, (*C.uchar)(unsafe.Pointer(&src.Pix[0])), C.int(src.Stride), packet)
	if status < 0 {
		var msg [256]C.char
		C.av_strerror(status, &msg[0], 256)
		return nil, fmt.Errorf("FFmpeg AV1 編碼失敗：%s", C.GoString(&msg[0]))
	}
	if packet.size <= 0 || packet.size > 32<<20 {
		return nil, fmt.Errorf("AV1 輸出大小無效")
	}
	return C.GoBytes(unsafe.Pointer(packet.data), packet.size), nil
}
func (e *Encoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	C.yd_av1_encoder_close(e.native)
	e.native = nil
	e.closed = true
	return nil
}
