//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <dispatch/dispatch.h>
#include <stdatomic.h>
#include <math.h>
void yd_keyboard_suspend(int value);

static atomic_bool fullscreenRequested = false;
static atomic_bool fullscreenTransitioning = false;
static NSInteger pendingMenuAction = 0;
static atomic_int titlebarAction = 0;
static atomic_int titlebarMode = 0;
static atomic_int qualityMode = 1;
static atomic_int displayIndex = 0;
static atomic_int displayCount = 0;
static atomic_bool displayPending = false;
static atomic_bool titlebarConfigured = false;
static atomic_bool titlebarConfigurePending = false;
static WKWebView *titlebarWeb;
static NSView *titlebarControls;
static NSArray<NSValue *> *titlebarInteractiveRects;
static atomic_bool fullscreenActive = false;
// 視窗式全螢幕保留同一個 NSWindow、WebView 與繪圖表面。
static NSRect windowedFrame;
static NSWindowStyleMask windowedStyle;
static NSWindowCollectionBehavior windowedCollection;
static NSInteger windowedLevel;
static NSApplicationPresentationOptions windowedPresentation;
static BOOL fullscreenPresentationApplied = NO;
static atomic_bool titlebarVisible = true;
static NSTimeInterval edgeHoverStarted = 0, titlebarLeaveStarted = 0;
static BOOL titlebarLoaded = NO;
// 僅由主執行緒存取。
static NSString *videoCodec=@"", *sourceEncoding=@"", *receiverDecoding=@"unknown";
static NSDictionary *enhancementStatus;
static BOOL renderFPS = NO;
static double trafficTX = 0, trafficRX = 0, viewerFPS = 0;
static NSPopover *titlebarTooltip;
// 僅在 AppKit 主執行緒存取，包含尚未顯示的排程。
static BOOL titlebarMenuOpen = NO;
static NSDictionary *titlebarStrings;
static NSString *titlebarLanguage = @"zh-Hant";
static NSString *ydText(NSString *source) { return titlebarStrings[source] ?: source; }


static BOOL ydSupportsTitlebarControls(NSWindow *window) {
    Class viewerClass = NSClassFromString(@"GLFWWindow");
    return window && viewerClass && [window isKindOfClass:viewerClass];
}

static void ydHideNativeTitlebar(NSWindow *window) {
    window.titlebarAppearsTransparent = YES;
    window.titleVisibility = NSWindowTitleHidden;
    for (NSNumber *kind in @[@(NSWindowCloseButton), @(NSWindowMiniaturizeButton), @(NSWindowZoomButton)]) {
        [window standardWindowButton:kind.unsignedIntegerValue].hidden = YES;
    }
    NSView *content = window.contentView;
    NSView *native = [window standardWindowButton:NSWindowCloseButton].superview;
    if (native && native != content && native != content.superview) native.hidden = YES;
}

// 所有 WebKit 與視窗操作都留在 AppKit 主執行緒。
static void ydUpdateTitlebar(void) {
    if (!titlebarLoaded || !titlebarWeb.window) return;
    NSDictionary *state = @{@"title": titlebarWeb.window.title ?: @"YourDesk",
        @"strings": titlebarStrings ?: @{}, @"language": titlebarLanguage,
        @"enhancement":enhancementStatus ?: @{},
        @"videoCodec":videoCodec,@"sourceEncoding":sourceEncoding,@"receiverDecoding":receiverDecoding,
        @"tx": @(trafficTX), @"rx": @(trafficRX), @"fps": @(viewerFPS), @"renderFPS": @(renderFPS),
        @"mode": @(atomic_load(&titlebarMode)), @"display": @(atomic_load(&displayIndex)),
        @"count": @(atomic_load(&displayCount)), @"pending": @(atomic_load(&displayPending))};
    NSData *data = [NSJSONSerialization dataWithJSONObject:state options:0 error:nil];
    NSString *json = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
    [titlebarWeb evaluateJavaScript:[NSString stringWithFormat:@"window.setTitlebarState(%@)", json] completionHandler:nil];
    [json release];
}

