//go:build darwin && cgo

package superres

/*
#cgo LDFLAGS: -framework Foundation -framework CoreML -framework CoreVideo
#include <stdlib.h>
int yd_sr_load(int modelID, const char *path, char *error, int capacity);
int yd_sr_predict(int modelID, const unsigned char *src,int width,int height,int stride,unsigned char *dst,char *error,int capacity);
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

// QuickSRNet Small 與 SESR M5 2×（BSD-3-Clause），FP16 Core ML 部署模型。
//
//go:embed QuickSRNetSmall_2x.mlpackage SESR_M5_2x.mlpackage
var modelFiles embed.FS

func loadModel(id int) error {
	packageName := []string{"QuickSRNetSmall_2x.mlpackage", "SESR_M5_2x.mlpackage"}[id]
	dir, err := os.MkdirTemp("", "yourdesk-coreml-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	err = fs.WalkDir(modelFiles, packageName, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		b, e := modelFiles.ReadFile(path)
		if e != nil {
			return e
		}
		return os.WriteFile(target, b, 0600)
	})
	if err != nil {
		return err
	}
	path := C.CString(filepath.Join(dir, packageName))
	defer C.free(unsafe.Pointer(path))
	var message [1024]C.char
	if C.yd_sr_load(C.int(id), path, &message[0], 1024) == 0 {
		return fmt.Errorf("%s", C.GoString(&message[0]))
	}
	return nil
}
func predict(id int, src *image.RGBA) (*image.RGBA, error) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if w < 1 || h < 1 || w > 4096 || h > 4096 {
		return nil, fmt.Errorf("Core ML 輸入尺寸超出限制")
	}
	out := image.NewRGBA(image.Rect(0, 0, w*2, h*2))
	var message [1024]C.char
	if C.yd_sr_predict(C.int(id), (*C.uchar)(unsafe.Pointer(&src.Pix[0])), C.int(w), C.int(h), C.int(src.Stride), (*C.uchar)(unsafe.Pointer(&out.Pix[0])), &message[0], 1024) == 0 {
		return nil, fmt.Errorf("%s", C.GoString(&message[0]))
	}
	return out, nil
}
