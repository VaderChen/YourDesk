//go:build windows && cgo
// Media Foundation 與 D3D11 共用後端；由固定 OS 執行緒建立、使用與釋放。
#include "native.h"
#include <windows.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mftransform.h>
#include <mferror.h>
#include <codecapi.h>
#include <strmif.h>
#include <cstdint>
#include <d3d11.h>
#include <d3d12.h>
#include <d3d10.h>
#include <dxgi1_2.h>
#include <wrl/client.h>
#include <vector>
#include <string>
#include <algorithm>
#include <new>
#include <memory>
#include <cstring>
#include <cstdio>
using Microsoft::WRL::ComPtr;
static thread_local char ydFailure[1024]{};
static HRESULT traceFailure(HRESULT hr,const char *step){
 if(!ydFailure[0])snprintf(ydFailure,sizeof(ydFailure),"%s (0x%08lX)",step,(unsigned long)hr);
 return hr;
}
#define CHECK(x) do{HRESULT h_=(x);if(FAILED(h_))return traceFailure(h_,#x);}while(0)
extern "C" const char *yd_media_error(){return ydFailure;}
static const size_t maxFrame=32u*1024u*1024u;
static bool validSize(int w,int h){return w>0&&h>0&&w<=8192&&h<=8192&&(uint64_t)w*h<=maxFrame;}
static HRESULT setCodec(IMFTransform *transform,const GUID &key,ULONG value,bool boolean=false){
 ComPtr<ICodecAPI> api;CHECK(transform->QueryInterface(IID_ICodecAPI,reinterpret_cast<void **>(api.GetAddressOf())));
 VARIANT v;VariantInit(&v);v.vt=boolean?VT_BOOL:VT_UI4;if(boolean)v.boolVal=value?VARIANT_TRUE:VARIANT_FALSE;else v.ulVal=value;
 return api->SetValue(&key,&v);
}
struct GPU {
 ComPtr<ID3D11Device> device;ComPtr<ID3D11DeviceContext> context;
 ComPtr<ID3D11VideoDevice> video;ComPtr<ID3D11VideoContext> videoContext;
 ComPtr<IMFDXGIDeviceManager> manager;
 ComPtr<ID3D11Texture2D> upload,target,staging;
 ComPtr<ID3D11VideoProcessorEnumerator> enumerator;ComPtr<ID3D11VideoProcessor> processor;
 int sw=0,sh=0,dw=0,dh=0;DXGI_FORMAT format=DXGI_FORMAT_UNKNOWN;
 std::string name;
 HRESULT open(IDXGIAdapter *selected=nullptr){
  D3D_FEATURE_LEVEL level;
  CHECK(D3D11CreateDevice(selected,selected?D3D_DRIVER_TYPE_UNKNOWN:D3D_DRIVER_TYPE_HARDWARE,nullptr,D3D11_CREATE_DEVICE_VIDEO_SUPPORT|D3D11_CREATE_DEVICE_BGRA_SUPPORT,nullptr,0,D3D11_SDK_VERSION,&device,&level,&context));
  ComPtr<ID3D10Multithread> threads;CHECK(device.As(&threads));threads->SetMultithreadProtected(TRUE);
  CHECK(device.As(&video));CHECK(context.As(&videoContext));
  UINT token=0;CHECK(MFCreateDXGIDeviceManager(&token,&manager));CHECK(manager->ResetDevice(device.Get(),token));
  ComPtr<IDXGIDevice> dxgi;ComPtr<IDXGIAdapter> adapter;DXGI_ADAPTER_DESC desc{};
  CHECK(device.As(&dxgi));CHECK(dxgi->GetAdapter(&adapter));CHECK(adapter->GetDesc(&desc));
  char text[512]{};WideCharToMultiByte(CP_UTF8,0,desc.Description,-1,text,sizeof(text),nullptr,nullptr);
  name=text;return S_OK;
 }
 HRESULT texture(int w,int h,DXGI_FORMAT fmt,UINT bind,D3D11_USAGE usage,UINT access,ID3D11Texture2D **out){
  D3D11_TEXTURE2D_DESC d{};d.Width=w;d.Height=h;d.MipLevels=1;d.ArraySize=1;d.Format=fmt;d.SampleDesc.Count=1;d.BindFlags=bind;d.Usage=usage;d.CPUAccessFlags=access;
  return device->CreateTexture2D(&d,nullptr,out);
 }
 HRESULT prepare(int iw,int ih,int ow,int oh,DXGI_FORMAT fmt){
  if(sw==iw&&sh==ih&&dw==ow&&dh==oh&&format==fmt&&processor)return S_OK;
  upload.Reset();target.Reset();staging.Reset();processor.Reset();enumerator.Reset();
  D3D11_VIDEO_PROCESSOR_CONTENT_DESC desc{};desc.InputFrameFormat=D3D11_VIDEO_FRAME_FORMAT_PROGRESSIVE;desc.InputWidth=iw;desc.InputHeight=ih;desc.OutputWidth=ow;desc.OutputHeight=oh;desc.InputFrameRate={30,1};desc.OutputFrameRate={30,1};desc.Usage=D3D11_VIDEO_USAGE_PLAYBACK_NORMAL;
  CHECK(video->CreateVideoProcessorEnumerator(&desc,&enumerator));
  UINT support=0;CHECK(enumerator->CheckVideoProcessorFormat(fmt,&support));if(!(support&D3D11_VIDEO_PROCESSOR_FORMAT_SUPPORT_OUTPUT))return E_NOINTERFACE;
  CHECK(video->CreateVideoProcessor(enumerator.Get(),0,&processor));
  CHECK(texture(ow,oh,fmt,D3D11_BIND_RENDER_TARGET,D3D11_USAGE_DEFAULT,0,&target));
  sw=iw;sh=ih;dw=ow;dh=oh;format=fmt;return S_OK;
 }
 HRESULT convert(ID3D11Texture2D *source,UINT sub,int iw,int ih,int ow,int oh,DXGI_FORMAT fmt){
  CHECK(prepare(iw,ih,ow,oh,fmt));
  D3D11_TEXTURE2D_DESC srcDesc{};source->GetDesc(&srcDesc);
  UINT support=0;CHECK(enumerator->CheckVideoProcessorFormat(srcDesc.Format,&support));if(!(support&D3D11_VIDEO_PROCESSOR_FORMAT_SUPPORT_INPUT))return E_NOINTERFACE;
  D3D11_VIDEO_PROCESSOR_INPUT_VIEW_DESC in{};in.ViewDimension=D3D11_VPIV_DIMENSION_TEXTURE2D;in.Texture2D.MipSlice=sub%srcDesc.MipLevels;in.Texture2D.ArraySlice=sub/srcDesc.MipLevels;
  D3D11_VIDEO_PROCESSOR_OUTPUT_VIEW_DESC out{};out.ViewDimension=D3D11_VPOV_DIMENSION_TEXTURE2D;
  ComPtr<ID3D11VideoProcessorInputView> input;ComPtr<ID3D11VideoProcessorOutputView> output;
  CHECK(video->CreateVideoProcessorInputView(source,enumerator.Get(),&in,&input));CHECK(video->CreateVideoProcessorOutputView(target.Get(),enumerator.Get(),&out,&output));
  RECT sr={0,0,iw,ih},dr={0,0,ow,oh};
  videoContext->VideoProcessorSetStreamFrameFormat(processor.Get(),0,D3D11_VIDEO_FRAME_FORMAT_PROGRESSIVE);
  videoContext->VideoProcessorSetStreamAutoProcessingMode(processor.Get(),0,FALSE);
  videoContext->VideoProcessorSetStreamSourceRect(processor.Get(),0,TRUE,&sr);videoContext->VideoProcessorSetStreamDestRect(processor.Get(),0,TRUE,&dr);videoContext->VideoProcessorSetOutputTargetRect(processor.Get(),TRUE,&dr);
  D3D11_VIDEO_PROCESSOR_COLOR_SPACE inputColor{},outputColor{};
  inputColor.YCbCr_Matrix=0;inputColor.Nominal_Range=srcDesc.Format==DXGI_FORMAT_NV12?1:2;
  outputColor.YCbCr_Matrix=0;outputColor.Nominal_Range=fmt==DXGI_FORMAT_NV12?1:2;
  videoContext->VideoProcessorSetStreamColorSpace(processor.Get(),0,&inputColor);videoContext->VideoProcessorSetOutputColorSpace(processor.Get(),&outputColor);
  D3D11_VIDEO_PROCESSOR_STREAM stream{};stream.Enable=TRUE;stream.pInputSurface=input.Get();
  CHECK(videoContext->VideoProcessorBlt(processor.Get(),output.Get(),0,1,&stream));return device->GetDeviceRemovedReason();
 }
 HRESULT rgba(const unsigned char *src,int w,int h,int stride,int ow,int oh,DXGI_FORMAT fmt){
  CHECK(prepare(w,h,ow,oh,fmt));
  // Video Processor 輸入不能僅有 SHADER_RESOURCE；使用規格允許的 RENDER_TARGET。
  if(!upload)CHECK(texture(w,h,DXGI_FORMAT_R8G8B8A8_UNORM,D3D11_BIND_RENDER_TARGET,D3D11_USAGE_DEFAULT,0,&upload));
  context->UpdateSubresource(upload.Get(),0,nullptr,src,stride,0);
  return convert(upload.Get(),0,w,h,ow,oh,fmt);
 }
 HRESULT read(unsigned char *out,int stride){
  if(!staging)CHECK(texture(dw,dh,format,0,D3D11_USAGE_STAGING,D3D11_CPU_ACCESS_READ,&staging));
  context->CopyResource(staging.Get(),target.Get());D3D11_MAPPED_SUBRESOURCE map{};CHECK(context->Map(staging.Get(),0,D3D11_MAP_READ,0,&map));
  int rows=format==DXGI_FORMAT_NV12?dh*3/2:dh;int bytes=format==DXGI_FORMAT_NV12?dw:dw*4;
  if(map.RowPitch<(UINT)bytes){context->Unmap(staging.Get(),0);return E_FAIL;}
  for(int y=0;y<rows;y++)memcpy(out+y*stride,(unsigned char*)map.pData+y*map.RowPitch,bytes);
  context->Unmap(staging.Get(),0);return S_OK;
 }
};
struct Transform {
 ComPtr<IMFTransform> mft;ComPtr<IMFActivate> activation;ComPtr<IMFMediaEventGenerator> events;
 DWORD input=0,output=0;bool async=false,d3d=false,drained=false;unsigned needInput=0,haveOutput=0;
 std::string name;
 ~Transform(){reset();}
 void reset(){if(mft){mft->ProcessMessage(MFT_MESSAGE_COMMAND_FLUSH,0);mft->ProcessMessage(MFT_MESSAGE_NOTIFY_END_STREAMING,0);}events.Reset();mft.Reset();if(activation)activation->ShutdownObject();activation.Reset();needInput=haveOutput=0;drained=false;}
 HRESULT attach(IMFActivate *act,GPU &gpu,bool requireGPU){
  reset();activation=act;CHECK(act->ActivateObject(IID_PPV_ARGS(&mft)));
  WCHAR *label=nullptr;UINT32 length=0;if(SUCCEEDED(act->GetAllocatedString(MFT_FRIENDLY_NAME_Attribute,&label,&length))){char text[512]{};WideCharToMultiByte(CP_UTF8,0,label,-1,text,sizeof(text),nullptr,nullptr);name=text;CoTaskMemFree(label);}
  ComPtr<IMFAttributes> attrs;CHECK(mft->GetAttributes(&attrs));UINT32 value=0;attrs->GetUINT32(MF_TRANSFORM_ASYNC,&value);async=value!=0;
  if(async){CHECK(attrs->SetUINT32(MF_TRANSFORM_ASYNC_UNLOCK,TRUE));CHECK(mft.As(&events));}
  value=0;attrs->GetUINT32(MF_SA_D3D11_AWARE,&value);d3d=value!=0;
  if(requireGPU&&!d3d)return E_NOINTERFACE;
  if(d3d)CHECK(mft->ProcessMessage(MFT_MESSAGE_SET_D3D_MANAGER,(ULONG_PTR)gpu.manager.Get()));
  DWORD ins=0,outs=0;CHECK(mft->GetStreamCount(&ins,&outs));if(ins!=1||outs!=1)return E_NOTIMPL;
  HRESULT ids=mft->GetStreamIDs(1,&input,1,&output);if(ids==E_NOTIMPL)input=output=0;else CHECK(ids);
  attrs->SetUINT32(MF_LOW_LATENCY,TRUE);
  // Microsoft H.264 解碼器使用 VT_UI4，其餘通常使用 VT_BOOL。
  HRESULT latency=setCodec(mft.Get(),CODECAPI_AVLowLatencyMode,1,!requireGPU);
  if(FAILED(latency))latency=setCodec(mft.Get(),CODECAPI_AVLowLatencyMode,1,requireGPU);
  // 低延遲屬性為選用；後續 DRAIN 與實際影格探測才決定可用性。
  if(FAILED(latency))OutputDebugStringA("YourDesk: low-latency property unavailable; using frame drain.\n");
  return S_OK;
 }
 HRESULT begin(){CHECK(mft->ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING,0));return mft->ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM,0);}
 HRESULT pump(){
  if(!async)return S_OK;
  for(int i=0;i<128;i++){
   ComPtr<IMFMediaEvent> event;HRESULT hr=events->GetEvent(MF_EVENT_FLAG_NO_WAIT,&event);if(hr==MF_E_NO_EVENTS_AVAILABLE)return S_OK;CHECK(hr);
   HRESULT status;CHECK(event->GetStatus(&status));CHECK(status);MediaEventType type;CHECK(event->GetType(&type));
   if(type==METransformNeedInput)needInput++;else if(type==METransformHaveOutput)haveOutput++;else if(type==METransformDrainComplete)drained=true;else if(type==MEError)return E_FAIL;
  }return S_OK;
 }
 HRESULT send(IMFSample *sample){
  ULONGLONG end=GetTickCount64()+2000;
  while(async&&!needInput){CHECK(pump());if(GetTickCount64()>=end)return HRESULT_FROM_WIN32(WAIT_TIMEOUT);if(!needInput)Sleep(1);}
  CHECK(mft->ProcessInput(input,sample,0));if(async)needInput--;return S_OK;
 }
 // 每張均為 IDR；排空可解除編解碼器內部的等待，且不會重建 MFT。
 HRESULT startDrain(){
  drained=false;
  CHECK(mft->ProcessMessage(MFT_MESSAGE_COMMAND_DRAIN,0));
  needInput=0;
  return S_OK;
 }
 HRESULT finishDrain(){
  if(async){
   ULONGLONG end=GetTickCount64()+2000;
   while(!drained){CHECK(pump());if(GetTickCount64()>=end)return HRESULT_FROM_WIN32(WAIT_TIMEOUT);if(!drained)Sleep(1);}
   // 單張輸入不應產生第二張影格；不可把它錯配到下一張桌面。
   if(haveOutput)return E_UNEXPECTED;
  }else{
   ComPtr<IMFSample> extra;
   HRESULT hr=receive(extra);
   if(hr!=MF_E_TRANSFORM_NEED_MORE_INPUT)return FAILED(hr)?hr:E_UNEXPECTED;
  }
  needInput=0;drained=false;
  return mft->ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM,0);
 }
 HRESULT receive(ComPtr<IMFSample> &sample){
  ULONGLONG end=GetTickCount64()+2000;
  for(;;){
   CHECK(pump());
   if(async&&!haveOutput){if(drained)return MF_E_TRANSFORM_NEED_MORE_INPUT;if(GetTickCount64()>=end)return HRESULT_FROM_WIN32(WAIT_TIMEOUT);Sleep(1);continue;}
   MFT_OUTPUT_STREAM_INFO info{};CHECK(mft->GetOutputStreamInfo(output,&info));
   MFT_OUTPUT_DATA_BUFFER data{};data.dwStreamID=output;
   ComPtr<IMFSample> supplied;
   if(!(info.dwFlags&MFT_OUTPUT_STREAM_PROVIDES_SAMPLES)){
    CHECK(MFCreateSample(&supplied));ComPtr<IMFMediaBuffer> buffer;
    if(info.cbSize>maxFrame)return E_OUTOFMEMORY;
    CHECK(MFCreateAlignedMemoryBuffer(std::max<DWORD>(info.cbSize,1024*1024),info.cbAlignment,&buffer));CHECK(supplied->AddBuffer(buffer.Get()));data.pSample=supplied.Get();
   }
   DWORD status=0;HRESULT hr=mft->ProcessOutput(0,1,&data,&status);if(data.pEvents)data.pEvents->Release();
   if(async&&haveOutput)haveOutput--;
   if(SUCCEEDED(hr)){
    if(!data.pSample)return E_FAIL;
    if(data.pSample==supplied.Get())sample=supplied;else sample.Attach(data.pSample);
    return S_OK;
   }
   if(data.pSample&&data.pSample!=supplied.Get())data.pSample->Release();
   if(hr==MF_E_TRANSFORM_STREAM_CHANGE)return hr;
   if(hr!=MF_E_TRANSFORM_NEED_MORE_INPUT)return hr;
   // 排空後沒有更多輸出屬正常狀態，交由呼叫者判斷。
   return hr;
  }
 }
};
static HRESULT mediaType(const GUID &subtype,int w,int h,int fps,ComPtr<IMFMediaType> &type){
 CHECK(MFCreateMediaType(&type));CHECK(type->SetGUID(MF_MT_MAJOR_TYPE,MFMediaType_Video));CHECK(type->SetGUID(MF_MT_SUBTYPE,subtype));
 if(w>0&&h>0)CHECK(MFSetAttributeSize(type.Get(),MF_MT_FRAME_SIZE,w,h));
 CHECK(MFSetAttributeRatio(type.Get(),MF_MT_FRAME_RATE,fps,1));CHECK(MFSetAttributeRatio(type.Get(),MF_MT_PIXEL_ASPECT_RATIO,1,1));
 return type->SetUINT32(MF_MT_INTERLACE_MODE,MFVideoInterlace_Progressive);
}
static HRESULT memorySample(const unsigned char *bytes,size_t size,ComPtr<IMFSample> &sample){
 if(size>maxFrame)return E_INVALIDARG;
 CHECK(MFCreateSample(&sample));ComPtr<IMFMediaBuffer> buffer;CHECK(MFCreateMemoryBuffer((DWORD)size,&buffer));BYTE *data=nullptr;CHECK(buffer->Lock(&data,nullptr,nullptr));memcpy(data,bytes,size);buffer->Unlock();CHECK(buffer->SetCurrentLength((DWORD)size));return sample->AddBuffer(buffer.Get());
}
struct Activations {
 IMFActivate **items=nullptr;UINT32 count=0;
 ~Activations(){for(UINT32 i=0;i<count;i++)items[i]->Release();CoTaskMemFree(items);}
};
// 動態取得 API，舊系統缺少 MFTEnum2 時回報不支援，不造成程式載入失敗。
// Windows SDK 的 MFT_ENUM_ADAPTER_LUID；部分 MinGW 標頭尚未宣告。
static const GUID ydAdapterLuid={0x1d39518c,0xe220,0x4da8,{0xa0,0x7f,0xba,0x17,0x25,0x52,0xd6,0xb1}};
using Enum2Fn=HRESULT (WINAPI *)(GUID,UINT32,const MFT_REGISTER_TYPE_INFO*,const MFT_REGISTER_TYPE_INFO*,IMFAttributes*,IMFActivate***,UINT32*);
static HRESULT adapterTransforms(const GUID &category,UINT32 flags,const MFT_REGISTER_TYPE_INFO &in,const MFT_REGISTER_TYPE_INFO &out,const LUID &luid,Activations &acts){
 HMODULE module=GetModuleHandleW(L"mfplat.dll");
 auto enumerate=reinterpret_cast<Enum2Fn>(module?GetProcAddress(module,"MFTEnum2"):nullptr);
 if(!enumerate)return traceFailure(E_NOTIMPL,"MFTEnum2 unavailable");
 ComPtr<IMFAttributes> attrs;CHECK(MFCreateAttributes(&attrs,1));
 CHECK(attrs->SetBlob(ydAdapterLuid,reinterpret_cast<const UINT8*>(&luid),sizeof(luid)));
 return enumerate(category,flags,&in,&out,attrs.Get(),&acts.items,&acts.count);
}
struct yd_media {
 GPU gpu;Transform transform;int mode,w=0,h=0,rate=0,fps=30,quality=0,gop=1;LONGLONG sequence=0;
 bool com=false,mf=false,softwareDecode=false,requestedSoftware=false;GUID decodeCodec=MFVideoFormat_H264,encodeCodec=MFVideoFormat_H264;std::string backend;
 explicit yd_media(int value):mode(value){softwareDecode=requestedSoftware=value==3;}
 ~yd_media(){transform.reset();gpu=GPU();if(mf)MFShutdown();if(com)CoUninitialize();}
 HRESULT open(){CHECK(CoInitializeEx(nullptr,COINIT_MULTITHREADED));com=true;CHECK(MFStartup(MF_VERSION,MFSTARTUP_FULL));mf=true;if(mode==0){CHECK(gpu.open());backend="D3D11 / "+gpu.name;}return S_OK;}
 template<class Configure> HRESULT selectTransform(const GUID &category,UINT32 flags,const MFT_REGISTER_TYPE_INFO &in,const MFT_REGISTER_TYPE_INFO &out,Configure configure){
  transform.reset();gpu=GPU();
  ComPtr<IDXGIFactory1> factory;CHECK(CreateDXGIFactory1(IID_PPV_ARGS(&factory)));
  HRESULT result=E_NOINTERFACE;
  for(UINT index=0;index<32;index++){
   ComPtr<IDXGIAdapter1> adapter;HRESULT hr=factory->EnumAdapters1(index,&adapter);
   if(hr==DXGI_ERROR_NOT_FOUND)break;CHECK(hr);
   DXGI_ADAPTER_DESC1 desc{};CHECK(adapter->GetDesc1(&desc));
   if(desc.Flags&DXGI_ADAPTER_FLAG_SOFTWARE)continue;
   Activations acts;result=adapterTransforms(category,flags,in,out,desc.AdapterLuid,acts);
   if(FAILED(result)||!acts.count){if(!acts.count&&SUCCEEDED(result))traceFailure(MF_E_TOPO_CODEC_NOT_FOUND,"MFTEnum2: no matching codec for adapter");continue;}
   transform.reset();gpu=GPU();result=gpu.open(adapter.Get());if(FAILED(result))continue;
   for(UINT32 i=0;i<acts.count;i++){
    ydFailure[0]=0;result=configure(acts.items[i]);
    if(SUCCEEDED(result))return S_OK;
    transform.reset();
   }
  }
  transform.reset();return FAILED(result)?result:E_NOINTERFACE;
 }
 HRESULT encoder(int width,int height,int bitrate,int frames,int q,int keyInterval){
  if(transform.mft&&w==width&&h==height&&rate==bitrate&&fps==frames&&quality==q&&gop==keyInterval)return S_OK;
  MFT_REGISTER_TYPE_INFO in={MFMediaType_Video,MFVideoFormat_NV12},out={MFMediaType_Video,encodeCodec};
  HRESULT result=selectTransform(MFT_CATEGORY_VIDEO_ENCODER,MFT_ENUM_FLAG_HARDWARE|MFT_ENUM_FLAG_SORTANDFILTER,in,out,[&](IMFActivate *act)->HRESULT{
    CHECK(transform.attach(act,gpu,false));ComPtr<IMFMediaType> input,output;
    CHECK(mediaType(encodeCodec,width,height,frames,output));// Quality 模式忽略平均碼率；保留正值以滿足媒體型別格式要求。
    CHECK(output->SetUINT32(MF_MT_AVG_BITRATE,bitrate>0?bitrate:1));if(encodeCodec==MFVideoFormat_H264)output->SetUINT32(MF_MT_MPEG2_PROFILE,eAVEncH264VProfile_Base);
    CHECK(setCodec(transform.mft.Get(),CODECAPI_AVEncCommonRateControlMode,bitrate>0?eAVEncCommonRateControlMode_CBR:eAVEncCommonRateControlMode_Quality));
    if(bitrate<=0)CHECK(setCodec(transform.mft.Get(),CODECAPI_AVEncCommonQuality,q));
    CHECK(transform.mft->SetOutputType(transform.output,output.Get(),0));CHECK(mediaType(MFVideoFormat_NV12,width,height,frames,input));CHECK(transform.mft->SetInputType(transform.input,input.Get(),0));
    if(bitrate>0)CHECK(setCodec(transform.mft.Get(),CODECAPI_AVEncCommonMeanBitRate,bitrate));
    // GOP 關閉 B 幀重排，第一張與每個週期要求 IDR。
    HRESULT intervalResult=setCodec(transform.mft.Get(),CODECAPI_AVEncMPVGOPSize,keyInterval);
    setCodec(transform.mft.Get(),CODECAPI_AVEncMPVDefaultBPictureCount,0);
    HRESULT force=setCodec(transform.mft.Get(),CODECAPI_AVEncVideoForceKeyFrame,1);
    if(FAILED(intervalResult)&&FAILED(force))return force;
    CHECK(transform.begin());return S_OK;
   });
  if(FAILED(result)){transform.reset();return result;}
  w=width;h=height;rate=bitrate;fps=frames;quality=q;gop=keyInterval;sequence=0;
  backend=std::string(encodeCodec==MFVideoFormat_AV1?"Media Foundation AV1 encoder / ":"Media Foundation H.264 encoder / ")+transform.name+" / "+gpu.name+(transform.d3d?" / GPU NV12":" / CPU NV12 transfer");return S_OK;
 }
 HRESULT decodeOutput(){
  for(DWORD i=0;i<64;i++){
   ComPtr<IMFMediaType> type;HRESULT hr=transform.mft->GetOutputAvailableType(transform.output,i,&type);if(hr==MF_E_NO_MORE_TYPES)break;CHECK(hr);
   GUID subtype{};if(FAILED(type->GetGUID(MF_MT_SUBTYPE,&subtype))||subtype!=MFVideoFormat_NV12)continue;
   if(SUCCEEDED(transform.mft->SetOutputType(transform.output,type.Get(),0)))return S_OK;
  }return MF_E_INVALIDMEDIATYPE;
 }
 HRESULT softwareDecoder(int width,int height){
  transform.reset();gpu=GPU();
  MFT_REGISTER_TYPE_INFO in={MFMediaType_Video,decodeCodec},out={MFMediaType_Video,MFVideoFormat_NV12};
  Activations acts;CHECK(MFTEnumEx(MFT_CATEGORY_VIDEO_DECODER,MFT_ENUM_FLAG_SYNCMFT|MFT_ENUM_FLAG_ASYNCMFT|MFT_ENUM_FLAG_SORTANDFILTER,&in,&out,&acts.items,&acts.count));
  HRESULT result=MF_E_TOPO_CODEC_NOT_FOUND;
  for(UINT32 i=0;i<acts.count;i++){
   result=[&]()->HRESULT{
    CHECK(transform.attach(acts.items[i],gpu,false));ComPtr<IMFMediaType> input;
    CHECK(mediaType(decodeCodec,width,height,30,input));CHECK(transform.mft->SetInputType(transform.input,input.Get(),0));
    HRESULT hr=decodeOutput();if(FAILED(hr)&&hr!=MF_E_TRANSFORM_TYPE_NOT_SET&&hr!=MF_E_INVALIDMEDIATYPE)return hr;
    return transform.begin();
   }();
   if(SUCCEEDED(result)){
    softwareDecode=true;w=width;h=height;
    backend="Media Foundation software decoder / "+transform.name;return S_OK;
   }
   transform.reset();
  }
  return result;
 }
 HRESULT decoder(int width,int height){
  if(transform.mft&&w==width&&h==height)return S_OK;
  if(softwareDecode)return softwareDecoder(width,height);
  transform.reset();
  MFT_REGISTER_TYPE_INFO in={MFMediaType_Video,decodeCodec},out={MFMediaType_Video,MFVideoFormat_NV12};
  HRESULT result=selectTransform(MFT_CATEGORY_VIDEO_DECODER,MFT_ENUM_FLAG_HARDWARE|MFT_ENUM_FLAG_SYNCMFT|MFT_ENUM_FLAG_ASYNCMFT|MFT_ENUM_FLAG_SORTANDFILTER,in,out,[&](IMFActivate *act)->HRESULT{
    CHECK(transform.attach(act,gpu,true));ComPtr<IMFMediaType> input;
    CHECK(mediaType(decodeCodec,width,height,30,input));
    CHECK(transform.mft->SetInputType(transform.input,input.Get(),0));
    // 某些解碼器需先讀入 SPS 才提供完整的輸出格式。
    HRESULT hr=decodeOutput();if(FAILED(hr)&&hr!=MF_E_TRANSFORM_TYPE_NOT_SET&&hr!=MF_E_INVALIDMEDIATYPE)return hr;
    return transform.begin();
   });
  if(FAILED(result))return mode==4?result:softwareDecoder(width,height);
  w=width;h=height;backend="Media Foundation D3D11 video decoder / "+transform.name+" / "+gpu.name;return result;
 }
};
extern "C" int yd_media_open(int mode,yd_media **out) try {
 ydFailure[0]=0;
 if(!out||mode<0||mode>4)return E_INVALIDARG;
 *out=nullptr;std::unique_ptr<yd_media> media(new yd_media(mode));
 CHECK(media->open());*out=media.release();return S_OK;
} catch(const std::bad_alloc &){return E_OUTOFMEMORY;} catch(...){return E_FAIL;}
extern "C" const char *yd_media_backend(yd_media *media){return media->backend.c_str();}
extern "C" void yd_media_close(yd_media *media){delete media;}
extern "C" void yd_media_free(yd_media_output *out){free(out->data);free(out->config);memset(out,0,sizeof(*out));}
extern "C" int yd_media_scale(yd_media *m,const unsigned char *src,int w,int h,int stride,unsigned char *dst,int dw,int dh,int dstStride)try {
 ydFailure[0]=0;
 if(!m||!src||!dst||!validSize(w,h)||!validSize(dw,dh)||stride<w*4||dstStride<dw*4)return E_INVALIDARG;
 CHECK(m->gpu.rgba(src,w,h,stride,dw,dh,DXGI_FORMAT_R8G8B8A8_UNORM));return m->gpu.read(dst,dstStride);
} catch(const std::bad_alloc &){return E_OUTOFMEMORY;} catch(...){return E_FAIL;}
extern "C" int yd_media_encode(yd_media *m,const unsigned char *src,int w,int h,int stride,int bitrate,int fps,int quality,int gop,yd_media_output *out)try {
 ydFailure[0]=0;
 if(!m||!src||!validSize(w,h)||(w&1)||(h&1)||stride<w*4||fps<1||fps>240||gop<1||gop>300)return E_INVALIDARG;
 CHECK(m->encoder(w,h,bitrate,fps,quality,gop));CHECK(m->gpu.rgba(src,w,h,stride,w,h,DXGI_FORMAT_NV12));
 ComPtr<IMFSample> input;ComPtr<IMFMediaBuffer> buffer;
 if(m->transform.d3d){CHECK(MFCreateSample(&input));CHECK(MFCreateDXGISurfaceBuffer(__uuidof(ID3D11Texture2D),m->gpu.target.Get(),0,FALSE,&buffer));CHECK(buffer->SetCurrentLength((DWORD)((size_t)w*h*3/2)));CHECK(input->AddBuffer(buffer.Get()));}
 else {std::vector<unsigned char> pixels((size_t)w*h*3/2);CHECK(m->gpu.read(pixels.data(),w));CHECK(memorySample(pixels.data(),pixels.size(),input));}
 input->SetSampleTime(m->sequence*10000000/fps);input->SetSampleDuration(10000000/fps);m->sequence++;
 if((m->sequence-1)%m->gop==0)CHECK(setCodec(m->transform.mft.Get(),CODECAPI_AVEncVideoForceKeyFrame,1));
 CHECK(m->transform.send(input.Get()));if(m->gop==1)CHECK(m->transform.startDrain());ComPtr<IMFSample> sample;CHECK(m->transform.receive(sample));if(m->gop==1)CHECK(m->transform.finishDrain());
 CHECK(sample->ConvertToContiguousBuffer(&buffer));BYTE *data=nullptr;DWORD size=0;CHECK(buffer->Lock(&data,nullptr,&size));
 if(size==0||size>maxFrame){buffer->Unlock();return E_FAIL;}
 out->data=(unsigned char*)malloc(size);if(!out->data){buffer->Unlock();return E_OUTOFMEMORY;}memcpy(out->data,data,size);out->size=size;buffer->Unlock();
 ComPtr<IMFMediaType> type;if(SUCCEEDED(m->transform.mft->GetOutputCurrentType(m->transform.output,&type))){
  UINT32 bytes=0;if(SUCCEEDED(type->GetBlobSize(MF_MT_MPEG_SEQUENCE_HEADER,&bytes))&&bytes>0&&bytes<65536){out->config=(unsigned char*)malloc(bytes);if(!out->config)return E_OUTOFMEMORY;CHECK(type->GetBlob(MF_MT_MPEG_SEQUENCE_HEADER,out->config,bytes,&bytes));out->config_size=bytes;}
 }
 return S_OK;
} catch(const std::bad_alloc &){return E_OUTOFMEMORY;} catch(...){return E_FAIL;}
static int decodeFrame(yd_media *m,const unsigned char *data,size_t size,int width,int height,yd_media_output *out)try {
 ydFailure[0]=0;
 if(!m||!data||size==0||size>maxFrame)return E_INVALIDARG;
 if(!validSize(width,height)&&!(width==0&&height==0&&(m->decodeCodec==MFVideoFormat_HEVC||m->decodeCodec==MFVideoFormat_AV1)))return E_INVALIDARG;
 CHECK(m->decoder(width,height));ComPtr<IMFSample> input;CHECK(memorySample(data,size,input));
 input->SetSampleTime(m->sequence*333333);input->SetSampleDuration(333333);m->sequence++;
 CHECK(m->transform.send(input.Get()));if(m->softwareDecode)CHECK(m->transform.startDrain());ComPtr<IMFSample> sample;
 HRESULT hr=m->transform.receive(sample);
 for(int changes=0;hr==MF_E_TRANSFORM_STREAM_CHANGE&&changes<4;changes++){CHECK(m->decodeOutput());hr=m->transform.receive(sample);}CHECK(hr);
 if(m->softwareDecode){
  ComPtr<IMFMediaType> type;CHECK(m->transform.mft->GetOutputCurrentType(m->transform.output,&type));
  UINT32 w=0,h=0;CHECK(MFGetAttributeSize(type.Get(),MF_MT_FRAME_SIZE,&w,&h));
  if(!validSize(w,h)||(w&1)||(h&1))return E_INVALIDARG;
  ComPtr<IMFMediaBuffer> buffer;CHECK(sample->ConvertToContiguousBuffer(&buffer));
  std::vector<unsigned char> pixels;UINT32 stride=w;ComPtr<IMF2DBuffer> twoD;
  if(SUCCEEDED(buffer.As(&twoD))){
   DWORD bytes=0;CHECK(twoD->GetContiguousLength(&bytes));if(bytes>maxFrame)return E_INVALIDARG;
   pixels.resize(bytes);CHECK(twoD->ContiguousCopyTo(pixels.data(),bytes));
  }else{
   type->GetUINT32(MF_MT_DEFAULT_STRIDE,&stride);
   if(stride<w||stride>maxFrame)return E_INVALIDARG;
   BYTE *data=nullptr;DWORD bytes=0;CHECK(buffer->Lock(&data,nullptr,&bytes));
   if(bytes>maxFrame){buffer->Unlock();return E_INVALIDARG;}
   try{pixels.assign(data,data+bytes);}catch(...){buffer->Unlock();throw;}buffer->Unlock();
  }
  if(pixels.size()<(size_t)stride*h*3/2)return E_FAIL;
  UINT32 matrix=MFVideoTransferMatrix_BT601,range=MFNominalRange_16_235;
  type->GetUINT32(MF_MT_YUV_MATRIX,&matrix);type->GetUINT32(MF_MT_VIDEO_NOMINAL_RANGE,&range);
  bool full=range==MFNominalRange_0_255,bt709=matrix==MFVideoTransferMatrix_BT709;
  out->size=(size_t)w*h*4;out->data=(unsigned char*)malloc(out->size);if(!out->data)return E_OUTOFMEMORY;out->width=w;out->height=h;
  auto clamp=[](int v)->unsigned char{return (unsigned char)std::max(0,std::min(255,v));};
  for(UINT32 y=0;y<h;y++)for(UINT32 x=0;x<w;x++){
   int l=pixels[(size_t)y*stride+x],u=pixels[(size_t)stride*h+(y/2)*stride+(x&~1)]-128,v=pixels[(size_t)stride*h+(y/2)*stride+(x&~1)+1]-128;
   int c=full?256*l:298*(l-16);
   size_t p=((size_t)y*w+x)*4;
   out->data[p]=clamp((c+(full?(bt709?403:359):(bt709?459:409))*v+128)>>8);
   out->data[p+1]=clamp((c-(full?(bt709?48:88):(bt709?55:100))*u-(full?(bt709?120:183):(bt709?136:208))*v+128)>>8);
   out->data[p+2]=clamp((c+(full?(bt709?475:454):(bt709?541:516))*u+128)>>8);out->data[p+3]=255;
  }
  CHECK(m->transform.finishDrain());return S_OK;
 }
 ComPtr<IMFMediaBuffer> buffer;CHECK(sample->GetBufferByIndex(0,&buffer));ComPtr<IMFDXGIBuffer> dxgi;CHECK(buffer.As(&dxgi));
 ComPtr<ID3D11Texture2D> texture;UINT sub=0;CHECK(dxgi->GetResource(IID_PPV_ARGS(&texture)));CHECK(dxgi->GetSubresourceIndex(&sub));
 D3D11_TEXTURE2D_DESC desc{};texture->GetDesc(&desc);UINT32 w=0,h=0;ComPtr<IMFMediaType> type;
 CHECK(m->transform.mft->GetOutputCurrentType(m->transform.output,&type));CHECK(MFGetAttributeSize(type.Get(),MF_MT_FRAME_SIZE,&w,&h));
 if(!validSize(w,h)||w>desc.Width||h>desc.Height)return E_INVALIDARG;
 CHECK(m->gpu.convert(texture.Get(),sub,w,h,w,h,DXGI_FORMAT_R8G8B8A8_UNORM));
 out->size=(size_t)w*h*4;out->data=(unsigned char*)malloc(out->size);if(!out->data)return E_OUTOFMEMORY;out->width=w;out->height=h;
 return m->gpu.read(out->data,w*4);
} catch(const std::bad_alloc &){return E_OUTOFMEMORY;} catch(...){return E_FAIL;}