@interface YDTitlebarBridge : NSObject <WKScriptMessageHandler, WKNavigationDelegate>
@end
@implementation YDTitlebarBridge
- (void)selectMenuItem:(NSMenuItem *)item {
    pendingMenuAction = item.tag;
}
- (void)showMenu:(NSDictionary *)body {
    pendingMenuAction = 0;
    NSMenu *menu = [[NSMenu alloc] initWithTitle:@""];
    menu.autoenablesItems = NO;
    void (^addItem)(NSString *, NSInteger, BOOL, BOOL) = ^(NSString *title, NSInteger tag, BOOL checked, BOOL enabled) {
        NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:ydText(title) action:@selector(selectMenuItem:) keyEquivalent:@""];
        item.target = self;
        item.tag = tag;
        item.state = checked ? NSControlStateValueOn : NSControlStateValueOff;
        item.enabled = enabled;
        [menu addItem:item];
        [item release];
    };
    if ([body[@"menu"] isEqual:@"quality"]) {
        int quality = atomic_load(&qualityMode);
        addItem(@"低流量",10,quality==0,YES);
        addItem(@"標準",11,quality==1,YES);
        addItem(@"高畫質",12,quality==2,YES);
    } else if ([body[@"menu"] isEqual:@"scale"]) {
        int mode = atomic_load(&titlebarMode);
        addItem(@"原始解析度（1:1）", 1, mode == 1, YES);
        addItem(@"自動符合視窗", 2, mode == 2, YES);
        addItem(@"只縮小，不放大", 3, mode == 0, YES);
        [menu addItem:[NSMenuItem separatorItem]];
        BOOL fullscreen = atomic_load(&fullscreenActive);
        addItem(fullscreen ? @"離開全螢幕" : @"進入全螢幕", 4, fullscreen, YES);
    } else if ([body[@"menu"] isEqual:@"display"]) {
        int count = atomic_load(&displayCount), selected = atomic_load(&displayIndex);
        for (int i = 0; i < count; i++) {
            addItem([NSString stringWithFormat:ydText(@"螢幕 %d"), i+1], 100+i, i == selected, !atomic_load(&displayPending));
        }
    }
    // 原生浮動選單不受 52pt WebView 裁切，支援方向鍵、Esc 與點擊外部關閉。
    // 選單與按鈕底部保留間距，避免浮動選單蓋住入口。
    CGFloat x = [body[@"x"] doubleValue], y = [body[@"y"] doubleValue] + 10;
    NSPoint point = NSMakePoint(x, titlebarWeb.isFlipped ? y : titlebarWeb.bounds.size.height-y);
    yd_keyboard_suspend(1);
    @try {
        if (titlebarWeb.window.visible) [menu popUpMenuPositioningItem:nil atLocation:point inView:titlebarWeb];
    } @finally {
        yd_keyboard_suspend(0);
        [titlebarWeb.window makeFirstResponder:titlebarWeb.window.contentView];
        [menu release];
    }
}

- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    if (!message.frameInfo.mainFrame) return;
    if ([message.body isKindOfClass:[NSDictionary class]]) {
        NSDictionary *body = message.body;
        if ([body[@"interactiveRects"] isKindOfClass:[NSArray class]]) {
            NSMutableArray *rects=[NSMutableArray array];
            NSArray *items=body[@"interactiveRects"];
            if(items.count>32)return;
            for(id item in items){
                if(![item isKindOfClass:[NSDictionary class]])return;
                for(NSString *key in @[@"x",@"y",@"width",@"height"]) if(![item[key] isKindOfClass:[NSNumber class]])return;
                NSRect r=NSMakeRect([item[@"x"] doubleValue],[item[@"y"] doubleValue],[item[@"width"] doubleValue],[item[@"height"] doubleValue]);
                if(!isfinite(r.origin.x)||!isfinite(r.origin.y)||!isfinite(r.size.width)||!isfinite(r.size.height)||r.size.width<=0||r.size.height<=0)return;
                [rects addObject:[NSValue valueWithRect:r]];
            }
            [titlebarInteractiveRects release];titlebarInteractiveRects=[rects copy];
            return;
        }
        [titlebarTooltip close];
        if ([body[@"menu"] isKindOfClass:[NSString class]]) {
            if (titlebarMenuOpen) return;
            titlebarMenuOpen = YES;
            // NSMenu 會啟動自己的事件迴圈；不可在 WebKit 訊息回呼尚未
            // 返回時直接進入，避免 WebKit／AppKit 巢狀事件重入。
            NSDictionary *request = [body copy];
            dispatch_async(dispatch_get_main_queue(), ^{
                @try {
                    [self showMenu:request];
                } @catch (NSException *exception) {
                    NSLog(@"YourDesk 選單開啟失敗：%@ %@", exception.name, exception.reason);
                } @finally {
                    titlebarMenuOpen = NO;
                    if (pendingMenuAction) {
                        atomic_store(&titlebarAction, (int)pendingMenuAction);
                        pendingMenuAction = 0;
                    }
                }
            });
            [request release];
            return;
        }
        if (titlebarMenuOpen) return;
        NSString *text = body[@"tooltip"];
        if (![text isKindOfClass:[NSString class]] || text.length == 0) return;
        if (!titlebarTooltip) {
            titlebarTooltip = [NSPopover new];
            titlebarTooltip.behavior = NSPopoverBehaviorApplicationDefined;
            titlebarTooltip.animates = NO;
        }
        // 提示泡泡由 hover/blur/Escape 管理，不消耗用來操作按鈕的第一次點擊。
        titlebarTooltip.behavior = NSPopoverBehaviorApplicationDefined;
        titlebarTooltip.animates = NO;
        NSTextField *label = [NSTextField wrappingLabelWithString:text];
        label.font = [NSFont systemFontOfSize:12];
        label.frame = NSMakeRect(12, 9, 260, 40);
        [label sizeToFit];
        NSViewController *content = [NSViewController new];
        content.view = [[[NSView alloc] initWithFrame:NSMakeRect(0,0,label.frame.size.width+24,label.frame.size.height+18)] autorelease];
        [content.view addSubview:label];
        // 各種狀態提示共用兩欄排版，依列數計算高度。
        NSArray *rows = body[@"rows"];
        if ([rows isKindOfClass:[NSArray class]] && rows.count > 0 && rows.count <= 12) {
            NSMutableArray *labels = [NSMutableArray array], *values = [NSMutableArray array];
            CGFloat labelWidth = 0, valueWidth = 0;
            for (id row in rows) {
                if (![row isKindOfClass:[NSDictionary class]] || ![row[@"label"] isKindOfClass:[NSString class]] || ![row[@"value"] isKindOfClass:[NSString class]]) break;
                NSTextField *key = [NSTextField labelWithString:row[@"label"]];
                key.font = [NSFont systemFontOfSize:12 weight:NSFontWeightMedium];
                key.textColor = [NSColor secondaryLabelColor];
                NSTextField *value = [NSTextField labelWithString:row[@"value"]];
                value.font = [NSFont systemFontOfSize:12];
                [key sizeToFit]; [value sizeToFit];
                labelWidth = MAX(labelWidth,key.frame.size.width); valueWidth = MAX(valueWidth,value.frame.size.width);
                [labels addObject:key]; [values addObject:value];
            }
            if (labels.count == rows.count) {
                content.view = [[[NSView alloc] initWithFrame:NSMakeRect(0,0,32+labelWidth+16+valueWidth,18+rows.count*26)] autorelease];
                for (NSUInteger i=0;i<rows.count;i++) {
                    NSTextField *key=labels[i], *value=values[i];
                    key.frame=NSMakeRect(16,14+(rows.count-1-i)*26,labelWidth,18);
                    value.frame=NSMakeRect(32+labelWidth,14+(rows.count-1-i)*26,valueWidth,18);
                    [content.view addSubview:key]; [content.view addSubview:value];
                }
            }
        }
        titlebarTooltip.contentViewController = content;
        titlebarTooltip.contentSize = content.view.frame.size;
        [content release];
        CGFloat x = [body[@"x"] doubleValue], y = [body[@"y"] doubleValue];
        CGFloat width = [body[@"width"] doubleValue], height = [body[@"height"] doubleValue];
        NSRect anchor = NSMakeRect(x, titlebarWeb.isFlipped ? y : titlebarWeb.bounds.size.height-y-height, width, height);
        [titlebarTooltip showRelativeToRect:anchor ofView:titlebarWeb preferredEdge:NSMinYEdge];
        titlebarTooltip.contentViewController.view.window.ignoresMouseEvents = YES;
        return;
    }
    if (![message.body isKindOfClass:[NSNumber class]]) return;
    [titlebarTooltip close];
    int action = [message.body intValue];
    if (action == 7) [titlebarWeb.window miniaturize:nil];
    else if (action >= 100 && action < 104 && action-100 < atomic_load(&displayCount) && !atomic_load(&displayPending)) atomic_store(&titlebarAction, action);
    else if ((action >= 1 && action <= 6) || action==13) atomic_store(&titlebarAction, action);
    // 按下 HTML 按鈕後，鍵盤焦點交還遠端畫布。
    [titlebarWeb.window makeFirstResponder:titlebarWeb.window.contentView];
}
- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    titlebarLoaded = YES;
    ydUpdateTitlebar();
}
- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action decisionHandler:(void (^)(WKNavigationActionPolicy))handler {
    handler([action.request.URL.scheme isEqualToString:@"about"] ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
}
@end

// 透明原生拖曳區覆蓋 HTML 的彈性標題區；不攔截兩側按鈕。
// 拖曳必須使用當下的 NSEvent，不能等待非同步 JavaScript 回呼。
@interface YDTitlebarDragView : NSView
@end
@implementation YDTitlebarDragView
- (NSView *)hitTest:(NSPoint)point {
    NSPoint webPoint=[titlebarWeb convertPoint:point fromView:self.superview];
    if(!titlebarWeb.isFlipped)webPoint.y=titlebarWeb.bounds.size.height-webPoint.y;
    for(NSValue *value in titlebarInteractiveRects) {
        if(NSPointInRect(webPoint,NSInsetRect(value.rectValue,-2,-2)))return nil;
    }
    return [super hitTest:point];
}
- (BOOL)acceptsFirstMouse:(NSEvent *)event { return YES; }
- (void)mouseDown:(NSEvent *)event {
    if (event.clickCount == 2) atomic_store(&fullscreenRequested, true);
    else if (!atomic_load(&fullscreenActive)) [self.window performWindowDragWithEvent:event];
}
@end

// 非作用中視窗的第一次點擊亦可直接操作工具列。
@interface YDTitlebarWebView : WKWebView
@end
@implementation YDTitlebarWebView
- (BOOL)acceptsFirstMouse:(NSEvent *)event { return YES; }
@end

static void ydUpdateTitlebarVisibility(void);

void yd_configure_titlebar(const char *html) {
    if (atomic_load(&titlebarConfigured) || atomic_exchange(&titlebarConfigurePending, true)) return;
    NSString *page = [[NSString alloc] initWithUTF8String:html];
    dispatch_async(dispatch_get_main_queue(), ^{
        atomic_store(&titlebarConfigurePending, false);
        if (atomic_load(&titlebarConfigured)) return;
        for (NSWindow *window in NSApp.windows) {
            if (!ydSupportsTitlebarControls(window)) continue;
            // 內容區延伸到整個視窗；HTML 自己佔用頂端 52pt，避免原生
            // 標題列配件的高度裁切與額外空白列。
            window.styleMask |= NSWindowStyleMaskFullSizeContentView;
            window.collectionBehavior = (window.collectionBehavior & ~(NSWindowCollectionBehaviorFullScreenPrimary | NSWindowCollectionBehaviorFullScreenAuxiliary)) | NSWindowCollectionBehaviorFullScreenNone;
            ydHideNativeTitlebar(window);
            NSView *contentView = window.contentView;
            CGFloat width = contentView.bounds.size.width;
            NSView *controls = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, width, 52)];
            titlebarControls = controls;
            controls.identifier = @"YourDesk.TitlebarControls";
            controls.translatesAutoresizingMaskIntoConstraints = NO;
            WKWebViewConfiguration *config = [WKWebViewConfiguration new];
            config.websiteDataStore = [WKWebsiteDataStore nonPersistentDataStore];
            YDTitlebarBridge *bridge = [YDTitlebarBridge new];
            [config.userContentController addScriptMessageHandler:bridge name:@"titlebar"];
            titlebarWeb = [[YDTitlebarWebView alloc] initWithFrame:controls.bounds configuration:config];
            titlebarWeb.navigationDelegate = bridge;
            titlebarWeb.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
            [controls addSubview:titlebarWeb];
            YDTitlebarDragView *drag = [[YDTitlebarDragView alloc] initWithFrame:NSMakeRect(88, 0, MAX(0, width-260), 52)];
            drag.autoresizingMask = NSViewWidthSizable;
            [controls addSubview:drag];
            [contentView addSubview:controls positioned:NSWindowAbove relativeTo:nil];
            [NSLayoutConstraint activateConstraints:@[
                [controls.topAnchor constraintEqualToAnchor:contentView.topAnchor],
                [controls.leadingAnchor constraintEqualToAnchor:contentView.leadingAnchor],
                [controls.trailingAnchor constraintEqualToAnchor:contentView.trailingAnchor],
                [controls.heightAnchor constraintEqualToConstant:52]
            ]];
            [contentView layoutSubtreeIfNeeded];
            [titlebarWeb loadHTMLString:page baseURL:nil];
            [drag release]; [controls release]; [config release]; [bridge release];
            atomic_store(&titlebarConfigured, true);
            NSTimer *timer = [NSTimer timerWithTimeInterval:0.05 repeats:YES block:^(NSTimer *timer) {
                ydUpdateTitlebarVisibility();
            }];
            [[NSRunLoop mainRunLoop] addTimer:timer forMode:NSRunLoopCommonModes];
            return;
        }
    });
    [page release];
}

