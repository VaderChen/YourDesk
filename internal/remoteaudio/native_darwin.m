#import <Foundation/Foundation.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <AudioToolbox/AudioToolbox.h>
#import <CoreMedia/CoreMedia.h>
#include <stdatomic.h>
#include "native.h"

static AudioStreamBasicDescription pcmFormat(void) {
 AudioStreamBasicDescription f={0};f.mSampleRate=48000;f.mFormatID=kAudioFormatLinearPCM;
 f.mFormatFlags=kAudioFormatFlagIsSignedInteger|kAudioFormatFlagIsPacked;f.mBytesPerPacket=4;f.mFramesPerPacket=1;f.mBytesPerFrame=4;f.mChannelsPerFrame=2;f.mBitsPerChannel=16;return f;
}
static AudioStreamBasicDescription aacFormat(void) {
 AudioStreamBasicDescription f={0};f.mSampleRate=48000;f.mFormatID=kAudioFormatMPEG4AAC;f.mChannelsPerFrame=2;f.mFramesPerPacket=1024;return f;
}
@interface YDAudioCapture : NSObject<SCStreamOutput,SCStreamDelegate> {
@public NSCondition *lock; SCStream *stream; dispatch_queue_t queue; NSMutableData *pending; BOOL ready,closed; NSString *failure;
}
-(void)begin;
-(void)shutdown;
@end
@implementation YDAudioCapture
-(instancetype)init {if((self=[super init])){lock=[NSCondition new];pending=[NSMutableData new];queue=dispatch_queue_create("YourDesk.audio.capture",DISPATCH_QUEUE_SERIAL);}return self;}
-(void)begin {
 [SCShareableContent getShareableContentExcludingDesktopWindows:YES onScreenWindowsOnly:YES completionHandler:^(SCShareableContent *content,NSError *error){@autoreleasepool{
  [lock lock];if(closed){[lock unlock];return;}
  if(error||!content.displays.count){failure=[(error.localizedDescription?:@"找不到系統聲音來源") copy];ready=YES;[lock broadcast];[lock unlock];return;}
  SCContentFilter *filter=[[SCContentFilter alloc]initWithDisplay:content.displays[0] excludingWindows:@[]];
  SCStreamConfiguration *config=[SCStreamConfiguration new];config.width=2;config.height=2;config.minimumFrameInterval=CMTimeMake(1,1);
  config.capturesAudio=YES;config.excludesCurrentProcessAudio=YES;config.sampleRate=48000;config.channelCount=2;
  stream=[[SCStream alloc]initWithFilter:filter configuration:config delegate:self];[filter release];[config release];
  NSError *e=nil;
  if(![stream addStreamOutput:self type:SCStreamOutputTypeAudio sampleHandlerQueue:queue error:&e]){failure=[e.localizedDescription copy];ready=YES;[lock broadcast];[lock unlock];return;}
  [stream startCaptureWithCompletionHandler:^(NSError *e){[lock lock];if(e){[failure release];failure=[e.localizedDescription copy];}ready=YES;BOOL stop=closed;[lock broadcast];[lock unlock];if(stop)[stream stopCaptureWithCompletionHandler:nil];}];
  [lock unlock];
 }}];
}
-(void)stream:(SCStream*)s didOutputSampleBuffer:(CMSampleBufferRef)sample ofType:(SCStreamOutputType)type {
 if(type!=SCStreamOutputTypeAudio||!CMSampleBufferIsValid(sample))return;
 const AudioStreamBasicDescription *f=CMAudioFormatDescriptionGetStreamBasicDescription(CMSampleBufferGetFormatDescription(sample));
 if(!f||f->mSampleRate!=48000||f->mChannelsPerFrame!=2||f->mFormatID!=kAudioFormatLinearPCM)return;
 size_t size=0;CMBlockBufferRef block=NULL;
 CMSampleBufferGetAudioBufferListWithRetainedBlockBuffer(sample,&size,NULL,0,NULL,NULL,0,NULL);
 AudioBufferList *list=malloc(size);if(!list)return;
 if(CMSampleBufferGetAudioBufferListWithRetainedBlockBuffer(sample,NULL,list,size,NULL,NULL,kCMSampleBufferFlag_AudioBufferList_Assure16ByteAlignment,&block)!=noErr){free(list);return;}
 size_t frames=CMSampleBufferGetNumSamples(sample);
 if(frames>48000){if(block)CFRelease(block);free(list);return;}
 BOOL floating=(f->mFormatFlags&kAudioFormatFlagIsFloat)!=0, planar=(f->mFormatFlags&kAudioFormatFlagIsNonInterleaved)!=0;
 if((floating&&f->mBitsPerChannel!=32)||(!floating&&f->mBitsPerChannel!=16)||list->mNumberBuffers<(planar?2:1)){if(block)CFRelease(block);free(list);return;}
 NSMutableData *pcm=[NSMutableData dataWithLength:frames*4];int16_t *out=pcm.mutableBytes;
 for(size_t i=0;i<frames;i++)for(int c=0;c<2;c++){
  AudioBuffer b=list->mBuffers[planar?c:0];size_t index=planar?i:i*2+c;
  if((index+1)*(floating?4:2)>b.mDataByteSize||!b.mData){out[i*2+c]=0;continue;}
  if(floating){float v=((float*)b.mData)[index];out[i*2+c]=(int16_t)(fmaxf(-1,fminf(1,v))*32767);}else out[i*2+c]=((int16_t*)b.mData)[index];
 }
 if(block)CFRelease(block);free(list);
 [lock lock];if(!closed){[pending appendData:pcm];if(pending.length>49152)[pending replaceBytesInRange:NSMakeRange(0,pending.length-49152) withBytes:NULL length:0];}[lock unlock];
}
-(void)stream:(SCStream*)s didStopWithError:(NSError*)e {[lock lock];[failure release];failure=[e.localizedDescription copy];[lock unlock];}
-(void)shutdown {[lock lock];closed=YES;[pending setLength:0];[lock unlock];[stream removeStreamOutput:self type:SCStreamOutputTypeAudio error:nil];[stream stopCaptureWithCompletionHandler:nil];}
-(void)dealloc {[stream release];[lock release];[pending release];[failure release];dispatch_release(queue);[super dealloc];}
@end

