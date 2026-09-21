//go:build cgo && (darwin || windows)

package clientui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Opt-in desktop regression test. It compiles the actual close hook into a
// standalone hidden-window fixture, without a remote peer or download files.
func TestNativeFilesCloseHook(t *testing.T) {
	if os.Getenv("YOURDESK_NATIVE_CLOSE_TESTS") != "1" {
		t.Skip("需設定 YOURDESK_NATIVE_CLOSE_TESTS=1 測試原生關窗攔截")
	}
	compiler := os.Getenv("CC")
	if compiler == "" {
		compiler = "cc"
	}
	if _, err := exec.LookPath(compiler); err != nil {
		t.Fatalf("native test compiler: %v", err)
	}
	dir := t.TempDir()
	source, fixture, suffix := "files_close_darwin.m", nativeFilesCloseDarwinFixture, ".m"
	libs := []string{"-framework", "Cocoa"}
	if runtime.GOOS == "windows" {
		source, fixture, suffix = "files_close_windows.c", nativeFilesCloseWindowsFixture, ".c"
		libs = []string{"-lcomctl32", "-luser32"}
	}
	input, binary := filepath.Join(dir, "close-fixture"+suffix), filepath.Join(dir, "close-fixture")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if err := os.WriteFile(input, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := append([]string{source, input, "-o", binary}, libs...)
	if output, err := exec.CommandContext(ctx, compiler, args...).CombinedOutput(); err != nil {
		t.Fatalf("compile native close fixture: %v\n%s", err, output)
	}
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("native close fixture: %v\n%s", err, output)
	}
}

const nativeFilesCloseDarwinFixture = `
#import <Cocoa/Cocoa.h>
#include <stdint.h>
#include <stdio.h>
extern void *yd_files_close_install(void *, uintptr_t);
extern void yd_files_close_remove(void *);
static int requested, shouldClose, willClose;
void ydFilesRequestClose(uintptr_t handle) { if (handle == 23) requested++; }
@interface FixtureDelegate : NSObject <NSWindowDelegate>
@end
@implementation FixtureDelegate
- (BOOL)windowShouldClose:(NSWindow *)window { shouldClose++; return YES; }
- (void)windowWillClose:(NSNotification *)notice { willClose++; }
- (NSSize)windowWillResize:(NSWindow *)window toSize:(NSSize)size { return NSMakeSize(333, 222); }
@end
#define CHECK(test) do { if (!(test)) { fprintf(stderr, "failed line %d: %s\n", __LINE__, #test); return 1; } } while (0)
static NSWindow *makeWindow(id delegate) {
    NSWindow *window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0,0,200,100)
        styleMask:NSWindowStyleMaskTitled|NSWindowStyleMaskClosable backing:NSBackingStoreBuffered defer:NO];
    window.releasedWhenClosed = NO;
    window.delegate = delegate;
    return window;
}
int main(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        FixtureDelegate *previous = [[FixtureDelegate alloc] init];
        NSWindow *window = makeWindow(previous);
        CHECK(yd_files_close_install(NULL, 23) == NULL);
        void *guard = yd_files_close_install(window, 23);
        CHECK(guard != NULL && window.delegate != previous);
        CHECK([window.delegate respondsToSelector:@selector(windowWillResize:toSize:)]);
        NSSize forwarded = [window.delegate windowWillResize:window toSize:NSMakeSize(1,1)];
        CHECK(forwarded.width == 333 && forwarded.height == 222);
        [window performClose:nil];
        [window performClose:nil];
        CHECK(requested == 2 && shouldClose == 0 && willClose == 0);
        yd_files_close_remove(guard);
        CHECK(window.delegate == previous);
        [window performClose:nil];
        CHECK(requested == 2 && shouldClose == 1 && willClose == 1);
        [window release];
        window = makeWindow(previous);
        guard = yd_files_close_install(window, 23);
        CHECK(guard != NULL);
        // Parent/process teardown uses direct close, not the intercepted gesture.
        [window close];
        CHECK(requested == 2 && willClose == 2);
        yd_files_close_remove(guard);
        CHECK(window.delegate == previous);
        [window release];
        [previous release];
    }
    return 0;
}
`

const nativeFilesCloseWindowsFixture = `
#include <windows.h>
#include <stdint.h>
#include <stdio.h>
extern void *yd_files_close_install(void *, uintptr_t);
extern void yd_files_close_remove(void *);
static int requested, closed;
void ydFilesRequestClose(uintptr_t handle) { if (handle == 23) requested++; }
static LRESULT CALLBACK fixtureProc(HWND window, UINT message, WPARAM wp, LPARAM lp) {
    if (message == WM_CLOSE) { closed++; return 0; }
    return DefWindowProc(window, message, wp, lp);
}
#define CHECK(test) do { if (!(test)) { fprintf(stderr, "failed line %d: %s\n", __LINE__, #test); return 1; } } while (0)
int main(void) {
    WNDCLASSW cls = {0};
    cls.lpfnWndProc = fixtureProc;
    cls.hInstance = GetModuleHandle(NULL);
    cls.lpszClassName = L"YourDeskCloseFixture";
    CHECK(RegisterClassW(&cls) != 0);
    HWND window = CreateWindowW(cls.lpszClassName, L"", WS_OVERLAPPEDWINDOW, 0,0,100,100,NULL,NULL,cls.hInstance,NULL);
    CHECK(window != NULL && yd_files_close_install(NULL, 23) == NULL);
    void *guard = yd_files_close_install(window, 23);
    CHECK(guard != NULL);
    SendMessage(window, WM_CLOSE, 0, 0);
    SendMessage(window, WM_CLOSE, 0, 0);
    CHECK(requested == 2 && closed == 0 && IsWindow(window));
    yd_files_close_remove(guard);
    SendMessage(window, WM_CLOSE, 0, 0);
    CHECK(requested == 2 && closed == 1 && IsWindow(window));
    guard = yd_files_close_install(window, 23);
    CHECK(guard != NULL);
    DestroyWindow(window);
    yd_files_close_remove(guard);
    CHECK(requested == 2 && closed == 1 && !IsWindow(window));
    UnregisterClassW(cls.lpszClassName, cls.hInstance);
    return 0;
}
`
