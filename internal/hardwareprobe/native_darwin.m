//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <Metal/Metal.h>
#import <VideoToolbox/VideoToolbox.h>
#import <CoreVideo/CoreVideo.h>
#import <IOKit/ps/IOPowerSources.h>
#include <sys/sysctl.h>
#include <time.h>

static double tick(void){struct timespec t;clock_gettime(CLOCK_MONOTONIC,&t);return t.tv_sec*1000.0+t.tv_nsec/1e6;}
static id number(const char *key){int64_t v=0;size_t n=sizeof(v);if(sysctlbyname(key,&v,&n,NULL,0))return [NSNull null];return @(v);}
static id string(const char *key){char v[256]={0};size_t n=sizeof(v);if(sysctlbyname(key,v,&n,NULL,0))return [NSNull null];return @(v);}
static NSString *four(OSType v){char s[5]={(char)(v>>24),(char)(v>>16),(char)(v>>8),(char)v,0};return [NSString stringWithUTF8String:s]?:@"unknown";}
static id hw(VTSessionRef session,CFStringRef key){CFTypeRef value=NULL;OSStatus s=VTSessionCopyProperty(session,key,NULL,&value);id result=[NSNull null];if(!s&&value&&CFGetTypeID(value)==CFBooleanGetTypeID())result=@(CFBooleanGetValue(value));NSString *type=value?CFBridgingRelease(CFCopyTypeIDDescription(CFGetTypeID(value))):@"none";if(value)CFRelease(value);return @{@"query_status":@(s),@"value":result,@"type":type};}
typedef struct{CMSampleBufferRef sample;OSStatus status;} Encoded;
static void encoded(void *ref,void *source,OSStatus status,VTEncodeInfoFlags flags,CMSampleBufferRef sample){(void)source;(void)flags;Encoded *r=ref;r->status=status;if(sample)r->sample=(CMSampleBufferRef)CFRetain(sample);}
typedef struct{CVImageBufferRef image;OSStatus status;} Decoded;
static void decoded(void *ref,void *source,OSStatus status,VTDecodeInfoFlags flags,CVImageBufferRef image,CMTime pts,CMTime duration){(void)source;(void)flags;(void)pts;(void)duration;Decoded *r=ref;r->status=status;if(image)r->image=CVPixelBufferRetain(image);}

static NSDictionary *inventory(void){
 double start=tick(),t=start;NSMutableDictionary *r=[NSMutableDictionary dictionary],*times=[NSMutableDictionary dictionary];
 NSMutableDictionary *cpu=[NSMutableDictionary dictionary];
 for(NSString *key in @[@"hw.logicalcpu",@"hw.physicalcpu",@"hw.memsize",@"hw.optional.neon",@"hw.optional.arm.FEAT_DotProd",@"hw.optional.arm.FEAT_I8MM",@"hw.optional.arm.FEAT_SVE",@"hw.optional.arm.FEAT_SME",@"hw.optional.avx2_0",@"hw.optional.avx512f",@"hw.optional.sse2",@"hw.optional.supplementalsse3"])cpu[key]=number(key.UTF8String);
 cpu[@"brand"]=string("machdep.cpu.brand_string");cpu[@"machine"]=string("hw.machine");cpu[@"os_build"]=string("kern.osversion");r[@"cpu"]=cpu;times[@"cpu_os_ms"]=@(tick()-t);t=tick();
 NSProcessInfo *process=NSProcessInfo.processInfo;
 CFTypeRef power=IOPSCopyPowerSourcesInfo();CFStringRef source=power?IOPSGetProvidingPowerSourceType(power):NULL;
 r[@"power"]=@{@"source":source?(__bridge NSString*)source:@"unknown",@"low_power":@(process.lowPowerModeEnabled),@"thermal_state":@(process.thermalState)};
 if(power)CFRelease(power);times[@"power_ms"]=@(tick()-t);t=tick();
 NSMutableArray *gpus=[NSMutableArray array];
 for(id<MTLDevice> d in MTLCopyAllDevices()){
  [gpus addObject:@{@"name":d.name,@"registry_id":@(d.registryID),@"unified_memory":@(d.hasUnifiedMemory),@"low_power":@(d.lowPower),@"removable":@(d.removable),@"recommended_working_set":@(d.recommendedMaxWorkingSetSize),@"max_buffer_length":@(d.maxBufferLength)}];
 }r[@"gpus"]=gpus;times[@"metal_enumeration_ms"]=@(tick()-t);t=tick();
 CFArrayRef list=NULL;OSStatus status=VTCopyVideoEncoderList(NULL,&list);r[@"encoder_list_status"]=@(status);r[@"encoders"]=list?CFBridgingRelease(list):@[];times[@"encoder_enumeration_ms"]=@(tick()-t);t=tick();
 NSMutableDictionary *dec=[NSMutableDictionary dictionary];
 CMVideoCodecType codecs[]={kCMVideoCodecType_JPEG,kCMVideoCodecType_H264,kCMVideoCodecType_HEVC,kCMVideoCodecType_AV1};
 for(int i=0;i<4;i++)dec[four(codecs[i])]=@(VTIsHardwareDecodeSupported(codecs[i]));
 r[@"decode_api_claims"]=dec;times[@"decoder_queries_ms"]=@(tick()-t);times[@"total_ms"]=@(tick()-start);r[@"timings"]=times;
 r[@"limitations"]=@"null 表示 sysctl 未提供，不能當成不支援；API 解碼宣告不是實際解碼證明。Metal 裝置資訊不是 GPU 效能／可用 VRAM 量測，未取得獨立驅動版本。";return r;
}

