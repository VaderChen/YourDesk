//go:build darwin && cgo

package prelogin

/*
#cgo LDFLAGS: -framework ApplicationServices -framework Security
#include <ApplicationServices/ApplicationServices.h>
#include <Security/AuthSession.h>
#include <libproc.h>
#include <stdlib.h>
#include <stdint.h>
// 必須同時位於圖形工作階段及目前主控台；UID 相同不代表同一個桌面。
static int yd_active_session(uint32_t *identifier) {
 SecuritySessionId session;
 SessionAttributeBits attributes;
 if (SessionGetInfo(callerSecuritySession, &session, &attributes) != errSecSuccess ||
     !(attributes & sessionHasGraphicAccess)) return 0;
 CFDictionaryRef info = CGSessionCopyCurrentDictionary();
 if (!info) return 0;
 CFTypeRef value = CFDictionaryGetValue(info, kCGSessionOnConsoleKey);
 int active = value && CFGetTypeID(value) == CFBooleanGetTypeID() && CFBooleanGetValue(value);
 CFRelease(info);
 if (active) *identifier = (uint32_t)session;
 return active;
}
static int yd_screen_allowed(void) { return CGPreflightScreenCaptureAccess(); }
static int yd_input_allowed(void) { return AXIsProcessTrusted(); }
static uint64_t yd_started(int pid) {
 struct proc_bsdinfo info;
 if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, sizeof(info)) != sizeof(info)) return 0;
 return (uint64_t)info.pbi_start_tvsec * 1000000 + info.pbi_start_tvusec;
}
static int yd_parent(int pid) {
 struct proc_bsdinfo info;
 if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, sizeof(info)) != sizeof(info)) return -1;
 return info.pbi_ppid;
}
static int yd_process_path(int pid, char *buffer) { return proc_pidpath(pid, buffer, PROC_PIDPATHINFO_MAXSIZE); }
*/
import "C"
import "unsafe"

func processPath(pid int) string {
	b := make([]byte, 4096)
	if C.yd_process_path(C.int(pid), (*C.char)(unsafe.Pointer(&b[0]))) <= 0 {
		return ""
	}
	return C.GoString((*C.char)(unsafe.Pointer(&b[0])))
}

func processParent(pid int) int { return int(C.yd_parent(C.int(pid))) }

func processStarted(pid int) uint64 { return uint64(C.yd_started(C.int(pid))) }

// activeSession 不要求已登入的使用者，允許 LoginWindow 的 root 圖形工作階段。
func activeSession() (uint32, bool) {
	var identifier C.uint32_t
	ok := C.yd_active_session(&identifier) != 0
	return uint32(identifier), ok
}

// 只查詢權限，不在登入畫面觸發無人可回應的授權提示。
func captureAllowed() bool { return C.yd_screen_allowed() != 0 }
func inputAllowed() bool   { return C.yd_input_allowed() != 0 }
