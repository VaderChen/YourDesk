//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#include <stdint.h>
extern void ydTrayQuit(uintptr_t handle);
extern void ydTrayShowMCP(uintptr_t handle);
extern void ydTrayDisconnectIncoming(uintptr_t handle);

// 保留 WebView 原有委派，只接管視窗關閉與應用程式結束。
@interface YDTrayController : NSObject <NSWindowDelegate, NSApplicationDelegate>
@property(nonatomic, retain) NSWindow *window;
@property(nonatomic, retain) NSStatusItem *statusItem;
@property(nonatomic, retain) id previousWindowDelegate;
@property(nonatomic, retain) id previousAppDelegate;
@property(nonatomic, retain) NSMenu *previousMainMenu;
@property(nonatomic, retain) NSMenu *trayMenu;
@property(nonatomic, retain) NSPopover *connectionPopover;
@property(nonatomic, retain) NSMenuItem *mcpItem;
@property(nonatomic, retain) NSMenuItem *incomingItem;
@property(nonatomic, retain) NSImage *normalIcon;
@property(nonatomic, retain) NSPopover *mcpPopover;
@property(nonatomic, assign) uintptr_t callback;
@end

@implementation YDTrayController
- (BOOL)windowShouldClose:(NSWindow *)sender {
    [sender orderOut:nil];
    return NO;
}
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender { return NO; }
- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
    ydTrayQuit(self.callback);
    return NSTerminateCancel;
}
- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)visible {
    [self showInterface:nil];
    return YES;
}
- (void)clickStatusItem:(id)sender {
    [self.connectionPopover close];
    NSEvent *event = NSApp.currentEvent;
    if (event.type == NSEventTypeRightMouseUp || (event.modifierFlags & NSEventModifierFlagControl)) {
        NSStatusBarButton *button = self.statusItem.button;
        NSPoint anchor = NSMakePoint(0, button.isFlipped ? NSMaxY(button.bounds) : NSMinY(button.bounds));
        [self.trayMenu popUpMenuPositioningItem:nil atLocation:anchor inView:button];
    } else {
        [self showInterface:sender];
    }
}
- (void)showInterface:(id)sender {
    [NSApp activateIgnoringOtherApps:YES];
    [self.window deminiaturize:nil];
    [self.window makeKeyAndOrderFront:nil];
}
- (void)disconnectIncoming:(id)sender {ydTrayDisconnectIncoming(self.callback);}
- (void)showMCP:(id)sender {ydTrayShowMCP(self.callback);}
- (void)closeMCPNotice { [self.mcpPopover close]; }
- (void)quitProgram:(id)sender { ydTrayQuit(self.callback); }
- (BOOL)respondsToSelector:(SEL)selector {
    return [super respondsToSelector:selector] ||
        [self.previousWindowDelegate respondsToSelector:selector] ||
        [self.previousAppDelegate respondsToSelector:selector];
}
- (id)forwardingTargetForSelector:(SEL)selector {
    if ([self.previousWindowDelegate respondsToSelector:selector]) return self.previousWindowDelegate;
    if ([self.previousAppDelegate respondsToSelector:selector]) return self.previousAppDelegate;
    return [super forwardingTargetForSelector:selector];
}
- (void)closeConnectionNotice { [self.connectionPopover close]; }
- (void)dealloc {
    [NSObject cancelPreviousPerformRequestsWithTarget:self];
    [_mcpPopover close];[_mcpPopover release];[_mcpItem release];[_normalIcon release];
    [_connectionPopover close];
    [_connectionPopover release];
    [_previousMainMenu release];
    [_trayMenu release];
    [_incomingItem release];
    [_window release];
    [_statusItem release];
    [_previousWindowDelegate release];
    [_previousAppDelegate release];
    [super dealloc];
}
@end

