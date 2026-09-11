//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
static void yd_agent_hide_window(void) {
 dispatch_async(dispatch_get_main_queue(), ^{
 for(NSWindow *w in NSApp.windows){if([NSStringFromClass(w.class) containsString:@"GLFW"])[w orderOut:nil];}
 });
}
static void yd_agent_show_window(void) {
 dispatch_async(dispatch_get_main_queue(), ^{
  [NSApp activateIgnoringOtherApps:YES];
  for(NSWindow *w in NSApp.windows){if([NSStringFromClass(w.class) containsString:@"GLFW"]){[w deminiaturize:nil];[w makeKeyAndOrderFront:nil];}}
 });
}
*/
import "C"

func nativeHideAgentWindow() { C.yd_agent_hide_window() }
func nativeShowAgentWindow() { C.yd_agent_show_window() }
