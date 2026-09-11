//go:build darwin && cgo
#import <Cocoa/Cocoa.h>
#import <dispatch/dispatch.h>
#include <IOKit/hidsystem/IOLLEvent.h>
#include <pthread.h>
#include <stdatomic.h>
#include <stdint.h>
typedef struct {int code,down,repeat,reset;uint64_t mods,flags;} YDKey;
static YDKey events[1024];
static int count=0;
static pthread_mutex_t keyMutex=PTHREAD_MUTEX_INITIALIZER;
static atomic_bool enabled=false,suspended=false;
static BOOL capsPhysicalDown=NO;
static uint64_t modifiers(NSEventModifierFlags f) {
 return ((f&NSEventModifierFlagShift)?1:0)|((f&NSEventModifierFlagControl)?2:0)|((f&NSEventModifierFlagOption)?4:0)|((f&NSEventModifierFlagCommand)?8:0)|((f&NSEventModifierFlagCapsLock)?16:0)|((f&NSEventModifierFlagFunction)?32:0);
}
void yd_keyboard_suspend(int value){
 atomic_store(&suspended,value!=0);
 if(value){pthread_mutex_lock(&keyMutex);if(count>=1024)count=0;events[count++]=(YDKey){.reset=1};pthread_mutex_unlock(&keyMutex);}
}
void yd_keyboard_enabled(int value) {
 atomic_store(&enabled,value!=0);
 static dispatch_once_t once;
 dispatch_once(&once,^{ dispatch_async(dispatch_get_main_queue(),^{
  [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown|NSEventMaskKeyUp|NSEventMaskFlagsChanged handler:^NSEvent *(NSEvent *event){
   if(event.CGEvent && CGEventGetIntegerValueField(event.CGEvent,kCGEventSourceUserData)==0x5944524b)return event;
   Class viewer=NSClassFromString(@"GLFWWindow");
   if(!atomic_load(&enabled)||atomic_load(&suspended)||!viewer||![event.window isKindOfClass:viewer]||!event.window.keyWindow||event.window.firstResponder!=event.window.contentView)return event;
   BOOL down=event.type==NSEventTypeKeyDown;
   if(event.type==NSEventTypeFlagsChanged){
    NSUInteger mask=0;
    switch(event.keyCode){case 56:mask=2;break;case 60:mask=4;break;case 59:mask=1;break;case 62:mask=0x2000;break;case 55:mask=8;break;case 54:mask=16;break;case 58:mask=32;break;case 61:mask=64;break;case 57:mask=NSEventModifierFlagCapsLock;break;case 63:mask=NSEventModifierFlagFunction;break;}
    down=(event.modifierFlags&mask)!=0;
   }
   YDKey key={event.keyCode,down,event.type==NSEventTypeKeyDown&&event.isARepeat,0,modifiers(event.modifierFlags),event.modifierFlags};
   pthread_mutex_lock(&keyMutex);
   if(count>=1022){count=0;events[count++]=(YDKey){.reset=1};}
   if(event.type==NSEventTypeFlagsChanged && event.keyCode==57){
    const uint64_t physical=NX_ALPHASHIFT_STATELESS_MASK|NX_DEVICE_ALPHASHIFT_STATELESS_MASK;
    BOOL pressed=(event.modifierFlags&physical)!=0;
    if(pressed || capsPhysicalDown){
     // 實體按鍵狀態與大寫鎖定狀態不同，保留按住時間及放開事件。
     key.down=pressed;capsPhysicalDown=pressed;events[count++]=key;
    }else{
     // 較舊的事件來源只回報鎖定變化，補成一次完整按鍵。
     key.down=1;key.flags|=physical;events[count++]=key;
     key.down=0;key.flags&=~physical;events[count++]=key;
    }
   }else{events[count++]=key;}
   pthread_mutex_unlock(&keyMutex);
   // 在 interpretKeyEvents／本機輸入法之前擷取，不讓 遠端顯示 消化組合鍵。
   return nil;
  }];
 });});
}
int yd_keyboard_pop(int *code,uint64_t *mods,uint64_t *flags,int *down,int *repeat,int *reset){
 pthread_mutex_lock(&keyMutex);
 if(!count){pthread_mutex_unlock(&keyMutex);return 0;}
 YDKey key=events[0];count--;memmove(events,events+1,count*sizeof(YDKey));
 pthread_mutex_unlock(&keyMutex);
 *code=key.code;*mods=key.mods;*flags=key.flags;*down=key.down;*repeat=key.repeat;*reset=key.reset;return 1;
}