void *yd_tray_install(void *nativeWindow, uintptr_t handle) {
    @autoreleasepool {
        YDTrayController *controller = [[YDTrayController alloc] init];
        controller.window = (NSWindow *)nativeWindow;
        controller.callback = handle;
        controller.previousWindowDelegate = controller.window.delegate;
        controller.previousAppDelegate = NSApp.delegate;
        controller.statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
        if (!controller.statusItem) { [controller release]; return NULL; }
        NSImage *icon = nil;
        if (@available(macOS 11.0, *)) {
            icon = [NSImage imageWithSystemSymbolName:@"desktopcomputer" accessibilityDescription:@"YourDesk"];
        }
        if (icon) {
            [icon setTemplate:YES];
            controller.statusItem.button.image = icon;
        } else {
            controller.statusItem.button.title = @"YD";
        }
        controller.statusItem.button.toolTip = @"YourDesk · Client 常駐執行";
        NSMenu *menu = [[NSMenu alloc] initWithTitle:@"YourDesk"];
        NSMenuItem *title = [menu addItemWithTitle:@"YourDesk" action:nil keyEquivalent:@""];
        title.enabled = NO;
        NSMenuItem *show = [menu addItemWithTitle:@"開啟介面" action:@selector(showInterface:) keyEquivalent:@""];
        show.target = controller;
        NSMenuItem *mcp=[menu addItemWithTitle:@"開啟 MCP 遠端畫面" action:@selector(showMCP:) keyEquivalent:@""];mcp.target=controller;mcp.hidden=YES;controller.mcpItem=mcp;
        NSMenuItem *incoming=[menu addItemWithTitle:@"關閉遠端連線" action:@selector(disconnectIncoming:) keyEquivalent:@""];incoming.target=controller;incoming.hidden=YES;controller.incomingItem=incoming;
        [menu addItem:[NSMenuItem separatorItem]];
        NSMenuItem *quit = [menu addItemWithTitle:@"關閉程式" action:@selector(quitProgram:) keyEquivalent:@""];
        quit.target = controller;
        controller.trayMenu = menu;
        controller.statusItem.button.target = controller;
        controller.statusItem.button.action = @selector(clickStatusItem:);
        [controller.statusItem.button sendActionOn:NSEventMaskLeftMouseUp | NSEventMaskRightMouseUp];
        [menu release];
        controller.previousMainMenu = NSApp.mainMenu;
        NSMenu *mainMenu = [[NSMenu alloc] initWithTitle:@"YourDesk"];
        NSMenuItem *appRoot = [mainMenu addItemWithTitle:@"YourDesk" action:nil keyEquivalent:@""];
        NSMenu *appMenu = [[NSMenu alloc] initWithTitle:@"YourDesk"];
        appRoot.submenu = appMenu;
        [appMenu addItemWithTitle:@"隱藏 YourDesk" action:@selector(hide:) keyEquivalent:@"h"];
        NSMenuItem *hideOthers = [appMenu addItemWithTitle:@"隱藏其他程式" action:@selector(hideOtherApplications:) keyEquivalent:@"h"];
        hideOthers.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagOption;
        [appMenu addItemWithTitle:@"全部顯示" action:@selector(unhideAllApplications:) keyEquivalent:@""];
        [appMenu addItem:[NSMenuItem separatorItem]];
        NSMenuItem *appQuit = [appMenu addItemWithTitle:@"結束 YourDesk" action:@selector(quitProgram:) keyEquivalent:@"q"];
        appQuit.target = controller;
        [appMenu release];
        NSMenuItem *editRoot = [mainMenu addItemWithTitle:@"編輯" action:nil keyEquivalent:@""];
        NSMenu *edit = [[NSMenu alloc] initWithTitle:@"編輯"];
        editRoot.submenu = edit;
        [edit addItemWithTitle:@"復原" action:@selector(undo:) keyEquivalent:@"z"];
        NSMenuItem *redo = [edit addItemWithTitle:@"重做" action:@selector(redo:) keyEquivalent:@"z"];
        redo.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagShift;
        [edit addItem:[NSMenuItem separatorItem]];
        [edit addItemWithTitle:@"剪下" action:@selector(cut:) keyEquivalent:@"x"];
        [edit addItemWithTitle:@"複製" action:@selector(copy:) keyEquivalent:@"c"];
        [edit addItemWithTitle:@"貼上" action:@selector(paste:) keyEquivalent:@"v"];
        [edit addItemWithTitle:@"全選" action:@selector(selectAll:) keyEquivalent:@"a"];
        [edit release];
        NSMenuItem *windowRoot = [mainMenu addItemWithTitle:@"視窗" action:nil keyEquivalent:@""];
        NSMenu *windowMenu = [[NSMenu alloc] initWithTitle:@"視窗"];
        windowRoot.submenu = windowMenu;
        [windowMenu addItemWithTitle:@"關閉介面" action:@selector(performClose:) keyEquivalent:@"w"];
        [windowMenu addItemWithTitle:@"最小化" action:@selector(performMiniaturize:) keyEquivalent:@"m"];
        [windowMenu release];
        NSApp.mainMenu = mainMenu;
        [mainMenu release];
        controller.window.delegate = controller;
        NSApp.delegate = controller;
        return controller;
    }
}

