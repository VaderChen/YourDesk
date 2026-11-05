//go:build darwin && cgo
#import <Cocoa/Cocoa.h>

#import <ApplicationServices/ApplicationServices.h>
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
static atomic_bool shortcutGuard=false;
void yd_keyboard_shortcut_guard(int value){atomic_store(&shortcutGuard,value!=0);}
static BOOL capsPhysicalDown=NO;
static BOOL tappedKeys[128];
static CFMachPortRef keyEventTap=NULL;
static CFRunLoopSourceRef keyEventTapSource=NULL;
static uint64_t modifiers(NSEventModifierFlags f) {
 return ((f&NSEventModifierFlagShift)?1:0)|((f&NSEventModifierFlagControl)?2:0)|((f&NSEventModifierFlagOption)?4:0)|((f&NSEventModifierFlagCommand)?8:0)|((f&NSEventModifierFlagCapsLock)?16:0)|((f&NSEventModifierFlagFunction)?32:0);
}
static BOOL modifierDown(int keyCode, uint64_t f) {
 switch(keyCode){
  case 56:return (f&2)!=0||((f&(2|4))==0&&(f&NSEventModifierFlagShift)!=0);
  case 60:return (f&4)!=0||((f&(2|4))==0&&(f&NSEventModifierFlagShift)!=0);
  case 59:return (f&1)!=0||((f&(1|0x2000))==0&&(f&NSEventModifierFlagControl)!=0);
  case 62:return (f&0x2000)!=0||((f&(1|0x2000))==0&&(f&NSEventModifierFlagControl)!=0);
  case 55:return (f&8)!=0||((f&(8|16))==0&&(f&NSEventModifierFlagCommand)!=0);
  case 54:return (f&16)!=0||((f&(8|16))==0&&(f&NSEventModifierFlagCommand)!=0);
  case 58:return (f&32)!=0||((f&(32|64))==0&&(f&NSEventModifierFlagOption)!=0);
  case 61:return (f&64)!=0||((f&(32|64))==0&&(f&NSEventModifierFlagOption)!=0);
  case 57:return (f&NSEventModifierFlagCapsLock)!=0;
  case 63:return (f&NSEventModifierFlagFunction)!=0;
  default:return NO;
 }
}
static BOOL ydViewerCapturingWindow(NSWindow *candidate) {
 if(!atomic_load(&enabled)||atomic_load(&suspended)||!NSApp.active)return NO;
 Class viewer=NSClassFromString(@"GLFWWindow");
 NSWindow *window=candidate ?: NSApp.keyWindow;
 if(!viewer||!window||![window isKindOfClass:viewer]||!window.keyWindow||window.attachedSheet)return NO;
 // 只觀察焦點，文字欄位和 WebView 需要自行處理本機編輯與組字。
 return window.firstResponder==window.contentView;
}
static void ydRecordKey(int keyCode, BOOL flagsChanged, BOOL isDown, BOOL isRepeat, uint64_t rawFlags) {
 BOOL down=flagsChanged ? modifierDown(keyCode,rawFlags) : isDown;
 YDKey key={keyCode,down,!flagsChanged&&isDown&&isRepeat,0,modifiers((NSEventModifierFlags)rawFlags),rawFlags};
 pthread_mutex_lock(&keyMutex);
 if(count>=1022){count=0;events[count++]=(YDKey){.reset=1};}
 if(flagsChanged && keyCode==57){
  const uint64_t physical=NX_ALPHASHIFT_STATELESS_MASK|NX_DEVICE_ALPHASHIFT_STATELESS_MASK;
  BOOL pressed=(rawFlags&physical)!=0;
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
}
static void ydUpdateKeyTap(void);
static CGEventRef ydKeyEventTapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
 if(type==kCGEventTapDisabledByTimeout||type==kCGEventTapDisabledByUserInput){
  if(keyEventTap && ydViewerCapturingWindow(nil))CGEventTapEnable(keyEventTap,true);
  return event;
 }
 if(type!=kCGEventKeyDown&&type!=kCGEventKeyUp&&type!=kCGEventFlagsChanged)return event;
 if(CGEventGetIntegerValueField(event,kCGEventSourceUserData)==0x5944524b)return event;
 int keyCode=(int)CGEventGetIntegerValueField(event,kCGKeyboardEventKeycode);
 uint64_t rawFlags=(uint64_t)CGEventGetFlags(event);
 // 系統層只接管遠端 Control 導覽／功能快捷鍵。一般編輯、輸入法及
 // 修飾鍵交給視窗事件；避免全域停用熱鍵或攔走其他應用程式的輸入。
 if(!ydViewerCapturingWindow(nil) || keyCode<0 || keyCode>=128)return event;
 if(type==kCGEventKeyDown){
  if(!(rawFlags&kCGEventFlagMaskControl))return event;
  switch(keyCode){
   case 49:case 96:case 97:case 98:case 99:case 100:case 101:case 103:case 105:
   case 106:case 107:case 109:case 111:case 113:case 114:case 115:case 116:
   case 117:case 118:case 119:case 120:case 121:case 122:case 123:case 124:
   case 125:case 126:case 64:case 79:case 80:case 90:break;
   default:return event;
  }
  tappedKeys[keyCode]=YES;
 }else if(type==kCGEventKeyUp && tappedKeys[keyCode]){
  tappedKeys[keyCode]=NO;
 }else return event;
 BOOL isRepeat=type==kCGEventKeyDown && CGEventGetIntegerValueField(event,kCGKeyboardEventAutorepeat)!=0;
 ydRecordKey(keyCode,type==kCGEventFlagsChanged,type==kCGEventKeyDown,isRepeat,rawFlags);
 return NULL;
}
static void ydUpdateKeyTap(void) {
 BOOL active=ydViewerCapturingWindow(nil);
 if(active){

  if(!keyEventTap && AXIsProcessTrusted()){
   CGEventMask mask=CGEventMaskBit(kCGEventKeyDown)|CGEventMaskBit(kCGEventKeyUp)|CGEventMaskBit(kCGEventFlagsChanged);
   keyEventTap=CGEventTapCreate(kCGSessionEventTap,kCGHeadInsertEventTap,kCGEventTapOptionDefault,mask,ydKeyEventTapCallback,NULL);
   if(keyEventTap){
    keyEventTapSource=CFMachPortCreateRunLoopSource(kCFAllocatorDefault,keyEventTap,0);
    if(keyEventTapSource)CFRunLoopAddSource(CFRunLoopGetMain(),keyEventTapSource,kCFRunLoopCommonModes);
    CGEventTapEnable(keyEventTap,true);
   }
  }
  if(keyEventTap && !CGEventTapIsEnabled(keyEventTap))CGEventTapEnable(keyEventTap,true);
 }else{
  memset(tappedKeys,0,sizeof(tappedKeys));
  if(keyEventTap)CGEventTapEnable(keyEventTap,false);
 }
}
int yd_shortcut_released(int code,uint64_t mods){
 return !CGEventSourceKeyState(kCGEventSourceStateHIDSystemState,(CGKeyCode)code) &&
  (modifiers((NSEventModifierFlags)CGEventSourceFlagsState(kCGEventSourceStateHIDSystemState)) & mods & 15)==0;
}
void yd_keyboard_suspend(int value){
 atomic_store(&suspended,value!=0);
 if(value){pthread_mutex_lock(&keyMutex);if(count>=1024)count=0;events[count++]=(YDKey){.reset=1};pthread_mutex_unlock(&keyMutex);}
 dispatch_async(dispatch_get_main_queue(),^{ydUpdateKeyTap();});
}
void yd_keyboard_enabled(int value) {
 bool prev=atomic_exchange(&enabled,value!=0);
 if(prev!=(value!=0)){
  dispatch_async(dispatch_get_main_queue(),^{ydUpdateKeyTap();});
 }
 static dispatch_once_t once;
 dispatch_once(&once,^{ dispatch_async(dispatch_get_main_queue(),^{
  NSNotificationCenter *nc=NSNotificationCenter.defaultCenter;
  for(NSNotificationName name in @[NSApplicationDidBecomeActiveNotification,NSApplicationDidResignActiveNotification,NSWindowDidBecomeKeyNotification,NSWindowDidResignKeyNotification]){
   [nc addObserverForName:name object:nil queue:NSOperationQueue.mainQueue usingBlock:^(NSNotification *note){ydUpdateKeyTap();}];
  }
  ydUpdateKeyTap();
  [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown|NSEventMaskKeyUp|NSEventMaskFlagsChanged handler:^NSEvent *(NSEvent *event){
   if(event.CGEvent && CGEventGetIntegerValueField(event.CGEvent,kCGEventSourceUserData)==0x5944524b)return event;
   if(atomic_load(&shortcutGuard) && (event.modifierFlags&NSEventModifierFlagCommand) && (event.keyCode==12||event.keyCode==13))return nil;
   ydUpdateKeyTap();
   if(!ydViewerCapturingWindow(event.window))return event;
   ydRecordKey(event.keyCode,event.type==NSEventTypeFlagsChanged,event.type==NSEventTypeKeyDown,event.type==NSEventTypeKeyDown&&event.isARepeat,event.modifierFlags);
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
