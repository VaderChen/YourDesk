#include <assert.h>
#include "../capture_stream_darwin.m"

// Synthetic pixel buffers: no screen-recording permission or desktop capture.
int main(void) { @autoreleasepool {
 YDLiveCapture *capture=[YDLiveCapture new];
 char error[512]={0};void *pixel=NULL;
 assert(yd_capture_take(capture,&pixel,2,2,error,sizeof(error))==0);
 assert(pixel==NULL);

 CVPixelBufferRef source=NULL;
 assert(CVPixelBufferCreate(NULL,2,2,kCVPixelFormatType_32BGRA,NULL,&source)==kCVReturnSuccess);
 assert(CVPixelBufferLockBaseAddress(source,0)==kCVReturnSuccess);
 unsigned char *base=CVPixelBufferGetBaseAddress(source);
 size_t stride=CVPixelBufferGetBytesPerRow(source);
 for(int y=0;y<2;y++)for(int x=0;x<2;x++){
  unsigned char *p=base+y*stride+x*4;p[0]=30;p[1]=20;p[2]=10;p[3]=7;
 }
 CVPixelBufferUnlockBaseAddress(source,0);
 capture->latest=source;capture->revision++;
 assert(yd_capture_take(capture,&pixel,2,2,error,sizeof(error))==1);
 assert(pixel!=NULL);
 void *empty=NULL;
 assert(yd_capture_take(capture,&empty,2,2,error,sizeof(error))==0);
 assert(empty==NULL);

 // Closing capture releases latest, but the taken frame must remain readable.
 yd_capture_close(capture);
 unsigned char rgba[16]={0};
 assert(yd_capture_copy(pixel,rgba,3,2,error,sizeof(error))==-1);
 assert(yd_capture_copy(pixel,rgba,2,2,error,sizeof(error))==1);
 for(int i=0;i<16;i+=4)assert(rgba[i]==10&&rgba[i+1]==20&&rgba[i+2]==30&&rgba[i+3]==255);
 yd_capture_release(pixel);

 capture=[YDLiveCapture new];
 [capture shutdown];
 pixel=NULL;
 assert(yd_capture_take(capture,&pixel,2,2,error,sizeof(error))==-1);
 assert(pixel==NULL);
 [capture release];
 return 0;
} }
