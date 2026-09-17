// 以最小 D3D11 替身執行正式 readback.h，能在沒有 Windows／GPU 的主機測試。
#include <cassert>
#include <cstdint>
#include <cstdio>
#include <initializer_list>
using HRESULT=std::int32_t;
using ULONGLONG=std::uint64_t;
constexpr HRESULT S_OK=0, E_FAIL=-1, DXGI_ERROR_WAS_STILL_DRAWING=-2;
constexpr unsigned D3D11_MAP_READ=1, D3D11_MAP_FLAG_DO_NOT_WAIT=0x100000, WAIT_TIMEOUT=258;
#define FAILED(hr) ((hr)<0)
#define HRESULT_FROM_WIN32(code) (static_cast<HRESULT>(0x80070000u|(code)))
ULONGLONG GetTickCount64(){assert(false);return 0;}
void Sleep(unsigned){assert(false);}
struct ID3D11Resource {};
struct D3D11_MAPPED_SUBRESOURCE {void *pData=nullptr;};
struct ID3D11Device {
 HRESULT status=S_OK;
 HRESULT GetDeviceRemovedReason(){return status;}
};
struct ID3D11DeviceContext {
 unsigned calls=0,flushes=0,busy=0;
 HRESULT result=S_OK;
 HRESULT Map(ID3D11Resource *,unsigned sub,unsigned mode,unsigned flags,D3D11_MAPPED_SUBRESOURCE *mapped){
  assert(sub==0 && mode==D3D11_MAP_READ && flags==D3D11_MAP_FLAG_DO_NOT_WAIT);
  if(calls++<busy)return DXGI_ERROR_WAS_STILL_DRAWING;
  if(result==S_OK)mapped->pData=this;
  return result;
 }
 void Flush(){++flushes;}
};
#include "../readback.h"
struct Clock {
 ULONGLONG ticks=0;
 ULONGLONG now(){return ticks;}
 void sleep(){++ticks;}
};
int main(){
 ID3D11Device device;
 ID3D11Resource resource;
 for(unsigned busy: {0u,1u,1999u,2001u}){
  ID3D11DeviceContext context;context.busy=busy;
  D3D11_MAPPED_SUBRESOURCE mapped;
  Clock clock;
  auto hr=yd_map_read_with_clock(&device,&context,&resource,&mapped,clock);
  assert(hr==(busy<=1999?S_OK:HRESULT_FROM_WIN32(WAIT_TIMEOUT)));
  assert(clock.ticks<=2000 && context.calls<=2001);
  assert(context.flushes==(busy?1u:0u));
  assert((mapped.pData!=nullptr)==(hr==S_OK));
 }
 // 裝置移除與 Map 自身的錯誤都應立即傳回，不能重試至逾時。
 for(bool removed: {false,true}){
  ID3D11DeviceContext context;
  context.busy=removed?10000:0;context.result=E_FAIL;
  device.status=removed?E_FAIL:S_OK;
  D3D11_MAPPED_SUBRESOURCE mapped;
  Clock clock;
  assert(yd_map_read_with_clock(&device,&context,&resource,&mapped,clock)==E_FAIL);
  assert(context.calls==1 && context.flushes==0 && clock.ticks==0);
 }
 // 長時間運作後一次 GPU 卡住；超時不留下跨影格的等待狀態。
 device.status=S_OK;
 ID3D11DeviceContext context;
 Clock clock;clock.ticks=1ull<<40;
 for(unsigned frame=0;frame<100000;frame++){
  context.calls=0;context.busy=frame==50000?10000:1;
  D3D11_MAPPED_SUBRESOURCE mapped;
  auto hr=yd_map_read_with_clock(&device,&context,&resource,&mapped,clock);
  assert(hr==(frame==50000?HRESULT_FROM_WIN32(WAIT_TIMEOUT):S_OK));
 }
 std::puts("readback tests passed");
}
