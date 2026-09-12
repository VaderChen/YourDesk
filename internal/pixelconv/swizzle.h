#ifndef YOURDESK_PIXEL_SWIZZLE_H
#define YOURDESK_PIXEL_SWIZZLE_H
#include <stddef.h>
#if defined(__aarch64__) || defined(_M_ARM64)
#include <arm_neon.h>
#elif defined(__SSE2__)
#include <emmintrin.h>
#endif

// RGBA ↔ BGRA，alpha 固定 255；只使用架構基線指令，允許非對齊列。
static inline void yd_swap_rb_opaque(unsigned char *dst,const unsigned char *src,size_t width) {
 size_t x=0;
#if defined(__aarch64__) || defined(_M_ARM64)
 for(;x+16<=width;x+=16){
  uint8x16x4_t p=vld4q_u8(src+x*4),q;
  q.val[0]=p.val[2];q.val[1]=p.val[1];q.val[2]=p.val[0];q.val[3]=vdupq_n_u8(255);
  vst4q_u8(dst+x*4,q);
 }
#elif defined(__SSE2__)
 const __m128i rb=_mm_set1_epi32(0x00ff00ff),green=_mm_set1_epi32(0x0000ff00),alpha=_mm_set1_epi32((int)0xff000000u);
 for(;x+4<=width;x+=4){
  __m128i p=_mm_loadu_si128((const __m128i *)(src+x*4));
  __m128i channels=_mm_and_si128(p,rb);
  __m128i q=_mm_or_si128(_mm_slli_epi32(channels,16),_mm_srli_epi32(channels,16));
  q=_mm_or_si128(_mm_or_si128(q,_mm_and_si128(p,green)),alpha);
  _mm_storeu_si128((__m128i *)(dst+x*4),q);
 }
#endif
 for(;x<width;x++){unsigned char r=src[x*4],g=src[x*4+1],b=src[x*4+2];dst[x*4]=b;dst[x*4+1]=g;dst[x*4+2]=r;dst[x*4+3]=255;}
}
#endif
