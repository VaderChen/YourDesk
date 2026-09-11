//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#import <dispatch/dispatch.h>

// 使用系統游標；取消輸入文字後暫時隱藏，不變更游標 hide/unhide 計數。
void yd_show_native_cursor(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSCursor setHiddenUntilMouseMoves:NO];
        [[NSCursor arrowCursor] set];
    });
}
