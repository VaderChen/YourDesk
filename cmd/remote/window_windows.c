//go:build windows && cgo

#include "window_windows.h"
#include <math.h>
#include <objbase.h>
#include <stdlib.h>

// 全部狀態由 WebView 的專用 UI 執行緒存取，不進入影像解碼執行緒。
static HWND viewer, chrome;
static LONG_PTR originalStyle, normalStyle;
static WINDOWPLACEMENT placement;
static int active, fullscreen, shown=1, popupOpen, previousWidth, previousHeight, comReady;
static double popupX,popupY,popupW,popupH;
static ULONGLONG edgeSince,leaveSince;
static int previousDown;
static RECT lastRegion;
static int regionDirty;
static int nativeMenuOpen;
static BOOL CALLBACK findViewer(HWND hwnd,LPARAM param) {
 DWORD pid=0; WCHAR name[64];
 GetWindowThreadProcessId(hwnd,&pid);
 if(pid==GetCurrentProcessId() && IsWindowVisible(hwnd) && GetClassNameW(hwnd,name,64) && lstrcmpW(name,L"GLFW30")==0) {
  *(HWND*)param=hwnd; return FALSE;
 }
 return TRUE;
}
void *yd_win_find_viewer(void) { HWND hwnd=NULL; EnumWindows(findViewer,(LPARAM)&hwnd);return hwnd; }
static double scale(void) {
 typedef UINT (WINAPI *DpiProc)(HWND);
 DpiProc dpi=(DpiProc)GetProcAddress(GetModuleHandleW(L"user32.dll"),"GetDpiForWindow");
 return dpi ? dpi(viewer)/96.0 : 1.0;
}
// 無裝飾視窗的外框與客戶區一致；縮放由命中測試處理。
// 原視窗程序保存在 HWND 屬性，回呼仍在 遠端顯示 所屬 UI 執行緒執行。
static const WCHAR originalProcKey[]=L"YourDeskOriginalViewerProc";
static LRESULT CALLBACK viewerChromeProc(HWND hwnd,UINT msg,WPARAM wp,LPARAM lp) {
 WNDPROC original=(WNDPROC)GetPropW(hwnd,originalProcKey);
 if(!original) return DefWindowProcW(hwnd,msg,wp,lp);
 if(msg==WM_NCCALCSIZE) {
  // 引擎以無裝飾視窗計算大小，整個外框必須等於客戶區。
  // 只扣除部分邊框會讓每次尺寸回饋再少一圈，造成持續縮小。
  return 0;
 }
 if(msg==WM_GETMINMAXINFO) {
  LRESULT result=CallWindowProcW(original,hwnd,msg,wp,lp);
  MONITORINFO monitor={0};monitor.cbSize=sizeof(monitor);
  if(GetMonitorInfoW(MonitorFromWindow(hwnd,MONITOR_DEFAULTTONEAREST),&monitor)) {
   MINMAXINFO *limits=(MINMAXINFO*)lp;
   limits->ptMaxPosition.x=monitor.rcWork.left-monitor.rcMonitor.left;
   limits->ptMaxPosition.y=monitor.rcWork.top-monitor.rcMonitor.top;
   limits->ptMaxSize.x=monitor.rcWork.right-monitor.rcWork.left;
   limits->ptMaxSize.y=monitor.rcWork.bottom-monitor.rcWork.top;
  }
  return result;
 }
 if(msg==WM_NCHITTEST) {
  LRESULT result=CallWindowProcW(original,hwnd,msg,wp,lp);
  RECT rect;GetWindowRect(hwnd,&rect);
  int x=(short)LOWORD(lp),y=(short)HIWORD(lp);
  if(result==HTCLIENT && !IsZoomed(hwnd) && (GetWindowLongPtrW(hwnd,GWL_STYLE)&WS_THICKFRAME)) {
   int left=x<rect.left+6,right=x>=rect.right-6,top=y<rect.top+6,bottom=y>=rect.bottom-6;
   if(top) return left?HTTOPLEFT:right?HTTOPRIGHT:HTTOP;
   if(bottom) return left?HTBOTTOMLEFT:right?HTBOTTOMRIGHT:HTBOTTOM;
   if(left)return HTLEFT;if(right)return HTRIGHT;
  }
  return result;
 }
 LRESULT result=CallWindowProcW(original,hwnd,msg,wp,lp);
 if(msg==WM_NCDESTROY) RemovePropW(hwnd,originalProcKey);
 return result;
}
static LRESULT CALLBACK chromeProc(HWND hwnd,UINT msg,WPARAM wp,LPARAM lp) {
 if(msg==WM_NCHITTEST && !IsZoomed(viewer) && (GetWindowLongPtrW(viewer,GWL_STYLE)&WS_THICKFRAME)) {
  RECT rect;GetWindowRect(viewer,&rect);
  int y=(short)HIWORD(lp);
  if(y>=rect.top && y<rect.top+4) return HTTRANSPARENT;
 }

 if(msg==WM_SIZE) {
  HWND widget=FindWindowExW(hwnd,NULL,L"webview_widget",NULL);
  RECT r;GetClientRect(hwnd,&r);
  if(widget) MoveWindow(widget,0,0,r.right,r.bottom,TRUE);
 }
 if(msg==WM_ERASEBKGND) return 1;
 return DefWindowProcW(hwnd,msg,wp,lp);
}
void *yd_win_create(void *parent) {
 if(FAILED(CoInitializeEx(NULL,COINIT_APARTMENTTHREADED))) return NULL;
 comReady=1;
 viewer=(HWND)parent;
 WNDCLASSW wc={0};wc.lpfnWndProc=chromeProc;wc.hInstance=GetModuleHandleW(NULL);wc.lpszClassName=L"YourDeskViewerChrome";wc.hCursor=LoadCursorW(NULL,MAKEINTRESOURCEW(32512));
 RegisterClassW(&wc);
 chrome=CreateWindowExW(WS_EX_CONTROLPARENT,wc.lpszClassName,L"",WS_CHILD|WS_CLIPCHILDREN|WS_CLIPSIBLINGS,0,0,1,1,viewer,NULL,wc.hInstance,NULL);
 if(!chrome) {CoUninitialize();comReady=0;}
 return chrome;
}
void yd_win_activate(void) {
 if(active || !IsWindow(viewer)) return;
 WNDPROC original=(WNDPROC)GetWindowLongPtrW(viewer,GWLP_WNDPROC);
 if(SetPropW(viewer,originalProcKey,(HANDLE)original)) {
  SetLastError(0);
  if(!SetWindowLongPtrW(viewer,GWLP_WNDPROC,(LONG_PTR)viewerChromeProc) && GetLastError()!=0) RemovePropW(viewer,originalProcKey);
 }
 originalStyle=GetWindowLongPtrW(viewer,GWL_STYLE);
 normalStyle=(originalStyle & ~WS_CAPTION)|WS_CLIPCHILDREN|WS_THICKFRAME|WS_MAXIMIZEBOX;
 SetWindowLongPtrW(viewer,GWL_STYLE,normalStyle);
 SetWindowPos(viewer,NULL,0,0,0,0,SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED);
 active=1;ShowWindow(chrome,SW_SHOWNOACTIVATE);
}
void yd_win_destroy(void) {
 if(IsWindow(chrome)) DestroyWindow(chrome);
 chrome=NULL;
 if(active && IsWindow(viewer)) {
  if((WNDPROC)GetWindowLongPtrW(viewer,GWLP_WNDPROC)==viewerChromeProc) {
   WNDPROC original=(WNDPROC)GetPropW(viewer,originalProcKey);
   if(original) {SetWindowLongPtrW(viewer,GWLP_WNDPROC,(LONG_PTR)original);RemovePropW(viewer,originalProcKey);}
  }
  SetWindowLongPtrW(viewer,GWL_STYLE,originalStyle);
  SetWindowPos(viewer,NULL,0,0,0,0,SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED);
 }
 active=0;
 if(comReady) {CoUninitialize();comReady=0;}
}
void yd_win_popup(double x,double y,double width,double height) {
 popupX=x;popupY=y;popupW=width;popupH=height;popupOpen=width>0 && height>0;
 // 下次 tick 重算裁切區，選單之外仍可操作遠端畫面。
 regionDirty=1;
}
int yd_win_tick(void) {
 if(!IsWindow(viewer) || !IsWindow(chrome)) return -1;
 if(!active) return 0;
 // 引擎在調整尺寸／DPI 後可能重新套用 style；保留縮放邊框，移除原生標題列。
 LONG_PTR style=GetWindowLongPtrW(viewer,GWL_STYLE);
 LONG_PTR desired=(style & ~WS_CAPTION)|WS_CLIPCHILDREN;
 if(fullscreen) desired &= ~WS_THICKFRAME;
 else desired |= WS_THICKFRAME|WS_MAXIMIZEBOX;
 if(style!=desired) {
  SetWindowLongPtrW(viewer,GWL_STYLE,desired);
  SetWindowPos(viewer,NULL,0,0,0,0,SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED);
 }
 RECT client;GetClientRect(viewer,&client);
 double dpi=scale();int header=(int)ceil(52*dpi);
 int height=min(client.bottom,(int)ceil(600*dpi));
 POINT point;GetCursorPos(&point);ScreenToClient(viewer,&point);
 ULONGLONG now=GetTickCount64();
 int foreground=GetForegroundWindow()==viewer;
 if(fullscreen) {
  if(!shown && foreground && point.x>=0 && point.x<client.right && point.y>=0 && point.y<=3*dpi) {
   if(!edgeSince) edgeSince=now;
   if(now-edgeSince>=1000) shown=1;
  } else if(!shown) edgeSince=0;
  if(shown && !popupOpen && !nativeMenuOpen && (!foreground || point.y>header || point.x<0 || point.x>=client.right)) {
   if(!leaveSince) leaveSince=now;
   if(now-leaveSince>=400) {shown=0;edgeSince=0;}
  } else leaveSince=0;
 } else shown=1;
 if(!!IsWindowVisible(chrome)!=!!shown) ShowWindow(chrome,shown?SW_SHOWNOACTIVATE:SW_HIDE);
 RECT popup={(LONG)floor(popupX*dpi),(LONG)floor(popupY*dpi),(LONG)ceil((popupX+popupW)*dpi),(LONG)ceil((popupY+popupH)*dpi)};
 int resized=previousWidth!=client.right || previousHeight!=height;
 if(resized || regionDirty || !EqualRect(&popup,&lastRegion)) {
  if(resized) SetWindowPos(chrome,HWND_TOP,0,0,client.right,height,SWP_NOACTIVATE);
  HRGN region=CreateRectRgn(0,0,client.right,header);
  if(popupOpen) {HRGN extra=CreateRoundRectRgn(popup.left,popup.top,popup.right,popup.bottom,(int)(24*dpi),(int)(24*dpi));CombineRgn(region,region,extra,RGN_OR);DeleteObject(extra);}
  if(!SetWindowRgn(chrome,region,TRUE)) DeleteObject(region);
  previousWidth=client.right;previousHeight=height;lastRegion=popup;regionDirty=0;
 }
 int down=(GetAsyncKeyState(VK_LBUTTON)&0x8000)!=0;
 int outside=popupOpen && ((!foreground) || (down&&!previousDown && point.y>=header && !PtInRect(&popup,point)));
 previousDown=down;
 return (fullscreen?1:0)|(shown?2:0)|(IsZoomed(viewer)?4:0)|(outside?8:0);
}
void yd_win_command(int action) {
 if(!IsWindow(viewer)) return;
 if(action==7) {ShowWindowAsync(viewer,SW_MINIMIZE);return;}
 if(action==8) {if(!fullscreen) ShowWindowAsync(viewer,IsZoomed(viewer)?SW_RESTORE:SW_MAXIMIZE);return;}
 if(action==9) {ReleaseCapture();POINT p;GetCursorPos(&p);PostMessageW(viewer,WM_NCLBUTTONDOWN,HTCAPTION,MAKELPARAM(p.x,p.y));return;}
 if(action==4) {
  if(!fullscreen) {
   placement.length=sizeof(placement);GetWindowPlacement(viewer,&placement);
   MONITORINFO monitor={0};monitor.cbSize=sizeof(monitor);GetMonitorInfoW(MonitorFromWindow(viewer,MONITOR_DEFAULTTONEAREST),&monitor);
   SetWindowLongPtrW(viewer,GWL_STYLE,(normalStyle & ~(WS_THICKFRAME|WS_MINIMIZE|WS_MAXIMIZE))|WS_POPUP);
   SetWindowPos(viewer,HWND_TOP,monitor.rcMonitor.left,monitor.rcMonitor.top,monitor.rcMonitor.right-monitor.rcMonitor.left,monitor.rcMonitor.bottom-monitor.rcMonitor.top,SWP_FRAMECHANGED|SWP_NOACTIVATE);
   fullscreen=1;shown=0;edgeSince=0;
  } else {
   SetWindowLongPtrW(viewer,GWL_STYLE,normalStyle);SetWindowPlacement(viewer,&placement);
   SetWindowPos(viewer,NULL,0,0,0,0,SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED);
   fullscreen=0;shown=1;
  }
 }
 // 本機按鈕完成後將鍵盤焦點交回 遠端顯示。
 if(action!=9) {
  DWORD target=GetWindowThreadProcessId(viewer,NULL),current=GetCurrentThreadId();
  if(AttachThreadInput(current,target,TRUE)) {SetFocus(viewer);AttachThreadInput(current,target,FALSE);}
 }
}
void yd_win_title(char *output,int length) {
 WCHAR title[1024];GetWindowTextW(viewer,title,1024);WideCharToMultiByte(CP_UTF8,0,title,-1,output,length,NULL,NULL);
}
void yd_win_locale(char *output,int length) {
 WCHAR locale[LOCALE_NAME_MAX_LENGTH];GetUserDefaultLocaleName(locale,LOCALE_NAME_MAX_LENGTH);WideCharToMultiByte(CP_UTF8,0,locale,-1,output,length,NULL,NULL);
}

