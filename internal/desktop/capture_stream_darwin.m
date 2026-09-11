#import <Foundation/Foundation.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreVideo/CoreVideo.h>

unsigned int yd_capture_display_id(int index) {
 uint32_t count=0; if(CGGetActiveDisplayList(0,NULL,&count)!=kCGErrorSuccess||index<0||index>=count)return 0;
 CGDirectDisplayID *ids=calloc(count,sizeof(*ids));CGGetActiveDisplayList(count,ids,&count);
 unsigned int result=index<count?ids[index]:0;free(ids);return result;
}

@interface YDLiveCapture : NSObject <SCStreamOutput,SCStreamDelegate> {
@public
 NSCondition *condition;
 SCStream *stream;
 dispatch_queue_t queue;
 CVPixelBufferRef latest;
 uint64_t revision,consumed;
 BOOL ready,closed;
 NSString *failure;
}
-(void)begin:(CGDirectDisplayID)display width:(int)w height:(int)h;
-(void)shutdown;
@end

@implementation YDLiveCapture
-(instancetype)init {if((self=[super init])){condition=[NSCondition new];queue=dispatch_queue_create("YourDesk.capture",DISPATCH_QUEUE_SERIAL);}return self;}
-(void)begin:(CGDirectDisplayID)display width:(int)w height:(int)h {
 [SCShareableContent getShareableContentExcludingDesktopWindows:NO onScreenWindowsOnly:YES completionHandler:^(SCShareableContent *content,NSError *err){
  @autoreleasepool {
   [condition lock];if(closed){[condition unlock];return;}
   SCDisplay *target=nil;for(SCDisplay *item in content.displays){if(item.displayID==display){target=item;break;}}
   if(err||!target){failure=[(err.localizedDescription?:@"找不到擷取螢幕") copy];ready=YES;[condition broadcast];[condition unlock];return;}
   SCContentFilter *filter=[[SCContentFilter alloc]initWithDisplay:target excludingWindows:@[]];
   SCStreamConfiguration *config=[SCStreamConfiguration new];config.width=w;config.height=h;
   config.pixelFormat=kCVPixelFormatType_32BGRA;config.showsCursor=NO;config.capturesAudio=NO;
   config.minimumFrameInterval=CMTimeMake(1,60);config.queueDepth=3;config.colorSpaceName=kCGColorSpaceSRGB;
   stream=[[SCStream alloc]initWithFilter:filter configuration:config delegate:self];
   [filter release];[config release];
   NSError *outputError=nil;
   if(![stream addStreamOutput:self type:SCStreamOutputTypeScreen sampleHandlerQueue:queue error:&outputError]){
    failure=[outputError.localizedDescription copy];ready=YES;[condition broadcast];[condition unlock];return;
   }
   [stream startCaptureWithCompletionHandler:^(NSError *startError){
    [condition lock];if(startError){[failure release];failure=[startError.localizedDescription copy];}
    ready=YES;BOOL stop=closed;[condition broadcast];[condition unlock];
    if(stop)[stream stopCaptureWithCompletionHandler:nil];
   }];
   [condition unlock];
  }
 }];
}
-(void)stream:(SCStream *)sender didOutputSampleBuffer:(CMSampleBufferRef)sample ofType:(SCStreamOutputType)type {
 if(type!=SCStreamOutputTypeScreen||!CMSampleBufferIsValid(sample))return;
 CFArrayRef attachments=CMSampleBufferGetSampleAttachmentsArray(sample,NO);
 if(!attachments||!CFArrayGetCount(attachments))return;
 NSDictionary *info=(NSDictionary *)CFArrayGetValueAtIndex(attachments,0);
 NSNumber *status=info[SCStreamFrameInfoStatus];
 [condition lock];
 if(!closed && status && status.integerValue==SCFrameStatusComplete){
  CVPixelBufferRef buffer=CMSampleBufferGetImageBuffer(sample);
  if(buffer){CVPixelBufferRetain(buffer);if(latest)CVPixelBufferRelease(latest);latest=buffer;revision++;[condition broadcast];}
 } else if(status && (status.integerValue==SCFrameStatusBlank || status.integerValue==SCFrameStatusSuspended)) {
  if(latest)CVPixelBufferRelease(latest);latest=NULL;
 }
 [condition unlock];
}
-(void)stream:(SCStream *)sender didStopWithError:(NSError *)err {
 [condition lock];[failure release];failure=[err.localizedDescription copy];[condition broadcast];[condition unlock];
}
-(void)shutdown {
 [condition lock];closed=YES;if(latest)CVPixelBufferRelease(latest);latest=NULL;[condition broadcast];[condition unlock];
 [stream removeStreamOutput:self type:SCStreamOutputTypeScreen error:nil];
 [stream stopCaptureWithCompletionHandler:nil];
}
-(void)dealloc {if(latest)CVPixelBufferRelease(latest);[stream release];[failure release];[condition release];dispatch_release(queue);[super dealloc];}
@end