static NSDictionary *probe(NSString *name,NSString *fmt,int width,int height){
 double start=tick(),t=start;NSMutableDictionary *r=[NSMutableDictionary dictionary],*times=[NSMutableDictionary dictionary];
 CMVideoCodecType codec=[name isEqual:@"jpeg"]?kCMVideoCodecType_JPEG:[name isEqual:@"h264"]?kCMVideoCodecType_H264:kCMVideoCodecType_HEVC;
 OSType pixel=[fmt isEqual:@"BGRA"]?kCVPixelFormatType_32BGRA:[fmt isEqual:@"NV12-full"]?kCVPixelFormatType_420YpCbCr8BiPlanarFullRange:kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange;
 r[@"phase"]=@"encode";r[@"encoderBackend"]=@"VideoToolbox";r[@"codec"]=name;r[@"input"]=fmt;r[@"width"]=@(width);r[@"height"]=@(height);
 NSDictionary *attrs=@{(__bridge NSString*)kCVPixelBufferPixelFormatTypeKey:@(pixel),(__bridge NSString*)kCVPixelBufferWidthKey:@(width),(__bridge NSString*)kCVPixelBufferHeightKey:@(height),(__bridge NSString*)kCVPixelBufferIOSurfacePropertiesKey:@{}};
 NSDictionary *spec=@{(__bridge NSString*)kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder:@YES};
 VTCompressionSessionRef session=NULL;Encoded output={NULL,-1};
 OSStatus s=VTCompressionSessionCreate(NULL,width,height,codec,(__bridge CFDictionaryRef)spec,(__bridge CFDictionaryRef)attrs,NULL,encoded,&output,&session);
 r[@"create_status"]=@(s);times[@"encoder_create_ms"]=@(tick()-t);
 if(!s&&session){
  r[@"hardware_encoder"]=hw(session,kVTCompressionPropertyKey_UsingHardwareAcceleratedVideoEncoder);
  r[@"realtime_status"]=@(VTSessionSetProperty(session,kVTCompressionPropertyKey_RealTime,kCFBooleanTrue));
  r[@"no_reorder_status"]=@(VTSessionSetProperty(session,kVTCompressionPropertyKey_AllowFrameReordering,kCFBooleanFalse));
  t=tick();s=VTCompressionSessionPrepareToEncodeFrames(session);r[@"prepare_status"]=@(s);times[@"encoder_prepare_ms"]=@(tick()-t);
  CVPixelBufferRef buffer=NULL;t=tick();
  if(!s)s=CVPixelBufferCreate(NULL,width,height,pixel,(__bridge CFDictionaryRef)attrs,&buffer);
  r[@"buffer_status"]=@(s);
  if(!s&&buffer){
   s=CVPixelBufferLockBaseAddress(buffer,0);r[@"buffer_lock_status"]=@(s);
   if(!s){
    NSMutableArray *rows=[NSMutableArray array];size_t planes=CVPixelBufferGetPlaneCount(buffer);
    if(!planes){size_t row=CVPixelBufferGetBytesPerRow(buffer);[rows addObject:@(row)];uint8_t *p=CVPixelBufferGetBaseAddress(buffer);for(int y=0;y<height;y++)for(int x=0;x<width;x++){p[y*row+4*x]=40;p[y*row+4*x+1]=80;p[y*row+4*x+2]=120;p[y*row+4*x+3]=255;}}
    else for(size_t p=0;p<planes;p++){size_t row=CVPixelBufferGetBytesPerRowOfPlane(buffer,p);[rows addObject:@(row)];memset(CVPixelBufferGetBaseAddressOfPlane(buffer,p),p?128:96,row*CVPixelBufferGetHeightOfPlane(buffer,p));}
    r[@"actual_plane_row_bytes"]=rows;r[@"iosurface_backed"]=@(CVPixelBufferGetIOSurface(buffer)!=NULL);CVPixelBufferUnlockBaseAddress(buffer,0);
   }
  }times[@"buffer_allocate_fill_ms"]=@(tick()-t);t=tick();
  if(!s)s=VTCompressionSessionEncodeFrame(session,buffer,CMTimeMake(0,30),CMTimeMake(1,30),NULL,NULL,NULL);
  if(!s)s=VTCompressionSessionCompleteFrames(session,kCMTimeInvalid);
  r[@"encode_status"]=@(s);r[@"encode_callback_status"]=@(output.status);r[@"sample_produced"]=@(output.sample!=NULL);times[@"first_encode_ms"]=@(tick()-t);
  if(buffer)CVPixelBufferRelease(buffer);
  if(output.sample)r[@"encoded_bytes"]=@(CMSampleBufferGetTotalSampleSize(output.sample));

 }
 if(session){VTCompressionSessionInvalidate(session);CFRelease(session);}if(output.sample)CFRelease(output.sample);
 times[@"total_ms"]=@(tick()-start);r[@"timings"]=times;return r;
}

