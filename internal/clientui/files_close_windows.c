//go:build windows && cgo

#include <windows.h>
#include <commctrl.h>
#include <stdint.h>
#include <stdlib.h>
extern void ydFilesRequestClose(uintptr_t handle);

typedef struct {
    HWND window;
    DWORD thread;
    uintptr_t callback;
} YDFilesCloseGuard;

static LRESULT CALLBACK yd_files_close_proc(HWND window, UINT message, WPARAM wParam, LPARAM lParam, UINT_PTR id, DWORD_PTR data) {
    YDFilesCloseGuard *guard = (YDFilesCloseGuard *)data;
    if (message == WM_CLOSE) {
        if (guard->callback) ydFilesRequestClose(guard->callback);
        return 0;
    }
    if (message == WM_NCDESTROY) {
        RemoveWindowSubclass(window, yd_files_close_proc, id);
        guard->window = NULL;
    }
    return DefSubclassProc(window, message, wParam, lParam);
}

void *yd_files_close_install(void *nativeWindow, uintptr_t callback) {
    HWND window = (HWND)nativeWindow;
    if (!IsWindow(window) || !callback || GetWindowThreadProcessId(window, NULL) != GetCurrentThreadId()) return NULL;
    YDFilesCloseGuard *guard = (YDFilesCloseGuard *)calloc(1, sizeof(*guard));
    if (!guard) return NULL;
    guard->window = window;
    guard->thread = GetCurrentThreadId();
    guard->callback = callback;
    if (!SetWindowSubclass(window, yd_files_close_proc, (UINT_PTR)guard, (DWORD_PTR)guard)) {
        free(guard);
        return NULL;
    }
    return guard;
}

void yd_files_close_remove(void *nativeGuard) {
    YDFilesCloseGuard *guard = (YDFilesCloseGuard *)nativeGuard;
    if (!guard || guard->thread != GetCurrentThreadId()) return;
    guard->callback = 0;
    if (guard->window) RemoveWindowSubclass(guard->window, yd_files_close_proc, (UINT_PTR)guard);
    free(guard);
}
