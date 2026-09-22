//go:build windows && cgo
// 各 MinGW 工具鏈的預設 WINVER 不同；產品最低需求為 Windows 10。
#ifndef WINVER
#define WINVER 0x0A00
#endif
#ifndef _WIN32_WINNT
#define _WIN32_WINNT WINVER
#endif
#include "native.h"
#include <windows.h>
#include <mmdeviceapi.h>
#include <audioclient.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mftransform.h>
#include <mferror.h>
#include <wrl/client.h>
#include <algorithm>
#include <cstdio>
#include <cstring>
#include <memory>
using Microsoft::WRL::ComPtr;
#define CHECK(x) do{HRESULT e_=(x);if(FAILED(e_))return e_;}while(0)
struct Audio {
 int mode,codec;bool com=false,mf=false,hw=false,async=false,started=false;unsigned needs=0,outputs=0;LONGLONG timestamp=0;
 DWORD inID=0,outID=0;UINT32 capacity=0;
 ComPtr<IAudioClient> client;ComPtr<IAudioCaptureClient> capture;ComPtr<IAudioRenderClient> render;
 ComPtr<IMFTransform> transform;ComPtr<IMFActivate> activation;ComPtr<IMFMediaEventGenerator> events;
 ~Audio(){if(client)client->Stop();capture.Reset();render.Reset();client.Reset();reset();if(mf)MFShutdown();if(com)CoUninitialize();}
 void reset(){if(transform){transform->ProcessMessage(MFT_MESSAGE_COMMAND_FLUSH,0);transform->ProcessMessage(MFT_MESSAGE_NOTIFY_END_STREAMING,0);}events.Reset();transform.Reset();if(activation)activation->ShutdownObject();activation.Reset();needs=outputs=0;async=false;}
 HRESULT init(){CHECK(CoInitializeEx(nullptr,COINIT_MULTITHREADED));com=true;CHECK(MFStartup(MF_VERSION));mf=true;return S_OK;}
 HRESULT device(){
  ComPtr<IMMDeviceEnumerator> list;ComPtr<IMMDevice> endpoint;
  CHECK(CoCreateInstance(__uuidof(MMDeviceEnumerator),nullptr,CLSCTX_ALL,IID_PPV_ARGS(&list)));
  CHECK(list->GetDefaultAudioEndpoint(eRender,eConsole,&endpoint));CHECK(endpoint->Activate(__uuidof(IAudioClient),CLSCTX_ALL,nullptr,reinterpret_cast<void**>(client.GetAddressOf())));
  WAVEFORMATEX f{};f.wFormatTag=WAVE_FORMAT_PCM;f.nChannels=2;f.nSamplesPerSec=48000;f.nAvgBytesPerSec=192000;f.nBlockAlign=4;f.wBitsPerSample=16;
  DWORD flags=AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM|AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY;
  if(mode==3)flags|=AUDCLNT_STREAMFLAGS_LOOPBACK;
  CHECK(client->Initialize(AUDCLNT_SHAREMODE_SHARED,flags,2000000,0,&f,nullptr));CHECK(client->GetBufferSize(&capacity));
  if(mode==3){CHECK(client->GetService(IID_PPV_ARGS(&capture)));CHECK(client->Start());started=true;}
  else CHECK(client->GetService(IID_PPV_ARGS(&render)));
  return S_OK;
 }
 HRESULT type(bool aac,int bitrate,ComPtr<IMFMediaType>&t){
  CHECK(MFCreateMediaType(&t));CHECK(t->SetGUID(MF_MT_MAJOR_TYPE,MFMediaType_Audio));CHECK(t->SetGUID(MF_MT_SUBTYPE,aac?MFAudioFormat_AAC:MFAudioFormat_PCM));
  CHECK(t->SetUINT32(MF_MT_AUDIO_NUM_CHANNELS,2));CHECK(t->SetUINT32(MF_MT_AUDIO_SAMPLES_PER_SECOND,48000));CHECK(t->SetUINT32(MF_MT_AUDIO_BITS_PER_SAMPLE,16));
  CHECK(t->SetUINT32(MF_MT_AUDIO_AVG_BYTES_PER_SECOND,aac?bitrate/8:192000));CHECK(t->SetUINT32(MF_MT_AUDIO_BLOCK_ALIGNMENT,aac?1:4));
  if(aac){CHECK(t->SetUINT32(MF_MT_AAC_PAYLOAD_TYPE,0));CHECK(t->SetUINT32(MF_MT_AAC_AUDIO_PROFILE_LEVEL_INDICATION,0x29));if(mode==2){const BYTE config[]={0,0,0x29,0,0,0,0,0,0,0,0,0,0x11,0x90};CHECK(t->SetBlob(MF_MT_USER_DATA,config,sizeof(config)));}}
  return S_OK;
 }
 HRESULT attach(IMFActivate *candidate,int bitrate){
  reset();activation=candidate;CHECK(candidate->ActivateObject(IID_PPV_ARGS(&transform)));
  ComPtr<IMFAttributes> attrs;if(SUCCEEDED(transform->GetAttributes(&attrs))){UINT32 flag=0;attrs->GetUINT32(MF_TRANSFORM_ASYNC,&flag);async=flag!=0;if(async){CHECK(attrs->SetUINT32(MF_TRANSFORM_ASYNC_UNLOCK,TRUE));CHECK(transform.As(&events));}attrs->SetUINT32(MF_LOW_LATENCY,TRUE);}
  DWORD ins=0,outs=0;CHECK(transform->GetStreamCount(&ins,&outs));if(ins!=1||outs!=1)return E_NOTIMPL;
  HRESULT ids=transform->GetStreamIDs(1,&inID,1,&outID);if(ids==E_NOTIMPL)inID=outID=0;else CHECK(ids);
  ComPtr<IMFMediaType> pcm,aac;CHECK(type(false,bitrate,pcm));CHECK(type(true,bitrate,aac));
  if(mode==1){CHECK(transform->SetOutputType(outID,aac.Get(),0));CHECK(transform->SetInputType(inID,pcm.Get(),0));}
  else {CHECK(transform->SetInputType(inID,aac.Get(),0));CHECK(transform->SetOutputType(outID,pcm.Get(),0));}
  CHECK(transform->ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING,0));return transform->ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM,0);
 }
 HRESULT codecOpen(int bitrate,int preference){
  MFT_REGISTER_TYPE_INFO in={MFMediaType_Audio,mode==1?MFAudioFormat_PCM:MFAudioFormat_AAC},out={MFMediaType_Audio,mode==1?MFAudioFormat_AAC:MFAudioFormat_PCM};
  HRESULT last=E_NOINTERFACE;
  for(int pass=0;pass<2;pass++){
   bool hardware=pass==0;if((preference==0&&hardware)||(preference==2&&!hardware))continue;
   IMFActivate **items=nullptr;UINT32 count=0;
   UINT32 flags=MFT_ENUM_FLAG_SORTANDFILTER|(hardware?MFT_ENUM_FLAG_HARDWARE:(MFT_ENUM_FLAG_SYNCMFT|MFT_ENUM_FLAG_ASYNCMFT|MFT_ENUM_FLAG_LOCALMFT));
   last=MFTEnumEx(mode==1?MFT_CATEGORY_AUDIO_ENCODER:MFT_CATEGORY_AUDIO_DECODER,flags,&in,&out,&items,&count);
   if(FAILED(last))continue;
   bool found=false;for(UINT32 i=0;i<count;i++){if(!found){last=attach(items[i],bitrate);if(SUCCEEDED(last)){found=true;hw=hardware;}}items[i]->Release();}CoTaskMemFree(items);
   if(found)return S_OK;
  }
  reset();return FAILED(last)?last:E_NOINTERFACE;
 }
 HRESULT pump(){
  if(!async)return S_OK;
  for(int i=0;i<64;i++){ComPtr<IMFMediaEvent> event;HRESULT e=events->GetEvent(MF_EVENT_FLAG_NO_WAIT,&event);if(e==MF_E_NO_EVENTS_AVAILABLE)return S_OK;CHECK(e);HRESULT status;CHECK(event->GetStatus(&status));CHECK(status);MediaEventType t;CHECK(event->GetType(&t));if(t==METransformNeedInput)needs++;if(t==METransformHaveOutput)outputs++;if(t==MEError)return E_FAIL;}return S_OK;
 }
 HRESULT process(const void *input,int length,void *output,int size,int &written){
  written=0;
  if(mode==3){
   // 每次只取有限包數；擷取環形緩衝由 WASAPI 管理，回呼不碰網路。
   for(int i=0;i<16;i++){UINT32 available=0;CHECK(capture->GetNextPacketSize(&available));if(!available)break;
    BYTE *data=nullptr;UINT32 frames=0;DWORD flags=0;CHECK(capture->GetBuffer(&data,&frames,&flags,nullptr,nullptr));
    if(frames*4>(UINT32)size){capture->ReleaseBuffer(frames);continue;}
    if(written+(int)frames*4>size){capture->ReleaseBuffer(0);break;}
    if(flags&AUDCLNT_BUFFERFLAGS_SILENT)memset((char*)output+written,0,frames*4);else memcpy((char*)output+written,data,frames*4);
    written+=frames*4;CHECK(capture->ReleaseBuffer(frames));
   }return S_OK;
  }
  if(mode==4){
   if(length<=0||length%4)return E_INVALIDARG;
   UINT32 queued=0;CHECK(client->GetCurrentPadding(&queued));UINT32 frames=length/4;if(frames>capacity-queued||queued>8192)return S_OK;
   BYTE *data=nullptr;CHECK(render->GetBuffer(frames,&data));memcpy(data,input,length);CHECK(render->ReleaseBuffer(frames,0));if(!started){CHECK(client->Start());started=true;}return S_OK;
  }
  if(codec==2){if(length>size)return E_INVALIDARG;memcpy(output,input,length);written=length;return S_OK;}
  ULONGLONG deadline=GetTickCount64()+100;
  while(async&&!needs){CHECK(pump());if(needs)break;if(GetTickCount64()>=deadline)return HRESULT_FROM_WIN32(WAIT_TIMEOUT);Sleep(1);}
  ComPtr<IMFSample> sample;ComPtr<IMFMediaBuffer> buffer;CHECK(MFCreateSample(&sample));CHECK(MFCreateMemoryBuffer(length,&buffer));BYTE *data=nullptr;CHECK(buffer->Lock(&data,nullptr,nullptr));memcpy(data,input,length);buffer->Unlock();CHECK(buffer->SetCurrentLength(length));CHECK(sample->AddBuffer(buffer.Get()));
  CHECK(sample->SetSampleTime(timestamp));CHECK(sample->SetSampleDuration(1024LL*10000000/48000));timestamp+=1024LL*10000000/48000;
  CHECK(transform->ProcessInput(inID,sample.Get(),0));if(async)needs--;
  CHECK(pump());if(async&&!outputs)return S_OK;
  for(int attempt=0;attempt<2;attempt++){
   MFT_OUTPUT_STREAM_INFO info{};CHECK(transform->GetOutputStreamInfo(outID,&info));if(info.cbSize>65536)return E_OUTOFMEMORY;
   MFT_OUTPUT_DATA_BUFFER result{};result.dwStreamID=outID;ComPtr<IMFSample> supplied;
   if(!(info.dwFlags&MFT_OUTPUT_STREAM_PROVIDES_SAMPLES)){CHECK(MFCreateSample(&supplied));ComPtr<IMFMediaBuffer> b;CHECK(MFCreateAlignedMemoryBuffer(std::max<DWORD>(info.cbSize,8192),info.cbAlignment,&b));CHECK(supplied->AddBuffer(b.Get()));result.pSample=supplied.Get();}
   DWORD status=0;HRESULT e=transform->ProcessOutput(0,1,&result,&status);if(result.pEvents)result.pEvents->Release();if(async&&outputs)outputs--;
   ComPtr<IMFSample> received;if(result.pSample==supplied.Get())received=supplied;else received.Attach(result.pSample);
   if(e==MF_E_TRANSFORM_NEED_MORE_INPUT)return S_OK;
   if(e==MF_E_TRANSFORM_STREAM_CHANGE&&mode==2){ComPtr<IMFMediaType> pcm;CHECK(type(false,128000,pcm));CHECK(transform->SetOutputType(outID,pcm.Get(),0));continue;}
   CHECK(e);if(!received)return E_FAIL;ComPtr<IMFMediaBuffer> b;CHECK(received->ConvertToContiguousBuffer(&b));DWORD bytes=0;CHECK(b->Lock(&data,nullptr,&bytes));if(bytes>(DWORD)size){b->Unlock();return E_OUTOFMEMORY;}memcpy(output,data,bytes);written=bytes;b->Unlock();return S_OK;
  }return E_FAIL;
 }
};
extern "C" void *yd_audio_open(int mode,int codec,int bitrate,int preference,int *hardware,char *error,int length)try{
 std::unique_ptr<Audio> d(new Audio);d->mode=mode;d->codec=codec;HRESULT e=d->init();if(SUCCEEDED(e))e=mode>=3?d->device():codec==1?d->codecOpen(bitrate,preference):preference==2?E_NOINTERFACE:S_OK;
 if(FAILED(e)){snprintf(error,length,"Windows audio (0x%08lX)",(unsigned long)e);return nullptr;}*hardware=d->hw;return d.release();
}catch(...){snprintf(error,length,"Windows audio allocation failed");return nullptr;}
extern "C" int yd_audio_process(void *handle,const void *input,int length,void *output,int capacity)try{int written=0;HRESULT e=((Audio*)handle)->process(input,length,output,capacity,written);return FAILED(e)?(int)e:written;}catch(...){return (int)E_OUTOFMEMORY;}
extern "C" void yd_audio_close(void *handle){delete (Audio*)handle;}