typedef struct {
 int mode,codec;AudioConverterRef converter;YDAudioCapture *capture;
 const void *input;UInt32 inputLength;BOOL consumed;AudioStreamPacketDescription packet;
 AudioQueueRef playback;AudioQueueBufferRef buffers[8];atomic_bool busy[8];BOOL started;
} YDAudio;
static OSStatus feed(AudioConverterRef converter,UInt32 *count,AudioBufferList *data,AudioStreamPacketDescription **description,void *context){
 YDAudio *d=context;
 if(d->consumed||!d->inputLength){*count=0;return -2222;}
 data->mNumberBuffers=1;data->mBuffers[0].mNumberChannels=2;data->mBuffers[0].mData=(void*)d->input;
 if(d->mode==1){*count=MIN(*count,d->inputLength/4);data->mBuffers[0].mDataByteSize=*count*4;d->input=(const char*)d->input+*count*4;d->inputLength-=*count*4;}
 else {*count=1;data->mBuffers[0].mDataByteSize=d->inputLength;d->packet=(AudioStreamPacketDescription){0,1024,d->inputLength};if(description)*description=&d->packet;d->consumed=YES;}
 return noErr;
}
static void played(void *context,AudioQueueRef q,AudioQueueBufferRef b){YDAudio *d=context;for(int i=0;i<8;i++)if(d->buffers[i]==b){atomic_store(&d->busy[i],false);break;}}
static OSStatus createConverter(YDAudio *d,int preference,int *hardware){
 AudioStreamBasicDescription pcm=pcmFormat(),aac=aacFormat();
 AudioStreamBasicDescription *in=d->mode==1?&pcm:&aac,*out=d->mode==1?&aac:&pcm;
 AudioFormatPropertyID property=d->mode==1?kAudioFormatProperty_Encoders:kAudioFormatProperty_Decoders;
 UInt32 format=kAudioFormatMPEG4AAC,size=0;OSStatus err=AudioFormatGetPropertyInfo(property,sizeof(format),&format,&size);if(err)return err;
 AudioClassDescription *classes=malloc(size);if(!classes)return -1;
 err=AudioFormatGetProperty(property,sizeof(format),&format,&size,classes);if(err){free(classes);return err;}
 err=-1;
 for(int pass=0;pass<2&&!d->converter;pass++){
  BOOL hw=pass==0;if((preference==0&&hw)||(preference==2&&!hw))continue;
  for(int i=0;i<size/sizeof(*classes);i++){
   BOOL candidate=classes[i].mManufacturer=='aphw' /* Apple 硬體製造商碼；macOS SDK 未公開常數，須實際列舉與驗證 */;
   if(candidate!=hw)continue;
   err=AudioConverterNewSpecific(in,out,1,&classes[i],&d->converter);
   if(!err&&d->converter){*hardware=hw;break;}
  }
 }
 free(classes);return d->converter?noErr:err;
}
void *yd_audio_open(int mode,int codec,int bitrate,int preference,int *hardware,char *error,int length){@autoreleasepool{
 YDAudio *d=calloc(1,sizeof(*d));if(!d)return NULL;d->mode=mode;d->codec=codec;*hardware=0;OSStatus err=0;
 if(mode==3){
  if(@available(macOS 13.0,*)){
   d->capture=[YDAudioCapture new];[d->capture begin];[d->capture->lock lock];NSDate *until=[NSDate dateWithTimeIntervalSinceNow:4];
   while(!d->capture->ready&&[d->capture->lock waitUntilDate:until]){}
   BOOL ok=d->capture->ready&&!d->capture->failure;
   if(!ok)snprintf(error,length,"%s",d->capture->failure.UTF8String?:"啟動系統聲音擷取逾時");
   [d->capture->lock unlock];if(!ok){yd_audio_close(d);return NULL;}
  }else{snprintf(error,length,"系統聲音需要 macOS 13 以上");yd_audio_close(d);return NULL;}
 }else if(mode==4){
  AudioStreamBasicDescription f=pcmFormat();err=AudioQueueNewOutput(&f,played,d,NULL,NULL,0,&d->playback);
  for(int i=0;i<8&&!err;i++)err=AudioQueueAllocateBuffer(d->playback,8192,&d->buffers[i]);
 }else if(codec==1){
  err=createConverter(d,preference,hardware);
  if(!err&&mode==1){UInt32 rate=bitrate;err=AudioConverterSetProperty(d->converter,kAudioConverterEncodeBitRate,sizeof(rate),&rate);}
 }else if(preference==2){err=-1;}
 if(err){snprintf(error,length,"AudioToolbox (%d)",(int)err);yd_audio_close(d);return NULL;}
 return d;
}}
int yd_audio_process(void *handle,const void *input,int length,void *output,int capacity){@autoreleasepool{
 YDAudio *d=handle;if(!d)return -1;
 if(d->mode==3){
  [d->capture->lock lock];if(d->capture->failure||d->capture->closed){[d->capture->lock unlock];return -1;}
  int n=MIN(capacity,d->capture->pending.length);n-=n%4;
  if(n){memcpy(output,d->capture->pending.bytes,n);[d->capture->pending replaceBytesInRange:NSMakeRange(0,n) withBytes:NULL length:0];}
  [d->capture->lock unlock];return n;
 }
 if(d->mode==4){
  if(length<=0||length>8192||length%4)return -1;
  int slot=-1;for(int i=0;i<8;i++){bool expected=false;if(atomic_compare_exchange_strong(&d->busy[i],&expected,true)){slot=i;break;}}
  if(slot<0)return 0; // 裝置壅塞時捨棄，絕不累積數秒的延遲。
  AudioQueueBufferRef b=d->buffers[slot];memcpy(b->mAudioData,input,length);b->mAudioDataByteSize=length;
  OSStatus e=AudioQueueEnqueueBuffer(d->playback,b,0,NULL);if(e){atomic_store(&d->busy[slot],false);return e;}
  if(!d->started){e=AudioQueueStart(d->playback,NULL);if(e)return e;d->started=YES;}return 0;
 }
 if(d->codec==2){if(length>capacity)return -1;memcpy(output,input,length);return length;}
 d->input=input;d->inputLength=length;d->consumed=NO;
 AudioBufferList list={0};list.mNumberBuffers=1;list.mBuffers[0]=(AudioBuffer){2,(UInt32)capacity,output};
 UInt32 packets=d->mode==1?1:capacity/4;AudioStreamPacketDescription description;
 OSStatus err=AudioConverterFillComplexBuffer(d->converter,feed,d,&packets,&list,d->mode==1?&description:NULL);
 d->input=NULL;d->inputLength=0;
 if(err&&err!=-2222)return err;
 return packets?list.mBuffers[0].mDataByteSize:0;
}}
void yd_audio_close(void *handle){@autoreleasepool{
 YDAudio *d=handle;if(!d)return;
 if(d->capture){[d->capture shutdown];[d->capture release];}
 if(d->converter)AudioConverterDispose(d->converter);
 if(d->playback){AudioQueueStop(d->playback,true);AudioQueueDispose(d->playback,true);}
 free(d);
}}
