#!/usr/bin/env python3
"""建立實際 CGEvent 並攔截 Post；驗證旗標，不注入使用者桌面。"""
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
preamble = (ROOT / 'internal/input/input_darwin.go').read_text().split('/*', 1)[1].split('*/', 1)[0]
preamble = '\n'.join(line for line in preamble.splitlines() if not line.startswith('#cgo'))
headers = '''
#import <Cocoa/Cocoa.h>
#include <ApplicationServices/ApplicationServices.h>
#include <IOKit/hidsystem/IOHIDLib.h>
#include <IOKit/hidsystem/IOHIDParameter.h>
#include <assert.h>
static CGEventFlags postedFlags;
static CGEventType postedType;
static int posts;
static void capturePost(CGEventTapLocation tap, CGEventRef event) {
    postedFlags=CGEventGetFlags(event);postedType=CGEventGetType(event);posts++;
}
#define CGEventPost capturePost
'''
main = '''
int main(void) { @autoreleasepool {
 const int codes[]={56,59,58,55};
 const CGEventFlags flags[]={kCGEventFlagMaskShift,kCGEventFlagMaskControl,kCGEventFlagMaskAlternate,kCGEventFlagMaskCommand};
 for(int i=0;i<4;i++) {
    yd_raw_key(codes[i],1,1ULL<<i,0,0,0);
    yd_mouse_button(10,10,1,1);assert(postedFlags==flags[i]);
    yd_mouse_move(11,11);assert(postedType==kCGEventLeftMouseDragged && postedFlags==flags[i]);
    yd_wheel(1);assert(postedType==kCGEventScrollWheel && postedFlags==flags[i]);
    yd_mouse_button(11,11,1,0);assert(postedFlags==flags[i]);
    yd_raw_key(codes[i],0,0,0,0,0);
    yd_mouse_move(12,12);assert(postedFlags==0);
    yd_key(codes[i],1);
    yd_mouse_button(12,12,2,1);assert(postedFlags==flags[i]);
    yd_mouse_button(12,12,2,0);
    yd_key(codes[i],0);yd_mouse_move(12,12);assert(postedFlags==0);
 }
 yd_key(56,1);yd_key(59,1);
 yd_mouse_button(12,12,3,1);assert(postedFlags==(kCGEventFlagMaskShift|kCGEventFlagMaskControl));
 yd_key(56,0);yd_mouse_move(13,13);assert(postedFlags==kCGEventFlagMaskControl);
 yd_mouse_button(13,13,3,0);yd_key(59,0);yd_wheel(1);assert(postedFlags==0);
 yd_raw_key(83,1,1,0,1,0);yd_mouse_move(14,14);assert(postedFlags==kCGEventFlagMaskShift);
 yd_raw_key(83,0,0,0,1,0);yd_mouse_move(14,14);assert(postedFlags==0);
 assert(posts>40);
 puts("PASS 原始／備援修飾鍵、左右中鍵、拖曳、滾輪與釋放；未向桌面注入事件");
 return 0;
} }
'''
with tempfile.TemporaryDirectory(prefix='yourdesk-input-smoke-') as temp:
    source=Path(temp)/'smoke.m'
    source.write_text(headers+preamble+'\n'+(ROOT/'internal/input/mouse_darwin.m').read_text()+'\n'+main)
    executable=Path(temp)/'smoke'
    subprocess.run(['clang','-fobjc-arc',str(source),'-o',str(executable),'-framework','Cocoa','-framework','ApplicationServices','-framework','IOKit'],check=True)
    subprocess.run([str(executable)],check=True)
