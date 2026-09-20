#ifndef YOURDESK_VIDEO_COLOR_H
#define YOURDESK_VIDEO_COLOR_H

// MF 的 range 列舉數值與 D3D11 相反，必須逐項對應，不能直接轉型。
// 未宣告矩陣依 MF 規格使用 BT.709；未宣告 NV12 range 使用 studio range。
static HRESULT yd_media_yuv_color(IMFMediaType *type,D3D11_VIDEO_PROCESSOR_COLOR_SPACE *out) {
 if(!type || !out)return E_INVALIDARG;
 UINT32 matrix=MFVideoTransferMatrix_Unknown,range=MFNominalRange_Unknown;
 HRESULT hr=type->GetUINT32(MF_MT_YUV_MATRIX,&matrix);
 if(FAILED(hr) && hr!=MF_E_ATTRIBUTENOTFOUND)return hr;
 hr=type->GetUINT32(MF_MT_VIDEO_NOMINAL_RANGE,&range);
 if(FAILED(hr) && hr!=MF_E_ATTRIBUTENOTFOUND)return hr;
 D3D11_VIDEO_PROCESSOR_COLOR_SPACE color{};
 switch(matrix){
  case MFVideoTransferMatrix_Unknown:case MFVideoTransferMatrix_BT709:color.YCbCr_Matrix=1;break;
  case MFVideoTransferMatrix_BT601:color.YCbCr_Matrix=0;break;
  default:return E_NOTIMPL; // 舊式 D3D11 色彩 API 無法表達 BT.2020 等矩陣，交給 FFmpeg。
 }
 switch(range){
  case MFNominalRange_Unknown:case MFNominalRange_16_235:color.Nominal_Range=D3D11_VIDEO_PROCESSOR_NOMINAL_RANGE_16_235;break;
  case MFNominalRange_0_255:color.Nominal_Range=D3D11_VIDEO_PROCESSOR_NOMINAL_RANGE_0_255;break;
  default:return E_NOTIMPL;
 }
 *out=color;return S_OK;
}

static unsigned char yd_color_clamp(int value){return (unsigned char)(value<0?0:value>255?255:value);}
// 原生 CPU 備援與 GPU 使用同一份已驗證的矩陣／range 決策。
static void yd_yuv_to_rgba(int y,int u,int v,const D3D11_VIDEO_PROCESSOR_COLOR_SPACE &color,unsigned char *out) {
 bool full=color.Nominal_Range==D3D11_VIDEO_PROCESSOR_NOMINAL_RANGE_0_255,bt709=color.YCbCr_Matrix==1;
 u-=128;v-=128;
 int c=full?256*y:298*(y-16);
 out[0]=yd_color_clamp((c+(full?(bt709?403:359):(bt709?459:409))*v+128)>>8);
 out[1]=yd_color_clamp((c-(full?(bt709?48:88):(bt709?55:100))*u-(full?(bt709?120:183):(bt709?136:208))*v+128)>>8);
 out[2]=yd_color_clamp((c+(full?(bt709?475:454):(bt709?541:516))*u+128)>>8);
 out[3]=255;
}
#endif