int yd_titlebar_action(void) { return atomic_exchange(&titlebarAction, 0); }
void yd_set_titlebar_mode(int mode) {
    if (atomic_exchange(&titlebarMode, mode) != mode) dispatch_async(dispatch_get_main_queue(), ^{ ydUpdateTitlebar(); });
}
void yd_set_displays(int index, int count, int pending) {
    int previousIndex = atomic_exchange(&displayIndex, index);
    int previousCount = atomic_exchange(&displayCount, count);
    bool previousPending = atomic_exchange(&displayPending, pending != 0);
    if (previousIndex != index || previousCount != count || previousPending != (pending != 0)) {
        dispatch_async(dispatch_get_main_queue(), ^{ ydUpdateTitlebar(); });
    }
}

// HTML 空白區的雙擊與鍵盤切換都交由遊戲執行緒執行。
int yd_fullscreen_requested(void) {
    return atomic_exchange(&fullscreenRequested, false);
}

void yd_set_traffic(double tx, double rx, double fps, int render) {
    dispatch_async(dispatch_get_main_queue(), ^{
        trafficTX = tx;
        trafficRX = rx;
        viewerFPS = fps;
        renderFPS = render != 0;
        ydUpdateTitlebar();
    });
}

void yd_set_quality(int quality) { atomic_store(&qualityMode,quality); }

