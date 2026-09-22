//go:build cgo && darwin

package clientui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeFilesPicker(t *testing.T) {
	if os.Getenv("YOURDESK_NATIVE_PICKER_TESTS") != "1" {
		t.Skip("需設定 YOURDESK_NATIVE_PICKER_TESTS=1 驗證 macOS 原生選檔視窗")
	}
	dir := t.TempDir()
	source, binary := filepath.Join(dir, "picker.m"), filepath.Join(dir, "picker")
	if err := os.WriteFile(source, []byte(nativeFilesPickerFixture), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "clang", "files_picker_darwin.m", source, "-fblocks", "-framework", "Cocoa", "-framework", "WebKit", "-o", binary).CombinedOutput(); err != nil {
		t.Fatalf("編譯原生選檔 Smoke：%v\n%s", err, output)
	}
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("原生選檔 Smoke：%v\n%s", err, output)
	}
}

const nativeFilesPickerFixture = `
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include <stdio.h>
extern void *yd_files_picker_install(void *);
extern void yd_files_picker_remove(void *);
@interface PickerParameters : NSObject
@property BOOL allowsMultipleSelection;
@property BOOL allowsDirectories;
@end
@implementation PickerParameters
@end
#define CHECK(test) do { if (!(test)) { fprintf(stderr, "failed line %d: %s\n", __LINE__, #test); return 1; } } while (0)
static void pump(void) { [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.02]]; }
int main(void) {
 @autoreleasepool {
  [NSApplication sharedApplication];
  NSWindow *window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0,0,600,400)
   styleMask:NSWindowStyleMaskTitled backing:NSBackingStoreBuffered defer:NO];
  window.releasedWhenClosed=NO;
  WKWebView *view=[[WKWebView alloc] initWithFrame:window.contentView.bounds];
  [window.contentView addSubview:view];
  NSObject *previous=[NSObject new];view.UIDelegate=(id)previous;
  void *guard=yd_files_picker_install(window);CHECK(guard && view.UIDelegate!=(id)previous);
  PickerParameters *parameters=[PickerParameters new];parameters.allowsMultipleSelection=YES;
  __block int completed=0;__block BOOL cancelled=NO;
  [view.UIDelegate webView:view runOpenPanelWithParameters:(id)parameters initiatedByFrame:nil
   completionHandler:^(NSArray *urls){completed++;cancelled=urls==nil;}];
  for(int i=0;i<100&&!window.attachedSheet;i++)pump();
  NSOpenPanel *panel=(id)window.attachedSheet;
  CHECK([panel isKindOfClass:[NSOpenPanel class]] && panel.allowsMultipleSelection && panel.canChooseFiles && !panel.canChooseDirectories);
  CHECK(completed==0);
  [panel cancel:nil];
  for(int i=0;i<100&&completed==0;i++)pump();
  CHECK(completed==1 && cancelled);
  // 取消後可再次開啟；視窗清理也會取消仍開著的選檔 Sheet。
  [view.UIDelegate webView:view runOpenPanelWithParameters:(id)parameters initiatedByFrame:nil
   completionHandler:^(NSArray *urls){completed++;cancelled=urls==nil;}];
  yd_files_picker_remove(guard);
  for(int i=0;i<100&&completed<2;i++)pump();
  CHECK(completed==2 && cancelled && view.UIDelegate==(id)previous);
  [parameters release];[view release];[window close];[window release];[previous release];
 }
 return 0;
}
`