void *yd_win_menu_create(void) {return CreatePopupMenu();}
void yd_win_menu_add(void *menu,const char *label,int action,int checked) {
 if(!menu) return;
 if(action==0) {AppendMenuW((HMENU)menu,MF_SEPARATOR,0,NULL);return;}
 int length=MultiByteToWideChar(CP_UTF8,0,label,-1,NULL,0);
 WCHAR *text=(WCHAR*)calloc(length,sizeof(WCHAR));
 if(!text) return;
 MultiByteToWideChar(CP_UTF8,0,label,-1,text,length);
 AppendMenuW((HMENU)menu,MF_STRING|(checked?MF_CHECKED:0),action,text);
 free(text);
}
int yd_win_menu_show(void *menu,double x,double y) {
 if(!menu || !IsWindow(chrome)) return 0;
 POINT point={(LONG)round(x*scale()),(LONG)round(y*scale())};ClientToScreen(chrome,&point);
 nativeMenuOpen=1;
 int result=TrackPopupMenuEx((HMENU)menu,TPM_RETURNCMD|TPM_NONOTIFY|TPM_RIGHTALIGN|TPM_TOPALIGN|TPM_RIGHTBUTTON,point.x,point.y,chrome,NULL);
 nativeMenuOpen=0;
 yd_win_command(0);
 return result;
}
void yd_win_menu_destroy(void *menu) {if(menu) DestroyMenu((HMENU)menu);}