// 診斷使用直接 MFT 輸入，不經 RGBA 轉 NV12，也不改動正式串流工作階段。
static HRESULT probeOutputType(Transform &t,const GUID &raw,int range){
 for(DWORD i=0;i<128;i++){
  ComPtr<IMFMediaType> type;HRESULT hr=t.mft->GetOutputAvailableType(t.output,i,&type);
  if(hr==MF_E_NO_MORE_TYPES)break;CHECK(hr);
  GUID subtype{};if(FAILED(type->GetGUID(MF_MT_SUBTYPE,&subtype))||subtype!=raw)continue;
  CHECK(type->SetUINT32(MF_MT_VIDEO_NOMINAL_RANGE,range));
  if(SUCCEEDED(t.mft->SetOutputType(t.output,type.Get(),0)))return S_OK;
 }
 return MF_E_INVALIDMEDIATYPE;
}
// 在同一類候選中實際送入與接收影格，失敗後繼續下一個 MFT。
template<class Run> static HRESULT probeCandidates(bool softwareOnly,bool encoding,const GUID &compressed,const GUID &raw,Run run){
 MFT_REGISTER_TYPE_INFO in={MFMediaType_Video,encoding?raw:compressed},out={MFMediaType_Video,encoding?compressed:raw};
 GUID category=encoding?MFT_CATEGORY_VIDEO_ENCODER:MFT_CATEGORY_VIDEO_DECODER;
 yd_media hardwareMedia(1);
 HRESULT last=MF_E_TOPO_CODEC_NOT_FOUND;
 if(!softwareOnly)last=hardwareMedia.selectTransform(category,MFT_ENUM_FLAG_HARDWARE|MFT_ENUM_FLAG_SORTANDFILTER,in,out,[&](IMFActivate *act)->HRESULT{
  CHECK(hardwareMedia.transform.attach(act,hardwareMedia.gpu,false));
  return run(hardwareMedia.transform,hardwareMedia.gpu,1);
 });
 if(SUCCEEDED(last))return last;
 Activations acts;
 HRESULT hr=MFTEnumEx(category,MFT_ENUM_FLAG_SORTANDFILTER|MFT_ENUM_FLAG_SYNCMFT|MFT_ENUM_FLAG_ASYNCMFT|MFT_ENUM_FLAG_LOCALMFT,&in,&out,&acts.items,&acts.count);
 if(FAILED(hr))return hr;
 // 系統解碼器可能透過 D3D11 加速，並非 HARDWARE 類別 MFT。
 // 額外實測 GPU 緩衝路徑，成功仍保留加速未知，不僅憑 D3D-aware 宣告硬解。
 for(int acceleration=(softwareOnly||encoding)?0:-1;acceleration<=0;acceleration++){
  for(UINT32 i=0;i<acts.count;i++){
   GPU gpu;Transform t;ydFailure[0]=0;
   if(acceleration<0&&FAILED(gpu.open()))continue;
   hr=t.attach(acts.items[i],gpu,acceleration<0);
   if(SUCCEEDED(hr))hr=run(t,gpu,acceleration);
   if(SUCCEEDED(hr))return S_OK;
   last=hr;
  }
 }
 return last;
}
static HRESULT probeBuffer(IMFSample *sample,std::vector<unsigned char> &bytes){
 ComPtr<IMFMediaBuffer> buffer;CHECK(sample->ConvertToContiguousBuffer(&buffer));
 BYTE *data=nullptr;DWORD size=0;CHECK(buffer->Lock(&data,nullptr,&size));
 if(!size||size>maxFrame){buffer->Unlock();return E_FAIL;}
 try {bytes.assign(data,data+size);}catch(...){buffer->Unlock();throw;}
 return buffer->Unlock();
}
static int probeCodec(bool softwareOnly,int codec,int format,int w,int h,yd_media_probe_result *out)try {
 if(!out)return E_INVALIDARG;
 memset(out,0,sizeof(*out));out->encode_status=out->decode_status=E_PENDING;
 out->hardware_encoder=out->hardware_decoder=-1;
 if(codec<0||codec>3||format<0||format>2||!validSize(w,h)||(w&1)||(h&1))return E_INVALIDARG;
 yd_media lifetime(1);HRESULT hr=lifetime.open();if(FAILED(hr)){out->encode_status=hr;return hr;}
 const GUID compressed=codec==0?MFVideoFormat_MJPG:codec==1?MFVideoFormat_H264:codec==3?MFVideoFormat_AV1:MFVideoFormat_HEVC;
 const GUID raw=format==0?MFVideoFormat_ARGB32:MFVideoFormat_NV12;
 const int range=format==2?MFNominalRange_16_235:MFNominalRange_0_255;
 std::vector<unsigned char> pixels((size_t)w*h*(format==0?4:3)/ (format==0?1:2));
 // 灰階漸層涵蓋合法範圍；NV12 色度固定中性，不以改標籤冒充格式轉換。
 for(int y=0;y<h;y++)for(int x=0;x<w;x++){
  unsigned char luma=(unsigned char)((format==2?16:0)+(x* (format==2?219:255)/std::max(1,w-1)));
  if(format==0){size_t p=((size_t)y*w+x)*4;pixels[p]=pixels[p+1]=pixels[p+2]=luma;pixels[p+3]=255;}
  else pixels[(size_t)y*w+x]=luma;
 }
 if(format!=0)std::fill(pixels.begin()+(size_t)w*h,pixels.end(),128);
 std::vector<unsigned char> encoded;ComPtr<IMFMediaType> encodedType;
 hr=probeCandidates(softwareOnly,true,compressed,raw,[&](Transform &t,GPU &gpu,int hardware)->HRESULT{
  ComPtr<IMFMediaType> input,output;
  CHECK(mediaType(compressed,w,h,30,output));
  if(codec!=0)CHECK(output->SetUINT32(MF_MT_AVG_BITRATE,std::max(1000000,w*h*4)));
  CHECK(output->SetUINT32(MF_MT_VIDEO_NOMINAL_RANGE,range));
  CHECK(t.mft->SetOutputType(t.output,output.Get(),0));
  CHECK(mediaType(raw,w,h,30,input));CHECK(input->SetUINT32(MF_MT_VIDEO_NOMINAL_RANGE,range));
  CHECK(input->SetUINT32(MF_MT_DEFAULT_STRIDE,format==0?w*4:w));
  CHECK(t.mft->SetInputType(t.input,input.Get(),0));
  setCodec(t.mft.Get(),CODECAPI_AVEncMPVGOPSize,1);setCodec(t.mft.Get(),CODECAPI_AVEncMPVDefaultBPictureCount,0);
  CHECK(t.begin());ComPtr<IMFSample> sample;
  ComPtr<ID3D11Texture2D> upload;
  if(hardware&&t.d3d){
   CHECK(gpu.texture(w,h,format==0?DXGI_FORMAT_B8G8R8A8_UNORM:DXGI_FORMAT_NV12,D3D11_BIND_RENDER_TARGET,D3D11_USAGE_DEFAULT,0,&upload));
   gpu.context->UpdateSubresource(upload.Get(),0,nullptr,pixels.data(),format==0?w*4:w,0);
   ComPtr<IMFMediaBuffer> buffer;CHECK(MFCreateSample(&sample));
   CHECK(MFCreateDXGISurfaceBuffer(__uuidof(ID3D11Texture2D),upload.Get(),0,FALSE,&buffer));
   CHECK(buffer->SetCurrentLength((DWORD)pixels.size()));CHECK(sample->AddBuffer(buffer.Get()));
  }else CHECK(memorySample(pixels.data(),pixels.size(),sample));
  CHECK(sample->SetSampleTime(0));CHECK(sample->SetSampleDuration(333333));
  CHECK(t.send(sample.Get()));CHECK(t.startDrain());ComPtr<IMFSample> result;CHECK(t.receive(result));
  CHECK(probeBuffer(result.Get(),encoded));
  CHECK(t.mft->GetOutputCurrentType(t.output,&encodedType));
  ComPtr<IMFMediaType> negotiated;CHECK(t.mft->GetInputCurrentType(t.input,&negotiated));
  UINT32 nominal=0;negotiated->GetUINT32(MF_MT_VIDEO_NOMINAL_RANGE,&nominal);out->input_range=nominal;
  out->encoder_d3d11=hardware&&t.d3d;out->hardware_encoder=hardware;snprintf(out->encoder,sizeof(out->encoder),"Media Foundation / %s",t.name.c_str());
  return S_OK;
 });
 out->encode_status=hr;out->encode_ok=SUCCEEDED(hr);
 return S_OK;
} catch(const std::bad_alloc &){return E_OUTOFMEMORY;}catch(...){return E_FAIL;}

