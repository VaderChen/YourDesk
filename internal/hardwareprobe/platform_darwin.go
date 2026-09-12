//go:build darwin && cgo

package hardwareprobe

/*
#cgo CFLAGS: -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Foundation -framework Metal -framework VideoToolbox -framework CoreMedia -framework CoreVideo -framework IOKit
#include <stdlib.h>
char *yd_hardware_probe(const char *,const char *,int,int,int,const unsigned char *,size_t);
*/
import "C"
import (
	"errors"
	"unsafe"
	"yourdesk/internal/video"
)

func platformJobs() []job {
	jobs := []job{{}}
	for _, codec := range []string{"jpeg", "h264", "hevc"} {
		for _, format := range []string{"BGRA", "NV12-full", "NV12-video"} {
			for _, size := range [][2]int{{128, 128}, {1920, 1080}} {
				jobs = append(jobs, job{"", codec, format, size[0], size[1]})
			}
		}
	}
	return splitJobs(jobs)
}
func platformProbe(j job) ([]byte, error) {
	var codec, format *C.char
	if j.Codec != "" {
		codec = C.CString(j.Codec)
		format = C.CString(j.Format)
		defer C.free(unsafe.Pointer(codec))
		defer C.free(unsafe.Pointer(format))
	}
	var fixture []byte
	var source *C.uchar
	var direction C.int
	if j.Phase == "decode" {
		var err error
		fixture, err = video.ProbeWireFixture(j.Codec, j.Width, j.Height)
		if err != nil {
			return nil, err
		}
		source = (*C.uchar)(unsafe.Pointer(&fixture[0]))
		direction = 1
	}
	data := C.yd_hardware_probe(codec, format, C.int(j.Width), C.int(j.Height), direction, source, C.size_t(len(fixture)))
	if data == nil {
		return nil, errors.New("原生能力查詢失敗")
	}
	defer C.free(unsafe.Pointer(data))
	return []byte(C.GoString(data)), nil
}
