//go:build windows && cgo

#include <windows.h>
#include <shellapi.h>
#include <stdint.h>
#include <stdlib.h>
extern void ydTrayQuit(uintptr_t handle);
extern void ydTrayShowMCP(uintptr_t handle);
extern void ydTrayDisconnectIncoming(uintptr_t handle);
#define YD_DISCONNECT_INCOMING 1004
#define YD_TRAY_MESSAGE (WM_APP + 81)
#define YD_SHOW 1001
#define YD_QUIT 1002
#define YD_MCP 1003
static const wchar_t *YD_PROPERTY = L"YourDeskTrayController";
typedef struct {
    int incomingConnected;
    int mcpCount;
 int mcpVisible;
    HICON normalIcon, mcpIcon;
    HWND window;
    WNDPROC previous;
    NOTIFYICONDATAW icon;
    UINT taskbarCreated;
    uintptr_t callback;
} YDTray;
static void yd_show(YDTray *tray) {
    ShowWindow(tray->window, SW_RESTORE);
    SetForegroundWindow(tray->window);
}
static LRESULT CALLBACK yd_window_proc(HWND window, UINT message, WPARAM wParam, LPARAM lParam) {
    YDTray *tray = (YDTray *)GetPropW(window, YD_PROPERTY);
    if (!tray) return DefWindowProcW(window, message, wParam, lParam);
    if (message == WM_CLOSE) { ShowWindow(window, SW_HIDE); return 0; }
    if (message == tray->taskbarCreated) { Shell_NotifyIconW(NIM_ADD, &tray->icon); return 0; }
    if (message == YD_TRAY_MESSAGE) {
        if (lParam == WM_LBUTTONUP || lParam == WM_LBUTTONDBLCLK) yd_show(tray);
        if (lParam == WM_RBUTTONUP || lParam == WM_CONTEXTMENU) {
            POINT point;
            GetCursorPos(&point);
            HMENU menu = CreatePopupMenu();
            AppendMenuW(menu, MF_STRING, YD_SHOW, L"\u958b\u555f\u4ecb\u9762");
            if(tray->mcpCount>0)AppendMenuW(menu,MF_STRING,YD_MCP,tray->mcpVisible?L"隱藏 MCP 遠端畫面":L"開啟 MCP 遠端畫面");
            if(tray->incomingConnected)AppendMenuW(menu,MF_STRING,YD_DISCONNECT_INCOMING,L"關閉遠端連線");
            AppendMenuW(menu, MF_SEPARATOR, 0, NULL);
            AppendMenuW(menu, MF_STRING, YD_QUIT, L"\u95dc\u9589\u7a0b\u5f0f");
            SetForegroundWindow(window);
            UINT selection = TrackPopupMenu(menu, TPM_RETURNCMD | TPM_RIGHTBUTTON, point.x, point.y, 0, window, NULL);
            DestroyMenu(menu);
            if (selection == YD_SHOW) yd_show(tray);
            if(selection==YD_DISCONNECT_INCOMING)ydTrayDisconnectIncoming(tray->callback);
            if(selection==YD_MCP)ydTrayShowMCP(tray->callback);
            if (selection == YD_QUIT) ydTrayQuit(tray->callback);
            PostMessageW(window, WM_NULL, 0, 0);
        }
        return 0;
    }
    return CallWindowProcW(tray->previous, window, message, wParam, lParam);
}
void *yd_tray_install(void *nativeWindow, uintptr_t handle) {
    YDTray *tray = calloc(1, sizeof(YDTray));
    if (!tray) return NULL;
    tray->window = (HWND)nativeWindow;
    tray->callback = handle;
    tray->taskbarCreated = RegisterWindowMessageW(L"TaskbarCreated");
    if (!SetPropW(tray->window, YD_PROPERTY, tray)) { free(tray); return NULL; }
    tray->previous = (WNDPROC)SetWindowLongPtrW(tray->window, GWLP_WNDPROC, (LONG_PTR)yd_window_proc);
    if (!tray->previous) { RemovePropW(tray->window, YD_PROPERTY); free(tray); return NULL; }
    tray->icon.cbSize = sizeof(NOTIFYICONDATAW);
    tray->icon.hWnd = tray->window;
    tray->icon.uID = 1;
    tray->icon.uFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP;
    tray->icon.uCallbackMessage = YD_TRAY_MESSAGE;
    tray->icon.hIcon = LoadIconW(GetModuleHandleW(NULL), MAKEINTRESOURCEW(1));
    if (!tray->icon.hIcon) tray->icon.hIcon = LoadIconW(NULL, MAKEINTRESOURCEW(32512));
    tray->normalIcon=tray->icon.hIcon;
    SendMessageW(tray->window, WM_SETICON, ICON_BIG, (LPARAM)tray->icon.hIcon);
    SendMessageW(tray->window, WM_SETICON, ICON_SMALL, (LPARAM)tray->icon.hIcon);
    lstrcpynW(tray->icon.szTip, L"YourDesk", 128);
    if (!Shell_NotifyIconW(NIM_ADD, &tray->icon)) {
        SetWindowLongPtrW(tray->window, GWLP_WNDPROC, (LONG_PTR)tray->previous);
        RemovePropW(tray->window, YD_PROPERTY); free(tray); return NULL;
    }
    return tray;
}
void yd_tray_remove(void *value) {
    YDTray *tray = (YDTray *)value;
    Shell_NotifyIconW(NIM_DELETE, &tray->icon);
    SetWindowLongPtrW(tray->window, GWLP_WNDPROC, (LONG_PTR)tray->previous);
    RemovePropW(tray->window, YD_PROPERTY);
    if(tray->mcpIcon)DestroyIcon(tray->mcpIcon);
    free(tray);
}

