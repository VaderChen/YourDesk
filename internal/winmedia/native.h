#ifndef YD_WINMEDIA_H
#define YD_WINMEDIA_H
#include <stddef.h>
#ifdef __cplusplus
extern "C" {
#endif
typedef struct yd_media yd_media;
typedef struct { unsigned char *data,*config;size_t size,config_size;int width,height; } yd_media_output;
typedef struct {
 int encode_status,decode_status,encode_ok,decode_ok,hardware_encoder,hardware_decoder;
 int decoder_attempted,input_range,output_range;
 int encoder_d3d11,decoder_d3d11;
 char encoder[512],decoder[512];
} yd_media_probe_result;
typedef struct {
 char name[512];
 int d3d11_status,d3d12_status;
 unsigned int d3d11_level,d3d12_level;
} yd_graphics_probe_result;
int yd_graphics_probe(unsigned int index,yd_graphics_probe_result *);
int yd_media_probe_software(int codec,int format,int width,int height,yd_media_probe_result *);
int yd_media_probe_decode(int,int,int,int,const unsigned char *,size_t,yd_media_probe_result *);
int yd_media_probe(int codec,int format,int width,int height,yd_media_probe_result *);
int yd_media_open(int mode,yd_media **out);
int yd_media_encode_format(yd_media *,int,const unsigned char *,int,int,int,int,int,int,int,yd_media_output *);
int yd_media_encode(yd_media *,const unsigned char *,int,int,int,int,int,int,int,yd_media_output *);
int yd_media_decode_format(yd_media *,int,const unsigned char *,size_t,int,int,yd_media_output *);
int yd_media_decoder_software(yd_media *);
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
