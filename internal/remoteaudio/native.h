#ifndef YD_AUDIO_NATIVE_H
#define YD_AUDIO_NATIVE_H
#ifdef __cplusplus
extern "C" {
#endif
void *yd_audio_open(int mode,int codec,int bitrate,int preference,int *hardware,char *error,int length);
int yd_audio_process(void *handle,const void *input,int length,void *output,int capacity);
void yd_audio_close(void *handle);
#ifdef __cplusplus
}
#endif
#endif