// 從固定 wire 樣本建立 CM sample；不建立 VTCompressionSession。
static OSStatus fixtureSample(CMVideoCodecType codec,int width,int height,const uint8_t *data,size_t size,CMSampleBufferRef *sample){
 if(!data||!size)return paramErr;
 CMVideoFormatDescriptionRef description=NULL;OSStatus status=0;
 const uint8_t *payload=data;size_t length=size;
 if(codec==kCMVideoCodecType_JPEG){
  status=CMVideoFormatDescriptionCreate(NULL,codec,width,height,NULL,&description);
 }else{
  size_t count=codec==kCMVideoCodecType_H264?2:3;
  if(data[0]!=count)return paramErr;
  const uint8_t *parameters[3]={0};size_t sizes[3]={0};size_t offset=1;
  for(size_t i=0;i<count;i++){
   if(size-offset<4)return paramErr;
   size_t n=((size_t)data[offset]<<24)|((size_t)data[offset+1]<<16)|((size_t)data[offset+2]<<8)|data[offset+3];offset+=4;
   if(!n||n>size-offset)return paramErr;
   parameters[i]=data+offset;sizes[i]=n;offset+=n;
  }
  if(offset>=size)return paramErr;
  if(codec==kCMVideoCodecType_H264)status=CMVideoFormatDescriptionCreateFromH264ParameterSets(NULL,count,parameters,sizes,4,&description);
  else status=CMVideoFormatDescriptionCreateFromHEVCParameterSets(NULL,count,parameters,sizes,4,NULL,&description);
  payload=data+offset;length=size-offset;
 }
 if(status)return status;
 CMBlockBufferRef block=NULL;
 status=CMBlockBufferCreateWithMemoryBlock(NULL,NULL,length,NULL,NULL,0,length,0,&block);
 if(!status)status=CMBlockBufferReplaceDataBytes(payload,block,0,length);
 CMSampleTimingInfo timing={CMTimeMake(1,30),kCMTimeZero,kCMTimeInvalid};
 if(!status)status=CMSampleBufferCreateReady(NULL,block,description,1,1,&timing,1,&length,sample);
 if(block)CFRelease(block);CFRelease(description);return status;
}