// 僅調整既有視窗的樣式與範圍，不呼叫原生全螢幕或切換 Space。
void yd_toggle_fullscreen(void) {
    if (atomic_exchange(&fullscreenTransitioning, true)) return;
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *window = titlebarWeb.window;
        if (!ydSupportsTitlebarControls(window)) { atomic_store(&fullscreenTransitioning, false); return; }
        [titlebarTooltip close];
        BOOL entering = !atomic_load(&fullscreenActive);
        @try {
            if (entering) {
                NSScreen *screen = window.screen ?: NSScreen.mainScreen;
                if (!screen) return;
                windowedFrame = window.frame;
                windowedStyle = window.styleMask;
                windowedCollection = window.collectionBehavior;
                windowedLevel = window.level;
                windowedPresentation = NSApp.presentationOptions;
                window.styleMask = NSWindowStyleMaskBorderless;
                [window setFrame:screen.frame display:NO animate:NO];
                atomic_store(&fullscreenActive, true);
                atomic_store(&titlebarVisible, false);
                titlebarControls.hidden = YES;
            } else {
                if (fullscreenPresentationApplied) {
                    NSApp.presentationOptions = windowedPresentation;
                    fullscreenPresentationApplied = NO;
                }
                window.level = windowedLevel;
                window.styleMask = windowedStyle;
                ydHideNativeTitlebar(window);
                window.collectionBehavior = windowedCollection;
                NSRect frame = windowedFrame;
                BOOL onScreen = NO;
                for (NSScreen *screen in NSScreen.screens) {
                    if (NSIntersectsRect(frame, screen.visibleFrame)) { onScreen = YES; break; }
                }
                if (!onScreen && NSScreen.mainScreen) {
                    NSRect visible = NSScreen.mainScreen.visibleFrame;
                    frame.size.width = MIN(frame.size.width, visible.size.width);
                    frame.size.height = MIN(frame.size.height, visible.size.height);
                    frame.origin = NSMakePoint(NSMidX(visible)-frame.size.width/2, NSMidY(visible)-frame.size.height/2);
                }
                [window setFrame:frame display:NO animate:NO];
                atomic_store(&fullscreenActive, false);
                atomic_store(&titlebarVisible, true);
                titlebarControls.hidden = NO;
            }
            edgeHoverStarted = titlebarLeaveStarted = 0;
            [window makeKeyAndOrderFront:nil];
            [window makeFirstResponder:window.contentView];
            ydUpdateTitlebarVisibility();
        } @catch (NSException *exception) {
            NSLog(@"YourDesk 視窗式全螢幕切換失敗：%@", exception.reason);
        } @finally {
            atomic_store(&fullscreenTransitioning, false);
        }
    });
}
int yd_fullscreen_transitioning(void) { return atomic_load(&fullscreenTransitioning); }