extern "C" int yd_media_probe_decode(int codec,int format,int w,int h,const unsigned char *fixture,size_t size,yd_media_probe_result *out)try {
 if(!out)return E_INVALIDARG;
 memset(out,0,sizeof(*out));out->hardware_encoder=out->hardware_decoder=-1;out->decoder_attempted=1;
 out->decode_status=E_PENDING;
 if(codec<0||codec>3||format<0||format>2||!validSize(w,h)||!fixture||!size)return E_INVALIDARG;
 yd_media lifetime(1);HRESULT hr=lifetime.open();if(FAILED(hr))return hr;
 const GUID compressed=codec==0?MFVideoFormat_MJPG:codec==1?MFVideoFormat_H264:codec==3?MFVideoFormat_AV1:MFVideoFormat_HEVC;
 const GUID raw=format==0?MFVideoFormat_ARGB32:MFVideoFormat_NV12;
 const int range=format==2?MFNominalRange_16_235:MFNominalRange_0_255;
 ComPtr<IMFMediaType> encodedType;CHECK(mediaType(compressed,w,h,30,encodedType));

 hr=probeCandidates(false,false,compressed,raw,[&](Transform &t,GPU &gpu,int hardware)->HRESULT{
  CHECK(t.mft->SetInputType(t.input,encodedType.Get(),0));
  HRESULT initial=probeOutputType(t,raw,range);
  if(FAILED(initial)&&initial!=MF_E_TRANSFORM_TYPE_NOT_SET&&initial!=MF_E_INVALIDMEDIATYPE)return initial;
  CHECK(t.begin());ComPtr<IMFSample> input;CHECK(memorySample(fixture,size,input));
  CHECK(input->SetSampleTime(0));CHECK(input->SetSampleDuration(333333));
  CHECK(t.send(input.Get()));CHECK(t.startDrain());ComPtr<IMFSample> decoded;
  HRESULT status=t.receive(decoded);
  for(int changes=0;status==MF_E_TRANSFORM_STREAM_CHANGE&&changes<4;changes++){
   CHECK(probeOutputType(t,raw,range));status=t.receive(decoded);
  }
  CHECK(status);ComPtr<IMFMediaType> type;CHECK(t.mft->GetOutputCurrentType(t.output,&type));
  UINT32 width=0,height=0;CHECK(MFGetAttributeSize(type.Get(),MF_MT_FRAME_SIZE,&width,&height));
  if(width!=(UINT32)w||height!=(UINT32)h)return MF_E_INVALIDMEDIATYPE;
  GUID subtype{};CHECK(type->GetGUID(MF_MT_SUBTYPE,&subtype));if(subtype!=raw)return MF_E_INVALIDMEDIATYPE;
  ComPtr<IMFMediaBuffer> buffer;CHECK(decoded->GetBufferByIndex(0,&buffer));
  ComPtr<IMFDXGIBuffer> dxgi;
  if(SUCCEEDED(buffer.As(&dxgi))){
   if(!gpu.device||!gpu.context)return E_UNEXPECTED;
   ComPtr<ID3D11Texture2D> texture;UINT sub=0;CHECK(dxgi->GetResource(IID_PPV_ARGS(&texture)));CHECK(dxgi->GetSubresourceIndex(&sub));
   D3D11_TEXTURE2D_DESC desc{};texture->GetDesc(&desc);
   if(desc.Width<(UINT)w||desc.Height<(UINT)h||desc.Format!=(format==0?DXGI_FORMAT_B8G8R8A8_UNORM:DXGI_FORMAT_NV12))return E_FAIL;
   ComPtr<ID3D11Texture2D> staging;CHECK(gpu.texture(desc.Width,desc.Height,desc.Format,0,D3D11_USAGE_STAGING,D3D11_CPU_ACCESS_READ,&staging));
   gpu.context->CopySubresourceRegion(staging.Get(),0,0,0,0,texture.Get(),sub,nullptr);
   D3D11_MAPPED_SUBRESOURCE mapped{};CHECK(gpu.context->Map(staging.Get(),0,D3D11_MAP_READ,0,&mapped));
   bool valid=mapped.pData&&mapped.RowPitch>=(UINT)(format==0?w*4:w);
   gpu.context->Unmap(staging.Get(),0);if(!valid)return E_FAIL;
  }else{
   std::vector<unsigned char> output;CHECK(probeBuffer(decoded.Get(),output));
   if(output.size()<(size_t)w*h*(format==0?4:3)/(format==0?1:2))return E_FAIL;
  }
  UINT32 nominal=0;type->GetUINT32(MF_MT_VIDEO_NOMINAL_RANGE,&nominal);out->output_range=nominal;
  out->decoder_d3d11=hardware!=0&&t.d3d;out->hardware_decoder=hardware;snprintf(out->decoder,sizeof(out->decoder),"Media Foundation / %s",t.name.c_str());
  return S_OK;
 });
 out->decode_status=hr;out->decode_ok=SUCCEEDED(hr);return S_OK;
} catch(const std::bad_alloc &){return E_OUTOFMEMORY;}catch(...){return E_FAIL;}

