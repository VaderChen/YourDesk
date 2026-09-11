#import <Foundation/Foundation.h>
#import <CoreML/CoreML.h>
#import <CoreVideo/CoreVideo.h>
#include <stdio.h>
static struct { MLModel *model; NSURL *compiledURL; int width,height; } models[2];
static void message(char *out,int n,NSString *text){snprintf(out,n,"%s",text.UTF8String ?: "Core ML error");}
int yd_sr_load(int modelID,const char *path,char *error,int capacity){@autoreleasepool{
#if !defined(__arm64__)
 message(error,capacity,@"僅支援 Apple Silicon Mac");return 0;
#else
 if(@available(macOS 12.0,*)){
 if(modelID<0 || modelID>=2){message(error,capacity,@"未知的 Core ML 模型");return 0;}
 NSError *err=nil;
 NSURL *url=[MLModel compileModelAtURL:[NSURL fileURLWithPath:@(path)] error:&err];
 if(!url){message(error,capacity,err.localizedDescription);return 0;}
 MLModelConfiguration *config=[MLModelConfiguration new];config.computeUnits=MLComputeUnitsAll;
 MLModel *model=[[MLModel modelWithContentsOfURL:url configuration:config error:&err] retain];[config release];
 if(!model){message(error,capacity,err.localizedDescription);return 0;}
 MLImageConstraint *input=model.modelDescription.inputDescriptionsByName[@"input_image"].imageConstraint;
 MLImageConstraint *output=model.modelDescription.outputDescriptionsByName[@"output_image"].imageConstraint;
 if(!input || !output || input.pixelsWide<64 || input.pixelsHigh<64 || output.pixelsWide!=input.pixelsWide*2 || output.pixelsHigh!=input.pixelsHigh*2){[model release];model=nil;message(error,capacity,@"Core ML 模型尺寸不相容");return 0;}
 models[modelID].width=(int)input.pixelsWide;models[modelID].height=(int)input.pixelsHigh;models[modelID].compiledURL=[url retain];models[modelID].model=model;return 1;
 }
 message(error,capacity,@"需要 macOS 12 或更新版本");return 0;
#endif
}}
int yd_sr_predict(int modelID,const unsigned char *src,int width,int height,int stride,unsigned char *dst,char *error,int capacity){@autoreleasepool{
 if(modelID<0 || modelID>=2){message(error,capacity,@"未知的 Core ML 模型");return 0;}
 MLModel *model=models[modelID].model;int tileWidth=models[modelID].width,tileHeight=models[modelID].height;
 if(!model){message(error,capacity,@"Core ML 模型尚未就緒");return 0;}
 // 固定輸入模型採重疊分塊，捨棄邊緣，避免直接拼接卷積邊界。
 const int border=16,stepX=tileWidth-32,stepY=tileHeight-32;
 CFAbsoluteTime start=CFAbsoluteTimeGetCurrent();
 for(int top=0;top<height;top+=stepY)for(int left=0;left<width;left+=stepX){@autoreleasepool{
  if(CFAbsoluteTimeGetCurrent()-start>2.0){message(error,capacity,@"Core ML 處理逾時，回退 FSR 1");return 0;}
  CVPixelBufferRef input=NULL;
  if(CVPixelBufferCreate(kCFAllocatorDefault,tileWidth,tileHeight,kCVPixelFormatType_32BGRA,NULL,&input)!=kCVReturnSuccess){message(error,capacity,@"Core ML 記憶體配置失敗");return 0;}
  CVPixelBufferLockBaseAddress(input,0);unsigned char *base=CVPixelBufferGetBaseAddress(input);size_t pitch=CVPixelBufferGetBytesPerRow(input);
  for(int y=0;y<tileHeight;y++)for(int x=0;x<tileWidth;x++){
   int sy=MAX(0,MIN(height-1,top+y-border)),sx=MAX(0,MIN(width-1,left+x-border));
   const unsigned char *p=src+sy*stride+sx*4;unsigned char *q=base+y*pitch+x*4;q[0]=p[2];q[1]=p[1];q[2]=p[0];q[3]=255;
  }
  CVPixelBufferUnlockBaseAddress(input,0);NSError *err=nil;
  MLDictionaryFeatureProvider *provider=[[MLDictionaryFeatureProvider alloc] initWithDictionary:@{@"input_image":[MLFeatureValue featureValueWithPixelBuffer:input]} error:&err];
  id<MLFeatureProvider> prediction=provider?[model predictionFromFeatures:provider error:&err]:nil;
  CVPixelBufferRef output=[prediction featureValueForName:@"output_image"].imageBufferValue;
  if(!output || CVPixelBufferGetPixelFormatType(output)!=kCVPixelFormatType_32BGRA || CVPixelBufferGetWidth(output)!=tileWidth*2 || CVPixelBufferGetHeight(output)!=tileHeight*2){message(error,capacity,err.localizedDescription ?: @"Core ML 輸出格式不相容");[provider release];CVPixelBufferRelease(input);return 0;}
  CVPixelBufferLockBaseAddress(output,kCVPixelBufferLock_ReadOnly);const unsigned char *pixels=CVPixelBufferGetBaseAddress(output);size_t row=CVPixelBufferGetBytesPerRow(output);
  for(int y=0;y<MIN(stepY,height-top)*2;y++)for(int x=0;x<MIN(stepX,width-left)*2;x++){
   const unsigned char *p=pixels+(y+border*2)*row+(x+border*2)*4;unsigned char *q=dst+((top*2+y)*width*2+left*2+x)*4;q[0]=p[2];q[1]=p[1];q[2]=p[0];q[3]=255;
  }
  CVPixelBufferUnlockBaseAddress(output,kCVPixelBufferLock_ReadOnly);[provider release];CVPixelBufferRelease(input);
 }}return 1;
}}