static void ydUpdateTitlebarVisibility(void) {
    NSWindow *window = titlebarWeb.window;
    if (!window) return;
    BOOL fullscreen = atomic_load(&fullscreenActive);
    // 僅在本視窗取得焦點時覆蓋系統列，切到其他 App 即還原。
    BOOL presentation = fullscreen && window.keyWindow && NSApp.active;
    if (presentation != fullscreenPresentationApplied) {
        NSApp.presentationOptions = presentation ? (NSApplicationPresentationAutoHideDock | NSApplicationPresentationAutoHideMenuBar) : windowedPresentation;
        window.level = presentation ? NSMainMenuWindowLevel+1 : windowedLevel;
        fullscreenPresentationApplied = presentation;
    }
    BOOL show = atomic_load(&titlebarVisible);
    if (!fullscreen) {
        show = YES; edgeHoverStarted = titlebarLeaveStarted = 0;
    } else {
        NSView *content = window.contentView;
        NSPoint mouse = [content convertPoint:[window convertPointFromScreen:NSEvent.mouseLocation] fromView:nil];
        CGFloat fromTop = content.isFlipped ? mouse.y-NSMinY(content.bounds) : NSMaxY(content.bounds)-mouse.y;
        BOOL inside = window.keyWindow && mouse.x >= NSMinX(content.bounds) && mouse.x <= NSMaxX(content.bounds) && fromTop >= 0;
        NSTimeInterval now = NSProcessInfo.processInfo.systemUptime;
        if (!show) {
            if (inside && fromTop <= 6) {
                if (!edgeHoverStarted) edgeHoverStarted = now;
                if (now-edgeHoverStarted >= 1.0) show = YES;
            } else edgeHoverStarted = 0;
        } else if (titlebarMenuOpen || (inside && fromTop <= 64)) {
            titlebarLeaveStarted = 0;
        } else {
            if (!titlebarLeaveStarted) titlebarLeaveStarted = now;
            if (now-titlebarLeaveStarted >= 0.25) {
                show = NO; edgeHoverStarted = titlebarLeaveStarted = 0;
            }
        }
    }
    if (atomic_exchange(&titlebarVisible, show) != show) {
        titlebarControls.hidden = !show;
        if (!show) [titlebarTooltip close];
    }
}
int yd_titlebar_overlay(void) { return atomic_load(&fullscreenActive); }
int yd_titlebar_visible(void) { return atomic_load(&titlebarVisible); }