void yd_tray_remove(void *tray) {
    @autoreleasepool {
        YDTrayController *controller = (YDTrayController *)tray;
        controller.window.delegate = controller.previousWindowDelegate;
        NSApp.delegate = controller.previousAppDelegate;
        NSApp.mainMenu = controller.previousMainMenu;
        [[NSStatusBar systemStatusBar] removeStatusItem:controller.statusItem];
        [controller release];
    }
}

// 使用內嵌圖示，不依賴外部檔案路徑。
void yd_set_app_icon(void *tray, const void *bytes, size_t length) {
    @autoreleasepool {
        NSData *data = [NSData dataWithBytes:bytes length:length];
        NSImage *icon = [[NSImage alloc] initWithData:data];
        if (!icon) return;
        // 原始圖檔包含灰色外圍；以透明畫布與圓角裁切呈現。
        NSSize size = icon.size;
        NSImage *clipped = [[NSImage alloc] initWithSize:size];
        [clipped lockFocus];
        [[NSColor clearColor] set];
        NSRectFillUsingOperation(NSMakeRect(0, 0, size.width, size.height), NSCompositingOperationCopy);
        [NSGraphicsContext saveGraphicsState];
        NSRect tile = NSMakeRect(size.width * .065, size.height * .065, size.width * .87, size.height * .865);
        [[NSBezierPath bezierPathWithRoundedRect:tile xRadius:size.width * .18 yRadius:size.height * .18] addClip];
        [icon drawInRect:NSMakeRect(0, 0, size.width, size.height) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1];
        [NSGraphicsContext restoreGraphicsState];
        [clipped unlockFocus];
        [NSApp setApplicationIconImage:clipped];
        // Tray 只取圖案範圍，避免透明留白讓圖示再次縮小。
        NSImage *statusIcon = [[NSImage alloc] initWithSize:NSMakeSize(22, 22)];
        [statusIcon lockFocus];
        [clipped drawInRect:NSMakeRect(0, 0, 22, 22) fromRect:tile operation:NSCompositingOperationSourceOver fraction:1];
        [statusIcon unlockFocus];
        [clipped release];
        statusIcon.template = NO;
        YDTrayController *controller = (YDTrayController *)tray;
        controller.statusItem.button.image = statusIcon;
        controller.normalIcon=statusIcon;
        [statusIcon release];
        [icon release];
    }
}

void yd_connection_notice(void *nativeWindow) {
    @autoreleasepool {
        NSWindow *window = (NSWindow *)nativeWindow;
        YDTrayController *controller = (YDTrayController *)window.delegate;
        [window orderOut:nil];
        [NSObject cancelPreviousPerformRequestsWithTarget:controller selector:@selector(closeConnectionNotice) object:nil];
        if (!controller.connectionPopover) {
            NSPopover *popover = [[NSPopover alloc] init];
            popover.behavior = NSPopoverBehaviorTransient;
            NSViewController *content = [[NSViewController alloc] init];
            NSView *view = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 220, 66)];
            NSTextField *label = [NSTextField labelWithString:@"YourDesk · 正在連線"];
            label.font = [NSFont systemFontOfSize:14 weight:NSFontWeightMedium];
            label.frame = NSMakeRect(18, 23, 190, 20);
            [view addSubview:label];
            content.view = view;
            [view release];
            popover.contentViewController = content;
            [content release];
            controller.connectionPopover = popover;
            [popover release];
        }
        [controller.connectionPopover showRelativeToRect:controller.statusItem.button.bounds ofView:controller.statusItem.button preferredEdge:NSMinYEdge];
        [controller performSelector:@selector(closeConnectionNotice) withObject:nil afterDelay:4.0];
    }
}