void yd_connection_notice(void *nativeWindow) {
    HWND window = (HWND)nativeWindow;
    YDTray *tray = (YDTray *)GetPropW(window, YD_PROPERTY);
    if (!tray) return;
    ShowWindow(window, SW_HIDE);
    NOTIFYICONDATAW notice = tray->icon;
    notice.uFlags = NIF_INFO;
    notice.dwInfoFlags = NIIF_INFO;
    lstrcpynW(notice.szInfoTitle, L"YourDesk", 64);
    lstrcpynW(notice.szInfo, L"\u6b63\u5728\u9023\u7dda", 256);
    Shell_NotifyIconW(NIM_MODIFY, &notice);
}

void yd_update_window(void *nativeWindow) {
    HWND window = (HWND)nativeWindow;
    ShowWindow(window, SW_RESTORE);
    SetForegroundWindow(window);
    FLASHWINFO flash = {sizeof(FLASHWINFO), window, FLASHW_TRAY, 3, 0};
    FlashWindowEx(&flash);
}

void yd_transfer_window(void *nativeWindow) {
 HWND window=(HWND)nativeWindow;
 ShowWindow(window, SW_SHOWNOACTIVATE);
 SetWindowPos(window, HWND_TOP, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE);
}

void yd_close_interface(void *window) { PostMessageW((HWND)window, WM_CLOSE, 0, 0); }

static HICON yd_tint_icon(HICON source) {
 BITMAPINFO info={0};info.bmiHeader.biSize=sizeof(BITMAPINFOHEADER);info.bmiHeader.biWidth=32;info.bmiHeader.biHeight=-32;info.bmiHeader.biPlanes=1;info.bmiHeader.biBitCount=32;info.bmiHeader.biCompression=BI_RGB;
 void *pixels=NULL;HDC dc=CreateCompatibleDC(NULL);if(!dc)return NULL;
 HBITMAP color=CreateDIBSection(dc,&info,DIB_RGB_COLORS,&pixels,NULL,0);if(!color){DeleteDC(dc);return NULL;}
 HGDIOBJ old=SelectObject(dc,color);ZeroMemory(pixels,32*32*4);DrawIconEx(dc,0,0,source,32,32,0,NULL,DI_NORMAL);
 DWORD *p=(DWORD*)pixels;for(int i=0;i<32*32;i++){
  DWORD a=p[i]>>24,r=(p[i]>>16)&255,g=(p[i]>>8)&255,b=p[i]&255;
  DWORD high=max(r,max(g,b)),low=min(r,min(g,b));
  double amount=a?min(1.0,(double)(high-low)/(a*0.35)):0;
  r=(DWORD)(r+(a-r)*amount);g=(DWORD)(g+(a*0.584-g)*amount);b=(DWORD)(b*(1-amount));
  p[i]=(a<<24)|(r<<16)|(g<<8)|b;
 }
 SelectObject(dc,old);BYTE maskBits[128]={0};HBITMAP mask=CreateBitmap(32,32,1,1,maskBits);ICONINFO ii={0};ii.fIcon=TRUE;ii.hbmColor=color;ii.hbmMask=mask;
 HICON result=mask?CreateIconIndirect(&ii):NULL;if(mask)DeleteObject(mask);DeleteObject(color);DeleteDC(dc);return result;
}
void yd_mcp_tray(void *nativeWindow,int count,int notify,int visible){
 YDTray *t=(YDTray*)GetPropW((HWND)nativeWindow,YD_PROPERTY);if(!t)return;
 t->mcpCount=count;t->mcpVisible=visible;if(count>0 && !t->mcpIcon)t->mcpIcon=yd_tint_icon(t->normalIcon);
 t->icon.hIcon=count>0 && t->mcpIcon?t->mcpIcon:t->normalIcon;
 lstrcpynW(t->icon.szTip,count>0?L"YourDesk · MCP 正在操作":L"YourDesk",128);
 NOTIFYICONDATAW update=t->icon;update.uFlags=NIF_ICON|NIF_TIP;Shell_NotifyIconW(NIM_MODIFY,&update);
 if(notify && count>0){update.uFlags=NIF_INFO;update.dwInfoFlags=NIIF_INFO;lstrcpynW(update.szInfoTitle,L"YourDesk · MCP 正在操作",64);lstrcpynW(update.szInfo,L"可從 Tray 選單開啟遠端畫面",256);Shell_NotifyIconW(NIM_MODIFY,&update);}
}

void yd_incoming_tray(void *window,int connected){YDTray *t=(YDTray*)GetPropW((HWND)window,YD_PROPERTY);if(t)t->incomingConnected=connected;}
