#include <windows.h>
void *yd_win_find_viewer(void);
void *yd_win_create(void *parent);
void yd_win_activate(void);
void yd_win_destroy(void);
int yd_win_tick(void);
void yd_win_popup(double x,double y,double width,double height);
void yd_win_command(int action);
void yd_win_title(char *output,int length);
void yd_win_locale(char *output,int length);

void *yd_win_menu_create(void);
void yd_win_menu_add(void *menu,const char *label,int action,int checked);
int yd_win_menu_show(void *menu,double x,double y);
void yd_win_menu_destroy(void *menu);
