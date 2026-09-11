//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Cocoa -framework WebKit
int yd_fullscreen_requested(void);
void yd_toggle_fullscreen(void);
int yd_fullscreen_transitioning(void);
int yd_titlebar_overlay(void);
int yd_titlebar_visible(void);
void yd_configure_titlebar(const char *html);
void yd_set_displays(int index, int count, int pending);
#include <stdlib.h>
int yd_titlebar_action(void);
void yd_set_titlebar_mode(int mode);
void yd_set_codec_status(const char *codec,const char *source,const char *receiver);
void yd_set_traffic(double tx, double rx, double fps, int render);
void yd_set_quality(int quality);
void yd_set_enhancement_status(const char *json);
void yd_show_close_confirmation(void);
void yd_set_language(const char *dictionary, const char *language);
*/
import "C"
import (
	"encoding/json"
	"unsafe"
)

var titlebarPage = C.CString(viewerTitlebarHTML())

func nativeFullscreenRequested() bool { return C.yd_fullscreen_requested() != 0 }

func nativeToggleFullscreen() bool        { C.yd_toggle_fullscreen(); return true }
func nativeFullscreenTransitioning() bool { return C.yd_fullscreen_transitioning() != 0 }

func nativeRestoresWindowFrame() bool { return true }

func nativeTitlebarControls() bool { return true }

func nativeConfigureTitlebar() { C.yd_configure_titlebar(titlebarPage) }

func nativeTitlebarAction() int { return int(C.yd_titlebar_action()) }

func nativeSetTitlebarMode(mode int) { C.yd_set_titlebar_mode(C.int(mode)) }

func nativeSetDisplays(index, count int, pending bool) {
	p := 0
	if pending {
		p = 1
	}
	C.yd_set_displays(C.int(index), C.int(count), C.int(p))
}
func nativeCloseTitlebar() { C.free(unsafe.Pointer(titlebarPage)) }

func nativeSetTraffic(tx, rx, fps float64, render bool) {
	flag := C.int(0)
	if render {
		flag = 1
	}
	C.yd_set_traffic(C.double(tx), C.double(rx), C.double(fps), flag)
}

func nativeSetQuality(quality int) { C.yd_set_quality(C.int(quality)) }

func nativeTitlebarOverlay() bool { return C.yd_titlebar_overlay() != 0 }
func nativeTitlebarVisible() bool { return C.yd_titlebar_visible() != 0 }

func nativeSetLanguage(dictionary, language string) {
	data, lang := C.CString(dictionary), C.CString(language)
	defer C.free(unsafe.Pointer(data))
	defer C.free(unsafe.Pointer(lang))
	C.yd_set_language(data, lang)
}

func nativeSetCodecStatus(codec, source, receiver string) {
	c, s, r := C.CString(codec), C.CString(source), C.CString(receiver)
	defer C.free(unsafe.Pointer(c))
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(r))
	C.yd_set_codec_status(c, s, r)
}

func nativeShowCloseConfirmation() { C.yd_show_close_confirmation() }

func nativeTitlebarPopupOpen() bool { return false }

func nativePrepareViewerWindow() {}

func nativeSetEnhancementStatus(status enhancementDisplayStatus) {
	data, err := json.Marshal(status)
	if err != nil {
		return
	}
	value := C.CString(string(data))
	defer C.free(unsafe.Pointer(value))
	C.yd_set_enhancement_status(value)
}
