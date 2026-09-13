#ifndef YD_INTRA_DARWIN_H
#define YD_INTRA_DARWIN_H
#include <VideoToolbox/VideoToolbox.h>
#include <CoreMedia/CoreMedia.h>
#include <CoreVideo/CoreVideo.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include "../pixelconv/swizzle.h"

typedef struct { unsigned char *data; size_t size; OSStatus status; } yd_intra_result;
static void yd_put32(unsigned char *p, uint32_t v) { p[0]=v>>24;p[1]=v>>16;p[2]=v>>8;p[3]=v; }
static uint32_t yd_get32(const unsigned char *p) { return ((uint32_t)p[0]<<24)|((uint32_t)p[1]<<16)|((uint32_t)p[2]<<8)|p[3]; }
static void yd_intra_callback(void *ref,void *source,OSStatus status,VTEncodeInfoFlags flags,CMSampleBufferRef sample) {
 yd_intra_result *r=source;
 if(!r)return;
 r->status=status;
 if(status||!sample) { r->status=-1;return; }
 CMFormatDescriptionRef format=CMSampleBufferGetFormatDescription(sample);
 CMBlockBufferRef block=CMSampleBufferGetDataBuffer(sample);
 if(!format||!block) {r->status=-1;return;}
 const uint8_t *params[3];size_t lengths[3],count=0;int nalu=0;
 CMVideoCodecType codec=CMFormatDescriptionGetMediaSubType(format);
 if(codec==kCMVideoCodecType_AV1) {
  size_t bytes=CMBlockBufferGetDataLength(block),extra=0;const unsigned char *config=NULL;
  CFDictionaryRef atoms=CMFormatDescriptionGetExtension(format,kCMFormatDescriptionExtension_SampleDescriptionExtensionAtoms);
  CFDataRef av1c=atoms?CFDictionaryGetValue(atoms,CFSTR("av1C")):NULL;
  if(av1c && CFGetTypeID(av1c)==CFDataGetTypeID() && CFDataGetLength(av1c)>4){extra=CFDataGetLength(av1c)-4;config=CFDataGetBytePtr(av1c)+4;}
  if(!bytes || bytes>32*1024*1024 || extra>32*1024*1024-bytes){r->status=-1;return;}
  unsigned char *data=malloc(bytes+extra);if(!data){r->status=-1;return;}
  if(extra)memcpy(data,config,extra);
  status=CMBlockBufferCopyDataBytes(block,0,bytes,data+extra);
  if(status){free(data);r->status=status;return;}
  r->data=data;r->size=bytes+extra;return;
 }
 for(size_t i=0;i<(codec==kCMVideoCodecType_HEVC?3:2);i++) {
  status=codec==kCMVideoCodecType_HEVC ? CMVideoFormatDescriptionGetHEVCParameterSetAtIndex(format,i,&params[i],&lengths[i],&count,&nalu) : CMVideoFormatDescriptionGetH264ParameterSetAtIndex(format,i,&params[i],&lengths[i],&count,&nalu);
  if(status) {r->status=status;return;}
 }
 if(nalu!=4||count!=(codec==kCMVideoCodecType_HEVC?3:2)) {r->status=-1;return;}
 size_t bytes=CMBlockBufferGetDataLength(block),total=1+bytes;
 for(size_t i=0;i<count;i++) total+=4+lengths[i];
 if(!bytes||total>32*1024*1024) {r->status=-1;return;}
 unsigned char *data=malloc(total);if(!data){r->status=-1;return;}
 data[0]=(unsigned char)count;size_t pos=1;
 for(size_t i=0;i<count;i++){yd_put32(data+pos,(uint32_t)lengths[i]);pos+=4;memcpy(data+pos,params[i],lengths[i]);pos+=lengths[i];}
 status=CMBlockBufferCopyDataBytes(block,0,bytes,data+pos);
 if(status){free(data);r->status=status;return;}
 r->data=data;r->size=total;
}
static void yd_intra_close(VTCompressionSessionRef session) {VTCompressionSessionInvalidate(session);CFRelease(session);}
static void yd_pixel_close(uintptr_t pixel) { CVPixelBufferRelease((CVPixelBufferRef)pixel); }
static OSStatus yd_intra_create(int w,int h,CMVideoCodecType codec,int gop,VTCompressionSessionRef *out) {
 *out=NULL;
 const void *key=kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder;
 const void *value=kCFBooleanTrue;
 CFDictionaryRef spec=CFDictionaryCreate(NULL,&key,&value,1,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 VTCompressionSessionRef session=NULL;
 OSStatus s=VTCompressionSessionCreate(NULL,w,h,codec,spec,NULL,NULL,yd_intra_callback,NULL,&session);
 CFRelease(spec);if(s)return s;
 s=VTSessionSetProperty(session,kVTCompressionPropertyKey_RealTime,kCFBooleanTrue);
 if(!s)s=VTSessionSetProperty(session,kVTCompressionPropertyKey_AllowFrameReordering,kCFBooleanFalse);
 CFNumberRef interval=CFNumberCreate(NULL,kCFNumberIntType,&gop);
 if(!s)s=VTSessionSetProperty(session,kVTCompressionPropertyKey_MaxKeyFrameInterval,interval);
 CFRelease(interval);
 if(!s)s=VTCompressionSessionPrepareToEncodeFrames(session);
 CFTypeRef hardware=NULL;
 if(!s)s=VTSessionCopyProperty(session,kVTCompressionPropertyKey_UsingHardwareAcceleratedVideoEncoder,NULL,&hardware);
 if(!s&&(!hardware||CFGetTypeID(hardware)!=CFBooleanGetTypeID()||!CFBooleanGetValue(hardware)))s=-1;
 if(hardware)CFRelease(hardware);
 if(s){yd_intra_close(session);return s;}
 *out=session;return noErr;
}
static OSStatus yd_intra_rate(VTCompressionSessionRef session,int bitrate,int fps) {
 CFNumberRef rate=CFNumberCreate(NULL,kCFNumberIntType,&bitrate);
 OSStatus s=VTSessionSetProperty(session,kVTCompressionPropertyKey_AverageBitRate,rate);
 CFRelease(rate);
 CFNumberRef frames=CFNumberCreate(NULL,kCFNumberIntType,&fps);
 if(!s)s=VTSessionSetProperty(session,kVTCompressionPropertyKey_ExpectedFrameRate,frames);
 CFRelease(frames);return s;
}
static OSStatus yd_intra_encode(VTCompressionSessionRef session,uintptr_t *cached,const unsigned char *rgba,int w,int h,int stride,int64_t sequence,int quality,int fps,int gop,unsigned char **out,size_t *len) {
 *out=NULL;*len=0;
 CVPixelBufferRef pixel=(CVPixelBufferRef)*cached;
 OSStatus s=noErr;
 if(!pixel){s=CVPixelBufferCreate(NULL,w,h,kCVPixelFormatType_32BGRA,NULL,&pixel);if(!s)*cached=(uintptr_t)pixel;}
 if(s)return s;
 s=CVPixelBufferLockBaseAddress(pixel,0);
 if(s)return s;
 unsigned char *base=CVPixelBufferGetBaseAddress(pixel);size_t row=CVPixelBufferGetBytesPerRow(pixel);
 for(int y=0;y<h;y++)yd_swap_rb_opaque(base+y*row,rgba+y*stride,w);
 CVPixelBufferUnlockBaseAddress(pixel,0);
 float q=(quality<1?1:quality>100?100:quality)/100.0f;
 CFNumberRef qualityNumber=CFNumberCreate(NULL,kCFNumberFloatType,&q);
 // 部分硬體不提供 Quality；此屬性非核心建立條件。
 VTSessionSetProperty(session,kVTCompressionPropertyKey_Quality,qualityNumber);CFRelease(qualityNumber);
 const void *key=kVTEncodeFrameOptionKey_ForceKeyFrame,*value=((sequence-1)%gop==0?kCFBooleanTrue:kCFBooleanFalse);
 CFDictionaryRef options=CFDictionaryCreate(NULL,&key,&value,1,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 yd_intra_result result={NULL,0,-1};
 s=VTCompressionSessionEncodeFrame(session,pixel,CMTimeMake(sequence,fps),CMTimeMake(1,fps),options,&result,NULL);
 CFRelease(options);
 OSStatus complete=VTCompressionSessionCompleteFrames(session,kCMTimeInvalid);
 // CompleteFrames 已同步完成，下一張才可覆寫同一 pixel buffer。
 if(!s)s=complete;if(!s)s=result.status;
 if(s){free(result.data);return s;}
 *out=result.data;*len=result.size;return noErr;
}
typedef struct { CVPixelBufferRef image; OSStatus status; } yd_intra_decoded;
static void yd_intra_decode_callback(void *ref,void *source,OSStatus status,VTDecodeInfoFlags flags,CVImageBufferRef image,CMTime pts,CMTime duration) {
 yd_intra_decoded *r=source;r->status=status;
 if(!status&&image)r->image=CVPixelBufferRetain(image);
}
typedef struct { VTDecompressionSessionRef session; CMVideoFormatDescriptionRef format; } yd_decoder;
typedef struct { int width,height,mode; } yd_decode_policy;
static void yd_decoder_close(yd_decoder *d) { if(d->session){VTDecompressionSessionInvalidate(d->session);CFRelease(d->session);} if(d->format)CFRelease(d->format); d->session=NULL;d->format=NULL; }
static OSStatus yd_intra_decode(yd_decoder *decoder,CMVideoCodecType codec,const unsigned char *data,size_t length,const yd_decode_policy *policy,int policyCount,unsigned char **out,int *width,int *height,int *hardware) {
 *out=NULL;*width=0;*height=0;*hardware=-1;
 if(length<1||data[0]!=(codec==kCMVideoCodecType_HEVC?3:2))return -1;
 size_t count=data[0],pos=1,sizes[3];const uint8_t *params[3];
 for(size_t i=0;i<count;i++){
  if(length-pos<4)return -1;sizes[i]=yd_get32(data+pos);pos+=4;
  if(!sizes[i]||sizes[i]>length-pos)return -1;params[i]=data+pos;pos+=sizes[i];
 }
 if(pos>=length)return -1;
 CMVideoFormatDescriptionRef format=NULL;
 OSStatus s=codec==kCMVideoCodecType_HEVC ? CMVideoFormatDescriptionCreateFromHEVCParameterSets(NULL,count,params,sizes,4,NULL,&format) : CMVideoFormatDescriptionCreateFromH264ParameterSets(NULL,count,params,sizes,4,&format);
 if(s)return s;
 CMVideoDimensions dims=CMVideoFormatDescriptionGetDimensions(format);
 if(dims.width<=0||dims.height<=0||dims.width>8192||dims.height>8192||(int64_t)dims.width*dims.height>32*1024*1024){CFRelease(format);return -1;}
 // 使用壓縮參數集的真實尺寸，而非可由對端任意填寫的外層尺寸。
 int preference=0;
 for(int i=0;i<policyCount;i++)if(policy[i].width==dims.width&&policy[i].height==dims.height){preference=policy[i].mode;break;}
 if(preference==-2){CFRelease(format);return -1;}
 int pixelFormat=kCVPixelFormatType_32BGRA;CFNumberRef pixelNumber=CFNumberCreate(NULL,kCFNumberIntType,&pixelFormat);
 const void *attrKey=kCVPixelBufferPixelFormatTypeKey,*attrValue=pixelNumber;
 CFDictionaryRef attrs=CFDictionaryCreate(NULL,&attrKey,&attrValue,1,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 CFRelease(pixelNumber);
 yd_intra_decoded result={NULL,-1};VTDecompressionOutputCallbackRecord cb={yd_intra_decode_callback,NULL};
 VTDecompressionSessionRef session=decoder->session;
 if(session&&!CMFormatDescriptionEqual(decoder->format,format)){yd_decoder_close(decoder);session=NULL;}
 // 已驗證硬解時優先啟用；硬解探測失敗時先用軟解；未測尺寸維持系統自選。
 if(!session){
  CFDictionaryRef spec=NULL;
  if(preference){const void *key=kVTVideoDecoderSpecification_EnableHardwareAcceleratedVideoDecoder,*value=preference>0?kCFBooleanTrue:kCFBooleanFalse;spec=CFDictionaryCreate(NULL,&key,&value,1,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);}
  s=VTDecompressionSessionCreate(NULL,format,spec,attrs,&cb,&session);
  if(spec)CFRelease(spec);
  // 策略不是保證：驅動／資源狀態改變時，允許既有自選路徑再建立一次。
  if(s&&preference){if(session){VTDecompressionSessionInvalidate(session);CFRelease(session);session=NULL;}s=VTDecompressionSessionCreate(NULL,format,NULL,attrs,&cb,&session);}
  if(!s){decoder->session=session;decoder->format=(CMVideoFormatDescriptionRef)CFRetain(format);}
 }CFRelease(attrs);
 CMBlockBufferRef block=NULL;CMSampleBufferRef sample=NULL;size_t bytes=length-pos;
 if(!s)s=CMBlockBufferCreateWithMemoryBlock(NULL,NULL,bytes,NULL,NULL,0,bytes,0,&block);
 if(!s)s=CMBlockBufferReplaceDataBytes(data+pos,block,0,bytes);
 if(!s)s=CMSampleBufferCreateReady(NULL,block,format,1,0,NULL,1,&bytes,&sample);
 if(!s)s=VTDecompressionSessionDecodeFrame(session,sample,0,&result,NULL);
 if(session){OSStatus wait=VTDecompressionSessionWaitForAsynchronousFrames(session);if(!s)s=wait;}
 if(!s&&session){
  CFTypeRef used=NULL;
  if(VTSessionCopyProperty(session,kVTDecompressionPropertyKey_UsingHardwareAcceleratedVideoDecoder,NULL,&used)==noErr&&used){
   if(CFGetTypeID(used)==CFBooleanGetTypeID())*hardware=CFBooleanGetValue((CFBooleanRef)used)?1:0;
   CFRelease(used);
  }
 }
 if(!s)s=result.status;
 if(!s&&!result.image)s=-1;
 if(!s){
  s=CVPixelBufferLockBaseAddress(result.image,kCVPixelBufferLock_ReadOnly);
  if(!s){
   size_t w=CVPixelBufferGetWidth(result.image),h=CVPixelBufferGetHeight(result.image),row=CVPixelBufferGetBytesPerRow(result.image);
   const unsigned char *base=CVPixelBufferGetBaseAddress(result.image);
   if(w!=(size_t)dims.width||h!=(size_t)dims.height||!base)s=-1;
   unsigned char *pixels=s?NULL:malloc(w*h*4);
   if(!pixels)s=-1;
   if(!s){for(size_t y=0;y<h;y++)yd_swap_rb_opaque(pixels+y*w*4,base+y*row,w);
   *out=pixels;*width=(int)w;*height=(int)h;}
   CVPixelBufferUnlockBaseAddress(result.image,kCVPixelBufferLock_ReadOnly);
  }
 }
 if(result.image)CFRelease(result.image);
 if(sample)CFRelease(sample);if(block)CFRelease(block);
 CFRelease(format);return s;
}
#endif
