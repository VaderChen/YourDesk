#ifndef YD_WINMEDIA_H
#define YD_WINMEDIA_H
#include <stddef.h>
#ifdef __cplusplus
extern "C" {
#endif
typedef struct yd_media yd_media;
typedef struct { unsigned char *data,*config;size_t size,config_size;int width,height; } yd_media_output;
int yd_media_open(int mode,yd_media **out);
int yd_media_encode(yd_media *,const unsigned char *,int,int,int,int,int,int,int,yd_media_output *);
int yd_media_decode(yd_media *,const unsigned char *,size_t,int,int,yd_media_output *);
int yd_media_scale(yd_media *,const unsigned char *,int,int,int,unsigned char *,int,int,int);
const char *yd_media_error(void);
const char *yd_media_backend(yd_media *);
void yd_media_free(yd_media_output *);
void yd_media_close(yd_media *);
#ifdef __cplusplus
}
#endif
#endif
