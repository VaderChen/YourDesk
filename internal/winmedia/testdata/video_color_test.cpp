// 執行正式 metadata→D3D 色彩對應及 CPU 像素轉換，不需要 Windows／GPU。
#include <cassert>
#include <cstdint>
#include <cstdlib>
#include <cstdio>
#include <initializer_list>
using HRESULT=std::int32_t;
using UINT32=std::uint32_t;
constexpr HRESULT S_OK=0,E_INVALIDARG=-1,E_NOTIMPL=-2,MF_E_ATTRIBUTENOTFOUND=-3;
#define FAILED(hr) ((hr)<0)
enum {MFVideoTransferMatrix_Unknown=0,MFVideoTransferMatrix_BT709=1,MFVideoTransferMatrix_BT601=2};
enum {MFNominalRange_Unknown=0,MFNominalRange_0_255=1,MFNominalRange_16_235=2};
enum {D3D11_VIDEO_PROCESSOR_NOMINAL_RANGE_16_235=1,D3D11_VIDEO_PROCESSOR_NOMINAL_RANGE_0_255=2};
enum {MF_MT_YUV_MATRIX=1,MF_MT_VIDEO_NOMINAL_RANGE=2};
struct D3D11_VIDEO_PROCESSOR_COLOR_SPACE {
 unsigned Usage:1,RGB_Range:1,YCbCr_Matrix:1,YCbCr_xvYCC:1,Nominal_Range:2,Reserved:26;
};
struct IMFMediaType {
 UINT32 matrix=MFVideoTransferMatrix_Unknown,range=MFNominalRange_Unknown;
 bool hasMatrix=true,hasRange=true;
 HRESULT error=S_OK;
 HRESULT GetUINT32(int key,UINT32 *out) {
  if(FAILED(error))return error;
  if(key==MF_MT_YUV_MATRIX){if(!hasMatrix)return MF_E_ATTRIBUTENOTFOUND;*out=matrix;return S_OK;}
  assert(key==MF_MT_VIDEO_NOMINAL_RANGE);
  if(!hasRange)return MF_E_ATTRIBUTENOTFOUND;*out=range;return S_OK;
 }
};
#include "../video_color.h"
static void near(const unsigned char *p,int r,int g,int b){
 assert(std::abs((int)p[0]-r)<=2 && std::abs((int)p[1]-g)<=2 && std::abs((int)p[2]-b)<=2 && p[3]==255);
}
int main(){
 IMFMediaType type;
 D3D11_VIDEO_PROCESSOR_COLOR_SPACE color{};
 for(unsigned matrix: {1u,2u})for(unsigned range: {1u,2u}){
  type.matrix=matrix;type.range=range;
  assert(yd_media_yuv_color(&type,&color)==S_OK);
  assert(color.YCbCr_Matrix==(matrix==MFVideoTransferMatrix_BT709?1u:0u));
  assert(color.Nominal_Range==(range==MFNominalRange_0_255?2u:1u));
  assert(color.RGB_Range==0 && color.Reserved==0);
  unsigned char pixel[4];
  bool full=range==MFNominalRange_0_255;
  yd_yuv_to_rgba(full?0:16,128,128,color,pixel);near(pixel,0,0,0);
  yd_yuv_to_rgba(full?255:235,128,128,color,pixel);near(pixel,255,255,255);
  // 彩色向量：只測灰階抓不到 601/709 矩陣混用。
  if(matrix==MFVideoTransferMatrix_BT709){
   yd_yuv_to_rgba(full?54:63,full?99:102,full?255:240,color,pixel);near(pixel,255,0,0);
   yd_yuv_to_rgba(full?182:173,full?30:42,full?12:26,color,pixel);near(pixel,0,255,0);
  }else{
   yd_yuv_to_rgba(full?76:81,full?85:90,full?255:240,color,pixel);near(pixel,255,0,0);
  }
 }
 type.hasMatrix=type.hasRange=false;
 assert(yd_media_yuv_color(&type,&color)==S_OK && color.YCbCr_Matrix==1 && color.Nominal_Range==1);
 type.hasMatrix=type.hasRange=true;type.matrix=type.range=0;
 assert(yd_media_yuv_color(&type,&color)==S_OK && color.YCbCr_Matrix==1 && color.Nominal_Range==1);
 type.matrix=4;type.range=2;assert(yd_media_yuv_color(&type,&color)==E_NOTIMPL);
 type.matrix=1;type.range=3;assert(yd_media_yuv_color(&type,&color)==E_NOTIMPL);
 type.error=E_INVALIDARG;assert(yd_media_yuv_color(&type,&color)==E_INVALIDARG);
 assert(yd_media_yuv_color(nullptr,&color)==E_INVALIDARG);
 std::puts("video color metadata and pixel tests passed");
}
