#import <Foundation/Foundation.h>
#import <VideoToolbox/VideoToolbox.h>
#import <CoreImage/CoreImage.h>
#include <stdio.h>
#include <math.h>
// 呼叫端以 inference mutex 序列化；重用 session 和 IOSurface，尺寸改變時釋放。
static VTFrameProcessor *processor;
static CIContext *ci;
static CVPixelBufferRef inputs[2],output;
static int width,height;
int yd_apple_supported(void){
#if defined(__arm64__)
 if(@available(macOS 26.0,*))return [VTLowLatencyFrameInterpolationConfiguration isSupported];
#endif
 return 0;
}
static void clearSession(void){
 if(processor){[processor endSession];[processor release];processor=nil;}
 for(int i=0;i<2;i++){if(inputs[i])CVPixelBufferRelease(inputs[i]);inputs[i]=NULL;}
 if(output)CVPixelBufferRelease(output);output=NULL;width=height=0;
}
static CVPixelBufferRef makeBuffer(NSDictionary *attrs,int w,int h,OSType fmt){
 NSMutableDictionary *d=[NSMutableDictionary dictionaryWithDictionary:attrs];d[(id)kCVPixelBufferIOSurfacePropertiesKey]=@{};
 CVPixelBufferRef b=NULL;CVPixelBufferCreate(NULL,w,h,fmt,(CFDictionaryRef)d,&b);return b;
}
// 真實幀與生成幀共用尺寸規則，避免清晰度交替。
void yd_apple_working_size(int w,int h,int *pw,int *ph){
 // macOS 26 沒有尺寸能力查詢；使用本機已驗證的 720p 工作預算。
 // 較新系統可查詢能力，但仍限制即時處理預算，避免 Core ML 放大後超量。
 double edge=1280.0,pixels=1280.0*720.0;
 if(@available(macOS 27.0,*)){
  NSInteger limit=[VTLowLatencyFrameInterpolationConfiguration maximumDimensionForSpatialScaleFactor:1];
  NSInteger count=[VTLowLatencyFrameInterpolationConfiguration maximumPixelCountForSpatialScaleFactor:1];
  if(limit>0)edge=fmin(edge,limit);if(count>0)pixels=fmin(pixels,count);
 }
 double scale=fmin(1.0,fmin(edge/fmax(w,h),sqrt(pixels/((double)w*h))));
 *pw=MAX(2,((int)floor(w*scale)/2)*2);
 *ph=MAX(2,((int)floor(h*scale)/2)*2);
}
int yd_apple_predict(const unsigned char *a,const unsigned char *b,int w,int h,int as,int bs,unsigned char *out,char *error,int n){@autoreleasepool{
 if(!yd_apple_supported()){snprintf(error,n,"Apple 補幀需要支援的 Mac 與 macOS 26 以上");return 0;}
 if(@available(macOS 26.0,*)){
 NSError *err=nil;
 int pw,ph;yd_apple_working_size(w,h,&pw,&ph);
 if(!processor||width!=pw||height!=ph){
  clearSession();
  VTLowLatencyFrameInterpolationConfiguration *c=[[VTLowLatencyFrameInterpolationConfiguration alloc] initWithFrameWidth:pw frameHeight:ph numberOfInterpolatedFrames:1];
  if(!c){snprintf(error,n,"Apple 補幀不支援此尺寸");return 0;}
  processor=[[VTFrameProcessor alloc]init];
  BOOL ok=[processor startSessionWithConfiguration:c error:&err];
  OSType fmt=[c.frameSupportedPixelFormats.firstObject unsignedIntValue];
  if(ok){inputs[0]=makeBuffer(c.sourcePixelBufferAttributes,pw,ph,fmt);inputs[1]=makeBuffer(c.sourcePixelBufferAttributes,pw,ph,fmt);output=makeBuffer(c.destinationPixelBufferAttributes,pw,ph,fmt);}
  [c release];
  if(!ok||!inputs[0]||!inputs[1]||!output){snprintf(error,n,"%s",err.localizedDescription.UTF8String?:"Apple 補幀緩衝配置失敗");clearSession();return 0;}
  width=pw;height=ph;
 }
 if(!ci)ci=[[CIContext contextWithOptions:@{kCIContextWorkingColorSpace:[NSNull null]}]retain];
 CGColorSpaceRef cs=CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
 for(int i=0;i<2;i++){
  NSData *data=[NSData dataWithBytes:i?b:a length:(size_t)(i?bs:as)*h];
  CIImage *im=[CIImage imageWithBitmapData:data bytesPerRow:i?bs:as size:CGSizeMake(w,h) format:kCIFormatRGBA8 colorSpace:cs];
  im=[im imageByApplyingTransform:CGAffineTransformMakeScale((double)pw/w,(double)ph/h)];
  [ci render:im toCVPixelBuffer:inputs[i] bounds:CGRectMake(0,0,pw,ph) colorSpace:cs];
 }
 VTFrameProcessorFrame *af=[[VTFrameProcessorFrame alloc]initWithBuffer:inputs[0] presentationTimeStamp:CMTimeMake(0,24)];
 VTFrameProcessorFrame *bf=[[VTFrameProcessorFrame alloc]initWithBuffer:inputs[1] presentationTimeStamp:CMTimeMake(2,24)];
 VTFrameProcessorFrame *df=[[VTFrameProcessorFrame alloc]initWithBuffer:output presentationTimeStamp:CMTimeMake(1,24)];
 VTLowLatencyFrameInterpolationParameters *params=[[VTLowLatencyFrameInterpolationParameters alloc]initWithSourceFrame:bf previousFrame:af interpolationPhase:@[@0.5] destinationFrames:@[df]];
 BOOL ok=[processor processWithParameters:params error:&err];
 if(ok){CIImage *result=[[CIImage imageWithCVPixelBuffer:output] imageByApplyingTransform:CGAffineTransformMakeScale((double)w/pw,(double)h/ph)];[ci render:result toBitmap:out rowBytes:w*4 bounds:CGRectMake(0,0,w,h) format:kCIFormatRGBA8 colorSpace:cs];}
 else snprintf(error,n,"%s",err.localizedDescription.UTF8String?:"Apple 補幀失敗");
 [params release];[af release];[bf release];[df release];CGColorSpaceRelease(cs);
 if(!ok)clearSession();return ok;
 }
 return 0;
}}