static NSDictionary *probeDecode(NSString *name,NSString *fmt,int width,int height,const uint8_t *fixture,size_t size){
 double start=tick(),t=start;NSMutableDictionary *r=[NSMutableDictionary dictionary],*times=[NSMutableDictionary dictionary];
 CMVideoCodecType codec=[name isEqual:@"jpeg"]?kCMVideoCodecType_JPEG:[name isEqual:@"h264"]?kCMVideoCodecType_H264:kCMVideoCodecType_HEVC;
 OSType pixel=[fmt isEqual:@"BGRA"]?kCVPixelFormatType_32BGRA:[fmt isEqual:@"NV12-full"]?kCVPixelFormatType_420YpCbCr8BiPlanarFullRange:kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange;
 r[@"codec"]=name;r[@"input"]=fmt;r[@"output"]=fmt;r[@"width"]=@(width);r[@"height"]=@(height);r[@"phase"]=@"decode";
 r[@"decoderSource"]=@"independent-fixture";r[@"decoderBackend"]=@"VideoToolbox";r[@"decoderAttempted"]=@YES;
 CMSampleBufferRef sample=NULL;OSStatus d=fixtureSample(codec,width,height,fixture,size,&sample);
 r[@"fixture_status"]=@(d);times[@"fixture_ms"]=@(tick()-t);t=tick();
 NSDictionary *dspec=@{(__bridge NSString*)kVTVideoDecoderSpecification_RequireHardwareAcceleratedVideoDecoder:@YES};
 NSDictionary *dattrs=@{(__bridge NSString*)kCVPixelBufferPixelFormatTypeKey:@(pixel),(__bridge NSString*)kCVPixelBufferIOSurfacePropertiesKey:@{}};
 Decoded output={NULL,-1};VTDecompressionOutputCallbackRecord cb={decoded,&output};VTDecompressionSessionRef decoder=NULL;
 if(!d)d=VTDecompressionSessionCreate(NULL,CMSampleBufferGetFormatDescription(sample),(__bridge CFDictionaryRef)dspec,(__bridge CFDictionaryRef)dattrs,&cb,&decoder);
 r[@"decoder_create_status"]=@(d);times[@"decoder_create_ms"]=@(tick()-t);t=tick();
 if(!d&&decoder){d=VTDecompressionSessionDecodeFrame(decoder,sample,0,NULL,NULL);OSStatus wait=VTDecompressionSessionWaitForAsynchronousFrames(decoder);if(!d)d=wait;r[@"hardware_decoder"]=hw(decoder,kVTDecompressionPropertyKey_UsingHardwareAcceleratedVideoDecoder);}
 r[@"decode_status"]=@(d);r[@"decode_callback_status"]=@(output.status);
 BOOL matches=output.image&&CVPixelBufferGetWidth(output.image)==(size_t)width&&CVPixelBufferGetHeight(output.image)==(size_t)height;
 BOOL formatMatches=output.image&&CVPixelBufferGetPixelFormatType(output.image)==pixel;
 r[@"decoded_size_matches"]=@(matches);r[@"decoded_format_matches"]=@(formatMatches);r[@"decodeOK"]=@(!d&&!output.status&&matches&&formatMatches);
 if(output.image){r[@"decoded_format"]=four(CVPixelBufferGetPixelFormatType(output.image));CVPixelBufferRelease(output.image);}
 times[@"first_decode_ms"]=@(tick()-t);
 if(decoder){VTDecompressionSessionInvalidate(decoder);CFRelease(decoder);}if(sample)CFRelease(sample);
 times[@"total_ms"]=@(tick()-start);r[@"timings"]=times;r[@"durationMS"]=@(tick()-start);return r;
}

char *yd_hardware_probe(const char *codec,const char *format,int width,int height,int decode,const unsigned char *fixture,size_t size){@autoreleasepool{
 NSDictionary *r=codec?(decode?probeDecode(@(codec),@(format),width,height,fixture,size):probe(@(codec),@(format),width,height)):inventory();
 NSError *error=nil;NSData *data=[NSJSONSerialization dataWithJSONObject:r options:0 error:&error];
 if(!data)return NULL;char *out=malloc(data.length+1);if(!out)return NULL;memcpy(out,data.bytes,data.length);out[data.length]=0;return out;
}}
