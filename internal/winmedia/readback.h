#pragma once

// 呼叫端先載入 Windows／D3D11 標頭。測試可注入裝置、context 與時鐘，
// 不需要真的讓顯示卡停住。僅等待 WAS_STILL_DRAWING，其餘結果直接返回。
template<class Device, class Context, class Resource, class Mapping, class Clock>
HRESULT yd_map_read_with_clock(Device *device, Context *context, Resource *resource,
                               Mapping *mapped, Clock &clock) {
 const auto started=clock.now();
 bool flushed=false;
 for(;;){
  HRESULT hr=context->Map(resource,0,D3D11_MAP_READ,D3D11_MAP_FLAG_DO_NOT_WAIT,mapped);
  if(hr!=DXGI_ERROR_WAS_STILL_DRAWING)return hr;
  hr=device->GetDeviceRemovedReason();
  if(FAILED(hr))return hr;
  if(clock.now()-started>=2000)return HRESULT_FROM_WIN32(WAIT_TIMEOUT);
  // DO_NOT_WAIT 不保證送出排隊中的 Copy；只在首次忙碌時 Flush。
  if(!flushed){context->Flush();flushed=true;}
  clock.sleep();
 }
}

struct yd_readback_clock {
 ULONGLONG now(){return GetTickCount64();}
 void sleep(){Sleep(1);}
};

inline HRESULT yd_map_read(ID3D11Device *device, ID3D11DeviceContext *context,
                            ID3D11Resource *resource, D3D11_MAPPED_SUBRESOURCE *mapped) {
 yd_readback_clock clock;
 return yd_map_read_with_clock(device,context,resource,mapped,clock);
}
