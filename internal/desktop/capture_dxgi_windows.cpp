//go:build windows && cgo

#include "capture_dxgi_windows.h"
#include <windows.h>
#include <d3d11.h>
#include <dxgi1_2.h>
#include <wrl/client.h>
#include <memory>
#include <new>
using Microsoft::WRL::ComPtr;
#define CAP_CHECK(expr) do { HRESULT status=(expr); if(FAILED(status)) return status; } while(0)

struct yd_dxgi_capture {
 ComPtr<ID3D11Device> device;
 ComPtr<ID3D11DeviceContext> context;
 ComPtr<IDXGIOutputDuplication> duplication;
 ComPtr<ID3D11Texture2D> staging;
 int width=0,height=0;
 bool captured=false;
 // 此工作階段由 Go 的固定 OS 執行緒持有，不跨執行緒使用 immediate context。
 HRESULT open(int x,int y,int w,int h){
  ComPtr<IDXGIFactory1> factory;CAP_CHECK(CreateDXGIFactory1(IID_PPV_ARGS(&factory)));
  for(UINT a=0;;a++){
   ComPtr<IDXGIAdapter1> adapter;HRESULT hr=factory->EnumAdapters1(a,&adapter);
   if(hr==DXGI_ERROR_NOT_FOUND)break;CAP_CHECK(hr);
   for(UINT o=0;;o++){
    ComPtr<IDXGIOutput> output;hr=adapter->EnumOutputs(o,&output);
    if(hr==DXGI_ERROR_NOT_FOUND)break;CAP_CHECK(hr);
    DXGI_OUTPUT_DESC desc{};CAP_CHECK(output->GetDesc(&desc));
    RECT r=desc.DesktopCoordinates;
    if(!desc.AttachedToDesktop || r.left!=x || r.top!=y || r.right-r.left!=w || r.bottom-r.top!=h)continue;
    // 旋轉畫面先使用 GDI，避免未實機驗證的旋轉座標造成錯向或裁切。
    if(desc.Rotation!=DXGI_MODE_ROTATION_IDENTITY && desc.Rotation!=DXGI_MODE_ROTATION_UNSPECIFIED)return DXGI_ERROR_UNSUPPORTED;
    D3D_FEATURE_LEVEL level;
    CAP_CHECK(D3D11CreateDevice(adapter.Get(),D3D_DRIVER_TYPE_UNKNOWN,nullptr,D3D11_CREATE_DEVICE_BGRA_SUPPORT,nullptr,0,D3D11_SDK_VERSION,&device,&level,&context));
    ComPtr<IDXGIOutput1> output1;CAP_CHECK(output.As(&output1));
    CAP_CHECK(output1->DuplicateOutput(device.Get(),&duplication));
    width=w;height=h;return S_OK;
   }
  }
  return DXGI_ERROR_NOT_FOUND;
 }
 HRESULT read(unsigned char *rgba,int w,int h){
  if(!rgba || w!=width || h!=height)return E_INVALIDARG;
  ComPtr<IDXGIResource> resource;DXGI_OUTDUPL_FRAME_INFO info{};
  HRESULT hr=duplication->AcquireNextFrame(16,&info,&resource);
  if(hr==DXGI_ERROR_WAIT_TIMEOUT)return S_FALSE;
  CAP_CHECK(hr);
  struct Release {IDXGIOutputDuplication *value;~Release(){value->ReleaseFrame();}} release{duplication.Get()};
  // 只移動硬體游標不算新桌面，不重送舊幀墊高 FPS。
  if(captured && info.LastPresentTime.QuadPart==0)return S_FALSE;
  ComPtr<ID3D11Texture2D> texture;CAP_CHECK(resource.As(&texture));
  D3D11_TEXTURE2D_DESC desc{};texture->GetDesc(&desc);
  if(desc.Width!=(UINT)w || desc.Height!=(UINT)h || desc.Format!=DXGI_FORMAT_B8G8R8A8_UNORM || desc.SampleDesc.Count!=1)return DXGI_ERROR_UNSUPPORTED;
  if(!staging){
   desc.Usage=D3D11_USAGE_STAGING;desc.BindFlags=0;desc.CPUAccessFlags=D3D11_CPU_ACCESS_READ;desc.MiscFlags=0;
   CAP_CHECK(device->CreateTexture2D(&desc,nullptr,&staging));
  }
  context->CopyResource(staging.Get(),texture.Get());
  D3D11_MAPPED_SUBRESOURCE mapped{};
  CAP_CHECK(context->Map(staging.Get(),0,D3D11_MAP_READ,0,&mapped));
  struct Unmap {ID3D11DeviceContext *context;ID3D11Resource *resource;~Unmap(){context->Unmap(resource,0);}} unmap{context.Get(),staging.Get()};
  if(!mapped.pData || mapped.RowPitch<(UINT)w*4)return E_FAIL;
  for(int y=0;y<h;y++){
   auto src=static_cast<const unsigned char*>(mapped.pData)+(size_t)y*mapped.RowPitch;
   auto dst=rgba+(size_t)y*w*4;
   for(int x=0;x<w;x++){dst[x*4]=src[x*4+2];dst[x*4+1]=src[x*4+1];dst[x*4+2]=src[x*4];dst[x*4+3]=255;}
  }
  captured=true;return S_OK;
 }
};
extern "C" int yd_dxgi_open(int x,int y,int w,int h,yd_dxgi_capture **out)try {
 if(!out)return E_INVALIDARG;*out=nullptr;
 if(w<1 || h<1 || w>8192 || h>8192 || (size_t)w*h>32u*1024u*1024u)return E_INVALIDARG;
 auto value=std::make_unique<yd_dxgi_capture>();CAP_CHECK(value->open(x,y,w,h));*out=value.release();return S_OK;
}catch(const std::bad_alloc&){return E_OUTOFMEMORY;}catch(...){return E_FAIL;}
extern "C" int yd_dxgi_read(yd_dxgi_capture *value,unsigned char *rgba,int w,int h)try {
 if(!value)return E_INVALIDARG;return value->read(rgba,w,h);
}catch(const std::bad_alloc&){return E_OUTOFMEMORY;}catch(...){return E_FAIL;}
extern "C" void yd_dxgi_close(yd_dxgi_capture *value){delete value;}