void *yd_capture_open(int display,int w,int h,char *error,int length){@autoreleasepool{
 if(@available(macOS 13.0,*)){
  YDLiveCapture *capture=[YDLiveCapture new];[capture begin:yd_capture_display_id(display) width:w height:h];
  [capture->condition lock];NSDate *until=[NSDate dateWithTimeIntervalSinceNow:5];
  while(!capture->ready && [capture->condition waitUntilDate:until]){}
  BOOL ok=capture->ready&&!capture->failure;
  if(!ok)snprintf(error,length,"%s",capture->failure.UTF8String?:"啟動擷取逾時");
  [capture->condition unlock];if(!ok){[capture shutdown];[capture release];return NULL;}return capture;
 }
 snprintf(error,length,"ScreenCaptureKit 需要 macOS 13 以上");return NULL;
}}
int yd_capture_read(void *handle,unsigned char *out,int w,int h,char *error,int length){@autoreleasepool{
 YDLiveCapture *capture=handle;[capture->condition lock];NSDate *until=[NSDate dateWithTimeIntervalSinceNow:0.1];
 while(!capture->closed&&!capture->failure&&(!capture->latest||capture->revision==capture->consumed)){
  if(![capture->condition waitUntilDate:until])break;
 }
 if(capture->failure||capture->closed){snprintf(error,length,"%s",capture->failure.UTF8String?:"擷取已結束");[capture->condition unlock];return -1;}
 CVPixelBufferRef buffer=capture->latest;
 if(!buffer||capture->revision==capture->consumed){[capture->condition unlock];return 0;}
 CVPixelBufferRetain(buffer);capture->consumed=capture->revision;[capture->condition unlock];
 if(CVPixelBufferGetWidth(buffer)!=w||CVPixelBufferGetHeight(buffer)!=h){CVPixelBufferRelease(buffer);snprintf(error,length,"擷取尺寸已改變");return -1;}
 CVReturn locked=CVPixelBufferLockBaseAddress(buffer,kCVPixelBufferLock_ReadOnly);
 if(locked!=kCVReturnSuccess){CVPixelBufferRelease(buffer);snprintf(error,length,"無法讀取擷取影像");return -1;}
 const unsigned char *base=CVPixelBufferGetBaseAddress(buffer);size_t stride=CVPixelBufferGetBytesPerRow(buffer);
 for(int y=0;y<h;y++){const unsigned char *src=base+y*stride;unsigned char *dst=out+(size_t)y*w*4;for(int x=0;x<w;x++){dst[x*4]=src[x*4+2];dst[x*4+1]=src[x*4+1];dst[x*4+2]=src[x*4];dst[x*4+3]=255;}}
 CVPixelBufferUnlockBaseAddress(buffer,kCVPixelBufferLock_ReadOnly);CVPixelBufferRelease(buffer);return 1;
}}
void yd_capture_close(void *handle){@autoreleasepool{YDLiveCapture *capture=handle;[capture shutdown];[capture release];}}