// 逐顯示卡實際建立裝置，不依 DLL 存在或作業系統版本推論 API 支援。
extern "C" int yd_graphics_probe(unsigned int index,yd_graphics_probe_result *out)try {
 if(!out||index>=32)return E_INVALIDARG;
 memset(out,0,sizeof(*out));out->d3d11_status=out->d3d12_status=E_NOINTERFACE;
 ComPtr<IDXGIFactory1> factory;CHECK(CreateDXGIFactory1(IID_PPV_ARGS(&factory)));
 ComPtr<IDXGIAdapter1> adapter;HRESULT hr=factory->EnumAdapters1(index,&adapter);
 if(hr==DXGI_ERROR_NOT_FOUND)return S_FALSE;CHECK(hr);
 DXGI_ADAPTER_DESC1 desc{};CHECK(adapter->GetDesc1(&desc));
 if(desc.Flags&DXGI_ADAPTER_FLAG_SOFTWARE)return 2;
 WideCharToMultiByte(CP_UTF8,0,desc.Description,-1,out->name,sizeof(out->name),nullptr,nullptr);
 {
  ComPtr<ID3D11Device> device;D3D_FEATURE_LEVEL level{};
  const D3D_FEATURE_LEVEL levels[]={D3D_FEATURE_LEVEL_12_1,D3D_FEATURE_LEVEL_12_0,D3D_FEATURE_LEVEL_11_1,D3D_FEATURE_LEVEL_11_0,D3D_FEATURE_LEVEL_10_1,D3D_FEATURE_LEVEL_10_0,D3D_FEATURE_LEVEL_9_3,D3D_FEATURE_LEVEL_9_2,D3D_FEATURE_LEVEL_9_1};
  // 由高至低要求 FL；舊 runtime 不認得最高列舉值時移除該值再試。
  // 其他裝置錯誤不降級；成功回報 API 實際選到的最高 FL。
  for(UINT first=0;first<9;first++){
   hr=D3D11CreateDevice(adapter.Get(),D3D_DRIVER_TYPE_UNKNOWN,nullptr,0,levels+first,9-first,D3D11_SDK_VERSION,&device,&level,nullptr);
   if(hr!=E_INVALIDARG)break;
  }
  out->d3d11_status=hr;if(SUCCEEDED(hr))out->d3d11_level=level;
 }
 // 動態載入，缺少 D3D12 的 Windows 仍可正常啟動及測試 D3D11。
 HMODULE module=LoadLibraryExW(L"d3d12.dll",nullptr,LOAD_LIBRARY_SEARCH_SYSTEM32);
 if(!module){out->d3d12_status=HRESULT_FROM_WIN32(GetLastError());return S_OK;}
 using Create12=HRESULT(WINAPI *)(IUnknown *,D3D_FEATURE_LEVEL,REFIID,void **);
 auto create=reinterpret_cast<Create12>(GetProcAddress(module,"D3D12CreateDevice"));
 {
  ComPtr<ID3D12Device> device;
  hr=create?create(adapter.Get(),D3D_FEATURE_LEVEL_11_0,__uuidof(ID3D12Device),reinterpret_cast<void **>(device.GetAddressOf())):E_NOTIMPL;
  out->d3d12_status=hr;
  if(SUCCEEDED(hr)){
   const D3D_FEATURE_LEVEL levels[]={D3D_FEATURE_LEVEL_12_2,D3D_FEATURE_LEVEL_12_1,D3D_FEATURE_LEVEL_12_0,D3D_FEATURE_LEVEL_11_1,D3D_FEATURE_LEVEL_11_0};
   for(UINT first=0;first<5;first++){
    D3D12_FEATURE_DATA_FEATURE_LEVELS support{5-first,levels+first,D3D_FEATURE_LEVEL_11_0};
    HRESULT query=device->CheckFeatureSupport(D3D12_FEATURE_FEATURE_LEVELS,&support,sizeof(support));
    if(SUCCEEDED(query)){out->d3d12_level=support.MaxSupportedFeatureLevel;break;}
    if(query!=E_INVALIDARG)break;
   }
  }
 }
 FreeLibrary(module);return S_OK;
} catch(const std::bad_alloc &){return E_OUTOFMEMORY;}catch(...){return E_FAIL;}

