package com.yourdesk.android;

/** JPEG／影片共用的可視區：1 倍完整置中，以來源座標保存中心，縮放與輸入採同一轉換。 */
final class DesktopViewport {
  static final float MAX_ZOOM = 8f;
  private float zoom = 1f, centerX = .5f, centerY = .5f;

  float zoom() { return zoom; }
  void reset() { zoom = 1f; centerX = centerY = .5f; }

  Geometry geometry(int sourceWidth, int sourceHeight, int viewWidth, int viewHeight) {
    if (sourceWidth <= 0 || sourceHeight <= 0 || viewWidth <= 0 || viewHeight <= 0) return null;
    float fit = Math.min(viewWidth / (float) sourceWidth, viewHeight / (float) sourceHeight);
    float width = sourceWidth * fit * zoom, height = sourceHeight * fit * zoom;
    float x = boundedCenter(centerX, width, viewWidth), y = boundedCenter(centerY, height, viewHeight);
    return new Geometry(viewWidth * .5f - x * width, viewHeight * .5f - y * height,
        width, height, viewWidth, viewHeight);
  }

  void transform(int sourceWidth, int sourceHeight, int viewWidth, int viewHeight,
      float factor, float fromX, float fromY, float toX, float toY) {
    Geometry before = geometry(sourceWidth, sourceHeight, viewWidth, viewHeight);
    if (before == null || !Float.isFinite(factor) || factor <= 0 || !Float.isFinite(fromX)
        || !Float.isFinite(fromY) || !Float.isFinite(toX) || !Float.isFinite(toY)) return;
    float anchorX = (fromX - before.left) / before.width;
    float anchorY = (fromY - before.top) / before.height;
    float next = Math.max(1f, Math.min(MAX_ZOOM, zoom * factor));
    float width = before.width * next / zoom, height = before.height * next / zoom;
    centerX = boundedCenter(anchorX + (viewWidth * .5f - toX) / width, width, viewWidth);
    centerY = boundedCenter(anchorY + (viewHeight * .5f - toY) / height, height, viewHeight);
    zoom = next;
  }

  private static float boundedCenter(float value, float content, float viewport) {
    if (content <= viewport) return .5f;
    float half = viewport / (2f * content);
    return Math.max(half, Math.min(1f - half, value));
  }

  static final class Geometry {
    final float left, top, width, height;
    final int viewWidth, viewHeight;
    Geometry(float left, float top, float width, float height, int viewWidth, int viewHeight) {
      this.left = left; this.top = top; this.width = width; this.height = height;
      this.viewWidth = viewWidth; this.viewHeight = viewHeight;
    }
    float[] mapTouch(float x, float y) {
      if (!Float.isFinite(x) || !Float.isFinite(y) || x < 0 || y < 0 || x >= viewWidth || y >= viewHeight
          || x < left || y < top || x >= left + width || y >= top + height) return null;
      return new float[]{Math.max(0f, Math.min(1f, (x - left) / width)),
          Math.max(0f, Math.min(1f, (y - top) / height))};
    }
  }
}
