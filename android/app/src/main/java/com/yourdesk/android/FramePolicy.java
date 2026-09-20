package com.yourdesk.android;

/** Pure state/size policy shared by the renderer and host-side regression tests. */
final class FramePolicy {
  static final int MAX_ENCODED_BYTES = 32 * 1024 * 1024;
  // Preserve the existing 6K-capable envelope; validate the JPEG header before allocation.
  static final long MAX_PIXELS = 32L * 1024 * 1024;
  private boolean valid;
  private long sequence;
  private int width, height, display;

  static boolean dimensions(int width, int height) {
    return width > 0 && height > 0 && width <= 8192 && height <= 8192
        && (long) width * height <= MAX_PIXELS;
  }

  static boolean patch(int fullWidth, int fullHeight, int x, int y,
      int patchWidth, int patchHeight, boolean keyframe) {
    return dimensions(fullWidth, fullHeight) && dimensions(patchWidth, patchHeight)
        && x >= 0 && y >= 0 && x < fullWidth && y < fullHeight
        && patchWidth <= fullWidth - x && patchHeight <= fullHeight - y
        && (!keyframe || (x == 0 && y == 0 && patchWidth == fullWidth && patchHeight == fullHeight));
  }

  boolean accept(long next, int w, int h, int d, boolean keyframe) {
    if (next <= 0 || next <= sequence || !dimensions(w, h)) return false;
    if (keyframe) return true;
    if (!valid || next != sequence + 1 || width != w || height != h || display != d) {
      valid = false;
      return false;
    }
    return true;
  }

  void commit(long next, int w, int h, int d) {
    sequence = next; width = w; height = h; display = d; valid = true;
  }

  void invalidate() { valid = false; }
  boolean needsKeyframe() { return !valid; }
  void reset() { valid = false; sequence = 0; width = height = display = 0; }
}
