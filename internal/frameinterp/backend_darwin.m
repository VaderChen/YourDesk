#import <Foundation/Foundation.h>
#import <CoreML/CoreML.h>
#import <CoreVideo/CoreVideo.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <dispatch/dispatch.h>
int yd_rife_predict(const unsigned char*,const unsigned char*,int,int,int,int,unsigned char*,char*,int);
static MLModel *rife;
static NSURL *compiled;
static const int MW=512,MH=384;
static void msg(char *out,int n,NSString *s){snprintf(out,n,"%s",s.UTF8String ?: "RIFE error");}
int yd_rife_load(const char *path,char *error,int n){@autoreleasepool{
#if !defined(__arm64__)
 msg(error,n,@"RIFE 僅支援 Apple Silicon Mac");return 0;
#else
 if(@available(macOS 13.0,*)){
 NSError *err=nil;NSURL *url=[MLModel compileModelAtURL:[NSURL fileURLWithPath:@(path)] error:&err];if(!url){msg(error,n,err.localizedDescription);return 0;}
 MLModelConfiguration *config=[MLModelConfiguration new];config.computeUnits=MLComputeUnitsCPUAndGPU;
 rife=[[MLModel modelWithContentsOfURL:url configuration:config error:&err] retain];[config release];
 if(!rife){msg(error,n,err.localizedDescription);return 0;}
 compiled=[url retain];
 unsigned char *blank=calloc(512*384*4,1),*out=malloc(512*384*4);
 if(!blank||!out){free(blank);free(out);msg(error,n,@"RIFE 記憶體配置失敗");return 0;}
 int ok=1;for(int i=0;i<2 && ok;i++)ok=yd_rife_predict(blank,blank,512,384,512*4,512*4,out,error,n);
 free(blank);free(out);return ok;
 }
 msg(error,n,@"RIFE 需要 macOS 13 或更新版本");return 0;
#endif
}}
static float pixel(const unsigned char *p,int stride,int w,int h,float x,float y,int c){
 x=fmaxf(0,fminf(w-1,x));y=fmaxf(0,fminf(h-1,y));int ix=(int)x,iy=(int)y,jx=MIN(ix+1,w-1),jy=MIN(iy+1,h-1);float fx=x-ix,fy=y-iy;
 return (p[iy*stride+ix*4+c]*(1-fx)+p[iy*stride+jx*4+c]*fx)*(1-fy)+(p[jy*stride+ix*4+c]*(1-fx)+p[jy*stride+jx*4+c]*fx)*fy;
}
static CVPixelBufferRef input(const unsigned char *p,int stride,int w,int h,int rw,int rh){
 CVPixelBufferRef buf=NULL;if(CVPixelBufferCreate(NULL,MW,MH,kCVPixelFormatType_32BGRA,NULL,&buf)!=kCVReturnSuccess)return NULL;
 CVPixelBufferLockBaseAddress(buf,0);unsigned char *base=CVPixelBufferGetBaseAddress(buf);size_t row=CVPixelBufferGetBytesPerRow(buf);
 for(int y=0;y<MH;y++)for(int x=0;x<MW;x++){
 float sx=(MIN(x,rw-1)+0.5f)*w/rw-0.5f,sy=(MIN(y,rh-1)+0.5f)*h/rh-0.5f;unsigned char *q=base+y*row+x*4;
 q[0]=(unsigned char)pixel(p,stride,w,h,sx,sy,2);q[1]=(unsigned char)pixel(p,stride,w,h,sx,sy,1);q[2]=(unsigned char)pixel(p,stride,w,h,sx,sy,0);q[3]=255;
 }CVPixelBufferUnlockBaseAddress(buf,0);return buf;
}
static float flowSample(const float *p,int cs,int ys,int xs,float x,float y,int c){
 x=fmaxf(0,fminf(MW-1,x));y=fmaxf(0,fminf(MH-1,y));int ix=x,iy=y,jx=MIN(ix+1,MW-1),jy=MIN(iy+1,MH-1);float fx=x-ix,fy=y-iy;
 return (p[c*cs+iy*ys+ix*xs]*(1-fx)+p[c*cs+iy*ys+jx*xs]*fx)*(1-fy)+(p[c*cs+jy*ys+ix*xs]*(1-fx)+p[c*cs+jy*ys+jx*xs]*fx)*fy;
}
int yd_rife_predict(const unsigned char *a,const unsigned char *b,int w,int h,int as,int bs,unsigned char *out,char *error,int n){@autoreleasepool{
 if(!rife){msg(error,n,@"RIFE 尚未就緒");return 0;}
 float scale=fminf((float)MW/w,(float)MH/h);int rw=MAX(1,MIN(MW,(int)roundf(w*scale))),rh=MAX(1,MIN(MH,(int)roundf(h*scale)));
 CVPixelBufferRef ia=input(a,as,w,h,rw,rh),ib=input(b,bs,w,h,rw,rh);
 if(!ia||!ib){if(ia)CVPixelBufferRelease(ia);if(ib)CVPixelBufferRelease(ib);msg(error,n,@"RIFE 記憶體配置失敗");return 0;}
 NSError *err=nil;MLDictionaryFeatureProvider *provider=[[MLDictionaryFeatureProvider alloc]initWithDictionary:@{@"frame_a":[MLFeatureValue featureValueWithPixelBuffer:ia],@"frame_b":[MLFeatureValue featureValueWithPixelBuffer:ib]} error:&err];
 id<MLFeatureProvider> result=provider?[rife predictionFromFeatures:provider error:&err]:nil;
 MLMultiArray *flow=[result featureValueForName:@"flow_mask"].multiArrayValue;
 if(!flow||flow.dataType!=MLMultiArrayDataTypeFloat32||flow.shape.count!=4||flow.shape[1].intValue!=5||flow.shape[2].intValue!=MH||flow.shape[3].intValue!=MW){msg(error,n,err.localizedDescription ?: @"RIFE 輸出不相容");[provider release];CVPixelBufferRelease(ia);CVPixelBufferRelease(ib);return 0;}
 const float *p=(float*)flow.dataPointer;int cs=flow.strides[1].intValue,ys=flow.strides[2].intValue,xs=flow.strides[3].intValue;
 for(int c=0;c<5;c++)for(int y=0;y<MH;y++)for(int x=0;x<MW;x++)if(!isfinite(p[c*cs+y*ys+x*xs])){msg(error,n,@"RIFE 輸出數值無效");[provider release];CVPixelBufferRelease(ia);CVPixelBufferRelease(ib);return 0;}
 dispatch_apply((h+15)/16,dispatch_get_global_queue(QOS_CLASS_USER_INITIATED,0),^(size_t stripe){
 for(int y=(int)stripe*16;y<MIN(h,(int)(stripe+1)*16);y++)for(int x=0;x<w;x++){
 float fx=(x+0.5f)*rw/w-0.5f,fy=(y+0.5f)*rh/h-0.5f;
 float dx0=flowSample(p,cs,ys,xs,fx,fy,0)*w/rw,dy0=flowSample(p,cs,ys,xs,fx,fy,1)*h/rh,dx1=flowSample(p,cs,ys,xs,fx,fy,2)*w/rw,dy1=flowSample(p,cs,ys,xs,fx,fy,3)*h/rh,m=flowSample(p,cs,ys,xs,fx,fy,4);
 unsigned char *q=out+(y*w+x)*4;
 m=fmaxf(0,fminf(1,m));for(int c=0;c<3;c++)q[c]=(unsigned char)fmaxf(0,fminf(255,pixel(a,as,w,h,x+dx0,y+dy0,c)*m+pixel(b,bs,w,h,x+dx1,y+dy1,c)*(1-m)));q[3]=255;
 }
 });
 [provider release];CVPixelBufferRelease(ia);CVPixelBufferRelease(ib);return 1;
}}
