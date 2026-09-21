//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#include <math.h>

// A native surface owns the actual mouse gesture. The web page never receives
// local file URLs and cannot initiate a synthetic operating-system drag.
@interface YDFileDragView : NSView <NSDraggingSource>
@property(nonatomic, copy) NSArray<NSURL *> *files;
@property(nonatomic, assign) NSPoint mouseOrigin;
@property(nonatomic, assign) BOOL pressed;
@property(nonatomic, assign) BOOL dragging;
@property(nonatomic, assign) BOOL invalid;
@end

@implementation YDFileDragView
- (BOOL)isOpaque { return YES; }
- (BOOL)acceptsFirstMouse:(NSEvent *)event { return YES; }
- (void)resetCursorRects {
    if (self.files.count && !self.invalid) [self addCursorRect:self.bounds cursor:NSCursor.openHandCursor];
}
- (void)drawRect:(NSRect)dirty {
    [NSColor.controlBackgroundColor setFill];
    NSRectFill(self.bounds);
    [NSColor.separatorColor setFill];
    NSRectFill(NSMakeRect(0, NSHeight(self.bounds)-1, NSWidth(self.bounds), 1));
    NSString *title = self.files.count
        ? [NSString stringWithFormat:@"拖曳已備妥的 %lu 個檔案到 Finder", (unsigned long)self.files.count]
        : @"先下載檔案，再從這裡拖到 Finder";
    NSMutableParagraphStyle *style = [[[NSMutableParagraphStyle alloc] init] autorelease];
    style.alignment = NSTextAlignmentCenter;
    style.lineBreakMode = NSLineBreakByTruncatingTail;
    [title drawInRect:NSMakeRect(16, 35, MAX(0, NSWidth(self.bounds)-32), 22)
        withAttributes:@{NSFontAttributeName:[NSFont systemFontOfSize:14 weight:NSFontWeightMedium],
            NSForegroundColorAttributeName:self.files.count ? NSColor.labelColor : NSColor.secondaryLabelColor,
            NSParagraphStyleAttributeName:style}];
    [@"Drag prepared files to Finder · Copy only" drawInRect:NSMakeRect(16, 13, MAX(0, NSWidth(self.bounds)-32), 18)
        withAttributes:@{NSFontAttributeName:[NSFont systemFontOfSize:12],
            NSForegroundColorAttributeName:NSColor.secondaryLabelColor, NSParagraphStyleAttributeName:style}];
}
- (void)mouseDown:(NSEvent *)event {
    self.pressed = !self.invalid && !self.dragging && self.files.count > 0;
    self.mouseOrigin = [self convertPoint:event.locationInWindow fromView:nil];
}
- (void)mouseUp:(NSEvent *)event { self.pressed = NO; }
- (void)mouseDragged:(NSEvent *)event {
    if (!self.pressed || self.invalid || self.dragging || !self.files.count) return;
    NSPoint point = [self convertPoint:event.locationInWindow fromView:nil];
    if (hypot(point.x-self.mouseOrigin.x, point.y-self.mouseOrigin.y) < 4) return;
    self.pressed = NO;
    NSMutableArray<NSDraggingItem *> *items = [NSMutableArray arrayWithCapacity:self.files.count];
    for (NSURL *url in self.files) {
        BOOL directory = NO;
        if (![NSFileManager.defaultManager fileExistsAtPath:url.path isDirectory:&directory] || directory) return;
        NSDraggingItem *item = [[[NSDraggingItem alloc] initWithPasteboardWriter:url] autorelease];
        NSImage *icon = [NSWorkspace.sharedWorkspace iconForFile:url.path];
        [item setDraggingFrame:NSMakeRect(point.x-16, point.y-16, 32, 32) contents:icon];
        [items addObject:item];
    }
    self.dragging = YES;
    // Window teardown can happen during the native drag loop. Keep the source
    // alive until AppKit reports completion, without retaining any Go callbacks.
    [self retain];
    NSDraggingSession *session = [self beginDraggingSessionWithItems:items event:event source:self];
    if (!session) { self.dragging = NO; [self release]; return; }
    session.animatesToStartingPositionsOnCancelOrFail = YES;
    session.draggingFormation = NSDraggingFormationStack;
}
- (NSDragOperation)draggingSession:(NSDraggingSession *)session sourceOperationMaskForDraggingContext:(NSDraggingContext)context {
    return NSDragOperationCopy;
}
- (BOOL)ignoreModifierKeysForDraggingSession:(NSDraggingSession *)session { return YES; }
- (void)draggingSession:(NSDraggingSession *)session endedAtPoint:(NSPoint)point operation:(NSDragOperation)operation {
    self.dragging = NO;
    self.pressed = NO;
    [self release];
}
- (void)dealloc { [_files release]; [super dealloc]; }
@end

void *yd_file_drag_install(void *nativeWindow) {
    @autoreleasepool {
        if (![NSThread isMainThread] || !nativeWindow) return NULL;
        NSWindow *window = (NSWindow *)nativeWindow;
        NSView *content = window.contentView;
        if (!content) return NULL;
        CGFloat y = content.isFlipped ? MAX(0, NSHeight(content.bounds)-72) : 0;
        YDFileDragView *view = [[YDFileDragView alloc] initWithFrame:NSMakeRect(0, y, NSWidth(content.bounds), 72)];
        view.autoresizingMask = NSViewWidthSizable | (content.isFlipped ? NSViewMinYMargin : NSViewMaxYMargin);
        view.files = @[];
        [view setAccessibilityElement:YES];
        [view setAccessibilityRole:NSAccessibilityGroupRole];
        [view setAccessibilityLabel:@"已下載檔案的 Finder 拖曳區 / Drag downloaded files to Finder"];
        [content addSubview:view positioned:NSWindowAbove relativeTo:nil];
        return view;
    }
}

int yd_file_drag_set(void *nativeView, const char **paths, int count) {
    @autoreleasepool {
        if (![NSThread isMainThread] || !nativeView || count < 0 || count > 64 || (count && !paths)) return 0;
        YDFileDragView *view = (YDFileDragView *)nativeView;
        if (view.invalid || !view.window) return 0;
        NSMutableArray<NSURL *> *files = [NSMutableArray arrayWithCapacity:count];
        for (int i = 0; i < count; ++i) {
            if (!paths[i]) return 0;
            NSString *path = [NSString stringWithUTF8String:paths[i]];
            if (!path.isAbsolutePath) return 0;
            [files addObject:[NSURL fileURLWithPath:path isDirectory:NO]];
        }
        view.files = files;
        view.pressed = NO;
        [view setNeedsDisplay:YES];
        [view.window invalidateCursorRectsForView:view];
        return 1;
    }
}

void yd_file_drag_remove(void *nativeView) {
    @autoreleasepool {
        if (!nativeView) return;
        YDFileDragView *view = (YDFileDragView *)nativeView;
        view.invalid = YES;
        view.pressed = NO;
        view.files = @[];
        [view removeFromSuperview];
        [view release];
    }
}
