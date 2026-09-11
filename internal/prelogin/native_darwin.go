//go:build darwin && cgo

package prelogin

/*
#include <libproc.h>
#include <stdlib.h>
#include <stdint.h>
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
