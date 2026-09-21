//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#include <stdint.h>
extern void ydFilesRequestClose(uintptr_t handle);

@interface YDFilesCloseGuard : NSObject <NSWindowDelegate>
@property(nonatomic, assign) NSWindow *window;
@property(nonatomic, retain) id previousDelegate;
@property(nonatomic, assign) uintptr_t callback;
@end

@implementation YDFilesCloseGuard
- (BOOL)windowShouldClose:(NSWindow *)sender {
    if (self.callback) ydFilesRequestClose(self.callback);
    return NO;
}
- (void)windowWillClose:(NSNotification *)notice {
    if (self.window.delegate == self) self.window.delegate = self.previousDelegate;
    self.window = nil;
    if ([self.previousDelegate respondsToSelector:@selector(windowWillClose:)])
        [self.previousDelegate windowWillClose:notice];
}
- (BOOL)respondsToSelector:(SEL)selector {
    return [super respondsToSelector:selector] || [self.previousDelegate respondsToSelector:selector];
}
- (id)forwardingTargetForSelector:(SEL)selector {
    if ([self.previousDelegate respondsToSelector:selector]) return self.previousDelegate;
    return [super forwardingTargetForSelector:selector];
}
- (void)dealloc { [_previousDelegate release]; [super dealloc]; }
@end

void *yd_files_close_install(void *nativeWindow, uintptr_t callback) {
    @autoreleasepool {
        if (![NSThread isMainThread] || !nativeWindow || !callback) return NULL;
        NSWindow *window = (NSWindow *)nativeWindow;
        YDFilesCloseGuard *guard = [[YDFilesCloseGuard alloc] init];
        guard.window = window;
        guard.previousDelegate = window.delegate;
        guard.callback = callback;
        window.delegate = guard;
        return guard;
    }
}

void yd_files_close_remove(void *nativeGuard) {
    @autoreleasepool {
        if (![NSThread isMainThread] || !nativeGuard) return;
        YDFilesCloseGuard *guard = (YDFilesCloseGuard *)nativeGuard;
        guard.callback = 0;
        if (guard.window.delegate == guard) guard.window.delegate = guard.previousDelegate;
        guard.window = nil;
        [guard release];
    }
}
