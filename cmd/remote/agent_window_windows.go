//go:build windows && cgo

package main

/*
#cgo LDFLAGS: -luser32
#include <windows.h>
static BOOL CALLBACK yd_agent_show(HWND w,LPARAM p){
 DWORD pid;WCHAR name[64];GetWindowThreadProcessId(w,&pid);
 if(pid==GetCurrentProcessId() && GetClassNameW(w,name,64) && lstrcmpW(name,L"GLFW30")==0){ShowWindowAsync(w,p?SW_HIDE:SW_RESTORE);if(!p)SetForegroundWindow(w);return FALSE;}return TRUE;
}
static void yd_agent_hide_window(void){EnumWindows(yd_agent_show,1);}
static void yd_agent_show_window(void){EnumWindows(yd_agent_show,0);}
*/
import "C"

func nativeHideAgentWindow() { C.yd_agent_hide_window() }
func nativeShowAgentWindow() { C.yd_agent_show_window() }
