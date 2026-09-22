//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

@interface YDFilesPicker : NSObject <WKUIDelegate>
@property(nonatomic, assign) WKWebView *view;
@property(nonatomic, retain) id previousDelegate;
@property(nonatomic, retain) NSOpenPanel *panel;
@end

@implementation YDFilesPicker
- (void)webView:(WKWebView *)view runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters
    initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *))completion {
    if (self.panel || !view.window) { completion(nil); return; }
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    panel.canChooseFiles = YES;
    panel.canChooseDirectories = parameters.allowsDirectories;
    panel.allowsMultipleSelection = parameters.allowsMultipleSelection;
    self.panel = panel;
    // Sheet 綁定檔案視窗；不在 WebKit 回呼中進入巢狀 runModal。
    [panel beginSheetModalForWindow:view.window completionHandler:^(NSModalResponse response) {
        NSArray<NSURL *> *urls = response == NSModalResponseOK ? panel.URLs : nil;
        completion(urls);
        self.panel = nil;
    }];
}
- (BOOL)respondsToSelector:(SEL)selector {
    return [super respondsToSelector:selector] || [self.previousDelegate respondsToSelector:selector];
}
- (id)forwardingTargetForSelector:(SEL)selector {
    if ([self.previousDelegate respondsToSelector:selector]) return self.previousDelegate;
    return [super forwardingTargetForSelector:selector];
}
- (void)dealloc { [_previousDelegate release]; [_panel release]; [super dealloc]; }
@end

static WKWebView *yd_files_webview(NSView *view) {
    if ([view isKindOfClass:[WKWebView class]]) return (WKWebView *)view;
    for (NSView *child in view.subviews) {
        WKWebView *found = yd_files_webview(child);
        if (found) return found;
    }
    return nil;
}

void *yd_files_picker_install(void *nativeWindow) {
    @autoreleasepool {
        if (![NSThread isMainThread] || !nativeWindow) return NULL;
        WKWebView *view = yd_files_webview(((NSWindow *)nativeWindow).contentView);
        if (!view) return NULL;
        YDFilesPicker *guard = [[YDFilesPicker alloc] init];
        guard.view = view;
        guard.previousDelegate = view.UIDelegate;
        view.UIDelegate = guard;
        return guard;
    }
}

void yd_files_picker_remove(void *nativeGuard) {
    @autoreleasepool {
        if (![NSThread isMainThread] || !nativeGuard) return;
        YDFilesPicker *guard = (YDFilesPicker *)nativeGuard;
        [guard.panel cancel:nil];
        if (guard.view.UIDelegate == guard) guard.view.UIDelegate = guard.previousDelegate;
        guard.view = nil;
        [guard release];
    }
}
