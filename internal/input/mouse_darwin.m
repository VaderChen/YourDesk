//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#import <CoreGraphics/CoreGraphics.h>
#include <pthread.h>
#include <limits.h>
#include <math.h>

// CGEvent 不會替人工注入事件累積 clickState，需對每顆按鍵保存連點狀態。
// 滑鼠座標是遠端的邏輯點，避免 Retina 與 遠端顯示 縮放改變連點距離判定。
typedef struct {
    BOOL pressed, released, moved;
    NSInteger count;
    NSTimeInterval lastDown;
    CGPoint origin;
} YDMouseButtonState;
static YDMouseButtonState buttons[3];
static int lastButton = -1;
static pthread_mutex_t mouseMutex = PTHREAD_MUTEX_INITIALIZER;

static void ydTrackPointer(CGPoint point) {
    for (int i=0; i<3; i++) {
        if (fabs(point.x-buttons[i].origin.x)>4 || fabs(point.y-buttons[i].origin.y)>4) buttons[i].moved=YES;
    }
}
void yd_mouse_move(double x,double y) {
    pthread_mutex_lock(&mouseMutex);
    CGPoint point=CGPointMake(x,y);
    ydTrackPointer(point);
    CGEventType type=kCGEventMouseMoved;
    CGMouseButton button=kCGMouseButtonLeft;
    if (buttons[0].pressed) type=kCGEventLeftMouseDragged;
    else if (buttons[1].pressed) {type=kCGEventRightMouseDragged;button=kCGMouseButtonRight;}
    else if (buttons[2].pressed) {type=kCGEventOtherMouseDragged;button=kCGMouseButtonCenter;}
    CGEventRef event=CGEventCreateMouseEvent(NULL,type,point,button);
    if(event){CGEventPost(kCGHIDEventTap,event);CFRelease(event);}
    pthread_mutex_unlock(&mouseMutex);
}
void yd_mouse_button(double x,double y,int button,int down) {
    if(button<1 || button>3)return;
    @autoreleasepool {
        pthread_mutex_lock(&mouseMutex);
        int index=button-1;
        CGPoint point=CGPointMake(x,y);
        ydTrackPointer(point);
        YDMouseButtonState *state=&buttons[index];
        NSTimeInterval now=NSProcessInfo.processInfo.systemUptime;
        if(down) {
            BOOL consecutive=lastButton==index && state->released && !state->pressed && !state->moved &&
                now-state->lastDown<=[NSEvent doubleClickInterval] && state->count<INT_MAX;
            state->count=consecutive ? state->count+1 : 1;
            state->lastDown=now;state->origin=point;
            state->pressed=YES;state->released=NO;state->moved=NO;
            lastButton=index;
        }
        CGEventType type=down?kCGEventLeftMouseDown:kCGEventLeftMouseUp;
        CGMouseButton nativeButton=kCGMouseButtonLeft;
        if(button==2){type=down?kCGEventRightMouseDown:kCGEventRightMouseUp;nativeButton=kCGMouseButtonRight;}
        if(button==3){type=down?kCGEventOtherMouseDown:kCGEventOtherMouseUp;nativeButton=kCGMouseButtonCenter;}
        CGEventRef event=CGEventCreateMouseEvent(NULL,type,point,nativeButton);
        if(event){
            CGEventSetIntegerValueField(event,kCGMouseEventClickState,MAX(1,state->count));
            CGEventPost(kCGHIDEventTap,event);CFRelease(event);
        }
        if(!down){state->released=state->pressed;state->pressed=NO;}
        pthread_mutex_unlock(&mouseMutex);
    }
}
