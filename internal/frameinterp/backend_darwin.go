//go:build darwin && cgo

package frameinterp

/*
#cgo LDFLAGS: -framework Foundation -framework CoreML -framework CoreVideo -framework VideoToolbox -framework CoreMedia -framework CoreImage -framework CoreGraphics
#include <stdlib.h>
int yd_apple_supported(void);
void yd_apple_working_size(int,int,int*,int*);
int yd_apple_predict(const unsigned char*,const unsigned char*,int,int,int,int,unsigned char*,char*,int);
int yd_rife_load(const char*,char*,int);
int yd_rife_predict(const unsigned char*,const unsigned char*,int,int,int,int,unsigned char*,char*,int);
*/
import "C"
import (
	"embed"
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"unsafe"
)

//go:embed RIFE425Lite.mlpackage
var files embed.FS

func load() error {
	dir, err := os.MkdirTemp("", "yourdesk-rife-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	err = fs.WalkDir(files, "RIFE425Lite.mlpackage", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0600)
	})
	if err != nil {
		return err
	}
	path := C.CString(filepath.Join(dir, "RIFE425Lite.mlpackage"))
	defer C.free(unsafe.Pointer(path))
	var message [1024]C.char
	if C.yd_rife_load(path, &message[0], 1024) == 0 {
		return fmt.Errorf("%s", C.GoString(&message[0]))
	}
	return nil
}
func predict(a, b *image.RGBA) (*image.RGBA, error) {
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	if w < 1 || h < 1 || w > 8192 || h > 8192 {
		return nil, fmt.Errorf("RIFE 尺寸超出限制")
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	var message [1024]C.char
	if C.yd_rife_predict((*C.uchar)(unsafe.Pointer(&a.Pix[0])), (*C.uchar)(unsafe.Pointer(&b.Pix[0])), C.int(w), C.int(h), C.int(a.Stride), C.int(b.Stride), (*C.uchar)(unsafe.Pointer(&out.Pix[0])), &message[0], 1024) == 0 {
		return nil, fmt.Errorf("%s", C.GoString(&message[0]))
	}
	return out, nil
}

func AppleSupported() bool { return C.yd_apple_supported() != 0 }
func applePredict(a, b *image.RGBA) (*image.RGBA, error) {
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	if w < 1 || h < 1 || w > 8192 || h > 8192 {
		return nil, fmt.Errorf("Apple 補幀尺寸超出限制")
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	var message [1024]C.char
	if C.yd_apple_predict((*C.uchar)(unsafe.Pointer(&a.Pix[0])), (*C.uchar)(unsafe.Pointer(&b.Pix[0])), C.int(w), C.int(h), C.int(a.Stride), C.int(b.Stride), (*C.uchar)(unsafe.Pointer(&out.Pix[0])), &message[0], 1024) == 0 {
		return nil, fmt.Errorf("%s", C.GoString(&message[0]))
	}
	return out, nil
}

// AppleWorkingSize 是 Apple 模式所有呈現影格共用的處理尺寸。
func AppleWorkingSize(w, h int) (int, int) {
	var pw, ph C.int
	C.yd_apple_working_size(C.int(w), C.int(h), &pw, &ph)
	return int(pw), int(ph)
}
