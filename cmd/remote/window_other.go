//go:build (!darwin && !windows) || !cgo

package main

func nativeFullscreenRequested() bool { return false }

func nativeRestoresWindowFrame() bool { return false }

func nativeTitlebarControls() bool { return false }

func nativeConfigureTitlebar() {}

func nativeTitlebarAction() int { return 0 }

func nativeSetTitlebarMode(int) {}

func nativeSetDisplays(int, int, bool) {}
func nativeCloseTitlebar()             {}

func nativeSetTraffic(float64, float64, float64, bool) {}

func nativeSetQuality(int) {}

func nativeToggleFullscreen() bool        { return false }
func nativeFullscreenTransitioning() bool { return false }

func nativeTitlebarOverlay() bool { return false }
func nativeTitlebarVisible() bool { return false }

func nativeSetLanguage(string, string) {}

func nativeSetCodecStatus(string, string, string) {}

func nativeShowCloseConfirmation() {}

func nativeTitlebarPopupOpen() bool { return false }

func nativePrepareViewerWindow() {}

func nativeSetEnhancementStatus(enhancementDisplayStatus) {}
