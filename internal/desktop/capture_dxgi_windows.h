#ifndef YD_CAPTURE_DXGI_H
#define YD_CAPTURE_DXGI_H
#ifdef __cplusplus
extern "C" {
#endif
typedef struct yd_dxgi_capture yd_dxgi_capture;
int yd_dxgi_open(int x,int y,int w,int h,yd_dxgi_capture **out);
int yd_dxgi_read(yd_dxgi_capture *,unsigned char *rgba,int w,int h);
void yd_dxgi_close(yd_dxgi_capture *);
#ifdef __cplusplus
}
#endif
#endif