extern "C" int yd_media_decode_format(yd_media *m,int codec,const unsigned char *data,size_t size,int w,int h,yd_media_output *out){
 if(!m||!out||(codec!=1&&codec!=2&&codec!=3))return E_INVALIDARG;
 GUID kind=codec==3?MFVideoFormat_AV1:codec==2?MFVideoFormat_HEVC:MFVideoFormat_H264;
 if(m->decodeCodec!=kind){m->transform.reset();m->decodeCodec=kind;m->softwareDecode=m->requestedSoftware;m->sequence=0;}
 HRESULT hr=decodeFrame(m,data,size,w,h,out);
 if(FAILED(hr)&&!m->softwareDecode&&m->mode!=4){
  yd_media_free(out);m->transform.reset();m->softwareDecode=true;m->sequence=0;
  hr=decodeFrame(m,data,size,w,h,out);
 }
 return hr;
}
extern "C" int yd_media_decode(yd_media *m,const unsigned char *data,size_t size,int w,int h,yd_media_output *out){return yd_media_decode_format(m,1,data,size,w,h,out);}
extern "C" int yd_media_decoder_software(yd_media *m){return m&&m->softwareDecode;}

extern "C" int yd_media_probe(int codec,int format,int w,int h,yd_media_probe_result *out){return probeCodec(false,codec,format,w,h,out);}
extern "C" int yd_media_probe_software(int codec,int format,int w,int h,yd_media_probe_result *out){return probeCodec(true,codec,format,w,h,out);}

extern "C" int yd_media_encode_format(yd_media *m,int codec,const unsigned char *src,int w,int h,int stride,int bitrate,int fps,int quality,int gop,yd_media_output *out) {
 if(!m||(codec!=1&&codec!=3))return E_INVALIDARG;
 GUID kind=codec==3?MFVideoFormat_AV1:MFVideoFormat_H264;
 if(m->encodeCodec!=kind){m->transform.reset();m->encodeCodec=kind;m->sequence=0;}
 return yd_media_encode(m,src,w,h,stride,bitrate,fps,quality,gop,out);
}