void yd_set_language(const char *dictionary, const char *language) {
    NSString *json = [[NSString alloc] initWithUTF8String:dictionary];
    NSString *requested = [[NSString alloc] initWithUTF8String:language];
    dispatch_async(dispatch_get_main_queue(), ^{
        NSString *locale = requested;
        if ([locale isEqualToString:@"auto"]) {
            locale = @"en";
            for (NSString *candidate in NSLocale.preferredLanguages) {
                NSString *prefix = [[candidate componentsSeparatedByString:@"-"] firstObject];
                if ([@[@"zh",@"en",@"ja",@"ko"] containsObject:prefix]) { locale = prefix; break; }
            }
        }
        NSInteger column = [locale hasPrefix:@"ja"] ? 1 : [locale hasPrefix:@"ko"] ? 2 : 0;
        NSDictionary *all = [NSJSONSerialization JSONObjectWithData:[json dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
        NSMutableDictionary *selected = [NSMutableDictionary dictionary];
        if (![locale hasPrefix:@"zh"]) {
            for (NSString *key in all) {
                NSArray *values = all[key];
                if ([values isKindOfClass:NSArray.class] && values.count > column) selected[key] = values[column];
            }
        }
        [titlebarStrings release]; titlebarStrings = [selected copy];
        [titlebarLanguage release]; titlebarLanguage = [([locale hasPrefix:@"zh"] ? @"zh-Hant" : locale) copy];
        ydUpdateTitlebar();
    });
    [json release]; [requested release];
}

void yd_set_codec_status(const char *codec,const char *source,const char *receiver){
 NSString *c=[[NSString alloc] initWithUTF8String:codec],*s=[[NSString alloc] initWithUTF8String:source],*r=[[NSString alloc] initWithUTF8String:receiver];
 dispatch_async(dispatch_get_main_queue(), ^{
  if(![videoCodec isEqualToString:c]||![sourceEncoding isEqualToString:s]||![receiverDecoding isEqualToString:r]){
   [videoCodec release];videoCodec=[c copy];[sourceEncoding release];sourceEncoding=[s copy];[receiverDecoding release];receiverDecoding=[r copy];ydUpdateTitlebar();
  }
 });
 [c release];[s release];[r release];
}

void yd_show_close_confirmation(void) {
 dispatch_async(dispatch_get_main_queue(), ^{
  if (titlebarLoaded && titlebarWeb.window.visible)
   [titlebarWeb evaluateJavaScript:@"window.showCloseConfirmation()" completionHandler:nil];
 });
}

void yd_set_enhancement_status(const char *json) {
 NSString *text=[[NSString alloc] initWithUTF8String:json];
 dispatch_async(dispatch_get_main_queue(), ^{
  id value=[NSJSONSerialization JSONObjectWithData:[text dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
  if([value isKindOfClass:[NSDictionary class]] && ![enhancementStatus isEqual:value]) {
   [enhancementStatus release]; enhancementStatus=[value copy]; ydUpdateTitlebar();
  }
 });
 [text release];
}
