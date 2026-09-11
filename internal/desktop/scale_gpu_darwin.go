//go:build darwin && cgo

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework CoreImage -framework Metal -framework CoreGraphics
#import <CoreImage/CoreImage.h>
#import <Metal/Metal.h>

static void *yd_scale_create(void) {
 @autoreleasepool {
  @try {
   id<MTLDevice> device=MTLCreateSystemDefaultDevice();
   if (!device) return NULL;
   CIContext *context=[[CIContext contextWithMTLDevice:device options:@{kCIContextUseSoftwareRenderer:@NO,kCIContextCacheIntermediates:@NO,kCIContextWorkingColorSpace:[NSNull null]}] retain];
   [device release];
   return context;
  } @catch(NSException *exception) { return NULL; }
 }
}
static void yd_scale_close(void *handle) { [(CIContext *)handle release]; }
static int yd_scale_render(void *handle,const unsigned char *src,int sw,int sh,int stride,unsigned char *dst,int dw,int dh,int dstStride) {
 @autoreleasepool {
  @try {
   // 複製輸入交由 NSData 持有，避免 Core Image 延遲釋放時保留 Go 記憶體。
   NSData *data=[NSData dataWithBytes:src length:(NSUInteger)stride*sh];
   CGColorSpaceRef color=CGColorSpaceCreateDeviceRGB();
   CIImage *input=[CIImage imageWithBitmapData:data bytesPerRow:stride size:CGSizeMake(sw,sh) format:kCIFormatRGBA8 colorSpace:color];
   CIFilter *filter=[CIFilter filterWithName:@"CILanczosScaleTransform"];
   [filter setValue:input forKey:kCIInputImageKey];
   [filter setValue:@((double)dh/sh) forKey:kCIInputScaleKey];
   [filter setValue:@(((double)dw/sw)/((double)dh/sh)) forKey:kCIInputAspectRatioKey];
   CIImage *output=filter.outputImage;
   if (!output) { CGColorSpaceRelease(color); return 0; }
   // 同步讀回縮小後的像素；來源與目的均維持 RGBA 的列順序。
   [(CIContext *)handle render:output toBitmap:dst rowBytes:dstStride bounds:CGRectMake(0,0,dw,dh) format:kCIFormatRGBA8 colorSpace:color];
   CGColorSpaceRelease(color);
   return 1;
  } @catch(NSException *exception) { return 0; }
 }
}
*/
import "C"
import (
	"fmt"
	"image"
	"unsafe"
)

type metalScaler struct{ context unsafe.Pointer }

func newGPUScaler() (gpuScaler, error) {
	context := C.yd_scale_create()
	if context == nil {
		return nil, fmt.Errorf("Metal 縮圖器無法初始化")
	}
	return &metalScaler{context: context}, nil
}
func (s *metalScaler) Scale(src, dst *image.RGBA) error {
	if C.yd_scale_render(s.context, (*C.uchar)(unsafe.Pointer(&src.Pix[0])), C.int(src.Bounds().Dx()), C.int(src.Bounds().Dy()), C.int(src.Stride), (*C.uchar)(unsafe.Pointer(&dst.Pix[0])), C.int(dst.Bounds().Dx()), C.int(dst.Bounds().Dy()), C.int(dst.Stride)) == 0 {
		return fmt.Errorf("Metal 縮圖失敗")
	}
	return nil
}
func (s *metalScaler) Close() { C.yd_scale_close(s.context); s.context = nil }
