//go:build cgo

package pixelconv

/*
#include "swizzle.h"
static void yd_swap_rows(unsigned char *dst,const unsigned char *src,int w,int h,size_t ss,size_t ds){
 for(int y=0;y<h;y++)yd_swap_rb_opaque(dst+y*ds,src+y*ss,w);
}
*/
import "C"
import "unsafe"

func swap(dst, src []byte, w, h, ss, ds int) {
	C.yd_swap_rows((*C.uchar)(unsafe.Pointer(&dst[0])), (*C.uchar)(unsafe.Pointer(&src[0])), C.int(w), C.int(h), C.size_t(ss), C.size_t(ds))
}
