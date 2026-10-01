package com.yourdesk.android;

public final class DesktopViewportTest {
  private static void near(float expected, float actual) {
    if (Math.abs(expected - actual) > .001f) throw new AssertionError(expected + " != " + actual);
  }
  public static void main(String[] args) {
    DesktopViewport view = new DesktopViewport();
    DesktopViewport.Geometry fit = view.geometry(1920, 1080, 1200, 600);
    near(66.6667f, fit.left); near(0, fit.top);
    if (fit.mapTouch(20, 300) != null || fit.mapTouch(1200, 300) != null) throw new AssertionError("黑邊／越界不可控制遠端");
    near(.5f, fit.mapTouch(600, 300)[0]);

    // 同時改變兩指距離與重心，來源中的焦點必須仍在手指下方。
    view.transform(1920, 1080, 1200, 600, 2, 400, 250, 480, 280);
    DesktopViewport.Geometry zoom = view.geometry(1920, 1080, 1200, 600);
    near(2, view.zoom());
    near(fit.mapTouch(400, 250)[0], zoom.mapTouch(480, 280)[0]);
    near(fit.mapTouch(400, 250)[1], zoom.mapTouch(480, 280)[1]);

    view.transform(1920, 1080, 1200, 600, 1, 0, 0, 10000, 10000);
    zoom = view.geometry(1920, 1080, 1200, 600);
    near(0, zoom.left); near(0, zoom.top);
    view.transform(1920, 1080, 1200, 600, 1, 0, 0, -10000, -10000);
    zoom = view.geometry(1920, 1080, 1200, 600);
    near(1200, zoom.left + zoom.width); near(600, zoom.top + zoom.height);

    // 來源解析度改變不應重設放大倍率；旋轉仍保持比例且不產生額外空白。
    DesktopViewport.Geometry resized = view.geometry(3840, 2160, 1200, 600);
    near(zoom.left, resized.left); near(zoom.width, resized.width);
    DesktopViewport.Geometry portrait = view.geometry(1920, 1080, 600, 1200);
    near(1920f / 1080, portrait.width / portrait.height);
    near((1200 - portrait.height) * .5f, portrait.top);
    if (portrait.left > 0 || portrait.left + portrait.width < 600) throw new AssertionError("平移不可露出左右空白");

    view.transform(1920, 1080, 1200, 600, 100, 600, 300, 600, 300);
    near(8, view.zoom());
    view.transform(1920, 1080, 1200, 600, Float.NaN, 0, 0, 0, 0);
    near(8, view.zoom());
    view.transform(1920, 1080, 1200, 600, .0001f, 600, 300, 600, 300);
    near(1, view.zoom()); near(fit.left, view.geometry(1920, 1080, 1200, 600).left);
    view.reset(); near(1, view.zoom());
    System.out.println("DesktopViewport：焦點縮放、四邊平移、座標反算、解析度／方向切換及倍率界限 PASS");
  }
}