void yd_update_window(void *nativeWindow) {
    NSWindow *window = (NSWindow *)nativeWindow;
    [NSApp activateIgnoringOtherApps:YES];
    if (window.miniaturized) [window deminiaturize:nil];
    [window makeKeyAndOrderFront:nil];
}

// 傳輸提示不切換 key window，避免 遠端顯示 失焦取消等待中的貼上。
void yd_transfer_window(void *nativeWindow) {
 NSWindow *window=(NSWindow *)nativeWindow;
 [NSApp unhideWithoutActivation];
 if (window.miniaturized) [window deminiaturize:nil];
 [window orderFrontRegardless];
}

void yd_close_interface(void *window) { [(NSWindow *)window performClose:nil]; }

void yd_mcp_tray(void *nativeWindow, int count, int notify, int visible) {
 @autoreleasepool {
  YDTrayController *c=(YDTrayController *)[(NSWindow *)nativeWindow delegate];
  c.mcpItem.hidden=count==0;
 c.mcpItem.title=visible?@"隱藏 MCP 遠端畫面":@"開啟 MCP 遠端畫面";
  c.statusItem.button.toolTip=count>0 ? [NSString stringWithFormat:@"YourDesk · MCP 正在操作（%d 個連線）",count] : @"YourDesk · Client 常駐執行";
  if(count>0 && c.normalIcon){
   // 只替換有彩度的品牌色，保留白色圖案與透明邊緣。
   NSBitmapImageRep *pixels=[[NSBitmapImageRep alloc] initWithData:[c.normalIcon TIFFRepresentation]];
   for(NSInteger y=0;y<pixels.pixelsHigh;y++)for(NSInteger x=0;x<pixels.pixelsWide;x++){
    NSColor *color=[[pixels colorAtX:x y:y] colorUsingColorSpace:[NSColorSpace deviceRGBColorSpace]];
    CGFloat r=color.redComponent,g=color.greenComponent,b=color.blueComponent;
    CGFloat saturation=MAX(r,MAX(g,b))-MIN(r,MIN(g,b));
    CGFloat amount=MIN(1.0,saturation/0.35);
    [pixels setColor:[NSColor colorWithDeviceRed:r+(1-r)*amount green:g+(0.584-g)*amount blue:b*(1-amount) alpha:color.alphaComponent] atX:x y:y];
   }
   NSImage *tinted=[[NSImage alloc] initWithSize:c.normalIcon.size];
   [tinted addRepresentation:pixels];[pixels release];
   tinted.template=NO;c.statusItem.button.image=tinted;[tinted release];
  }else if(c.normalIcon){c.statusItem.button.image=c.normalIcon;}
  [NSObject cancelPreviousPerformRequestsWithTarget:c selector:@selector(closeMCPNotice) object:nil];
  [c.mcpPopover close];
  if(!notify || count<=0)return;
  if(!c.mcpPopover){
   NSPopover *popover=[[NSPopover alloc] init];popover.behavior=NSPopoverBehaviorTransient;
   NSViewController *content=[[NSViewController alloc] init];NSView *view=[[NSView alloc] initWithFrame:NSMakeRect(0,0,280,76)];
   NSTextField *label=[NSTextField labelWithString:@"MCP 正在操作遠端\n可從 Tray 選單開啟遠端畫面"];
   label.frame=NSMakeRect(16,16,248,44);label.font=[NSFont systemFontOfSize:13];[view addSubview:label];
   content.view=view;[view release];popover.contentViewController=content;[content release];c.mcpPopover=popover;[popover release];
  }
  [c.mcpPopover showRelativeToRect:c.statusItem.button.bounds ofView:c.statusItem.button preferredEdge:NSMinYEdge];
  [c performSelector:@selector(closeMCPNotice) withObject:nil afterDelay:8.0];
 }
}

void yd_incoming_tray(void *window,int connected){@autoreleasepool {YDTrayController *c=(YDTrayController *)[(NSWindow *)window delegate];c.incomingItem.hidden=!connected;}}
