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

#include <pthread.h>
#include <stdatomic.h>
#include <stdbool.h>
#include <time.h>

static pthread_mutex_t pointerLock=PTHREAD_MUTEX_INITIALIZER;
static atomic_bool pointerPending=false;
static double pointerX,pointerY,pointerAt;
static int pointerValid;
static double yd_pointer_time(void){
    struct timespec t;clock_gettime(CLOCK_MONOTONIC,&t);
    return (double)t.tv_sec+(double)t.tv_nsec/1e9;
}

// AppKit 主執行緒直接取系統游標，不依賴 GLFW 收到 mouseMoved。
// 合併尚未執行的請求，UI 忙碌時不累积主執行緒工作。
void yd_poll_native_pointer(void){
    if(atomic_exchange(&pointerPending,true))return;
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *window=NSApp.keyWindow;
        Class viewer=NSClassFromString(@"GLFWWindow");
        int valid=NSApp.active && window && viewer && [window isKindOfClass:viewer];
        double x=0,y=0;
        if(valid){
            NSView *view=window.contentView;
            NSRect bounds=view.bounds;
            valid=bounds.size.width>0 && bounds.size.height>0;
            if(valid){
                NSPoint point=[view convertPoint:[window convertPointFromScreen:NSEvent.mouseLocation] fromView:nil];
                x=(point.x-NSMinX(bounds))/bounds.size.width;
                y=(view.isFlipped?point.y-NSMinY(bounds):NSMaxY(bounds)-point.y)/bounds.size.height;
            }
        }
        pthread_mutex_lock(&pointerLock);
        pointerX=x;pointerY=y;pointerValid=valid;pointerAt=yd_pointer_time();
        pthread_mutex_unlock(&pointerLock);
        atomic_store(&pointerPending,false);
    });
}

int yd_native_pointer(double *x,double *y){
    pthread_mutex_lock(&pointerLock);
    int valid=pointerValid && yd_pointer_time()-pointerAt<0.25;
    if(valid){*x=pointerX;*y=pointerY;}
    pthread_mutex_unlock(&pointerLock);
    return valid;
}
