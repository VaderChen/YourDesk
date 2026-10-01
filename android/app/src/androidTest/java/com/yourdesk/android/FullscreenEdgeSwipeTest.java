package com.yourdesk.android;

import static org.junit.Assert.*;

import android.app.Instrumentation;
import android.content.Intent;
import android.content.pm.ActivityInfo;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.graphics.Rect;
import android.os.SystemClock;
import android.util.Base64;
import android.view.InputDevice;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.TextureView;
import android.webkit.WebView;
import android.widget.Button;
import android.widget.FrameLayout;

import com.yourdesk.android.video.EncodedVideoFrame;

import androidx.core.view.ViewCompat;
import androidx.core.view.WindowInsetsCompat;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;

import org.json.JSONObject;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;
import org.junit.runner.RunWith;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.atomic.AtomicReference;
import java.util.function.BooleanSupplier;
import java.util.function.Supplier;

/** 使用實際 Activity、JPEG 呈現器與系統手勢；測試影像不需要遠端站台憑證。 */
@RunWith(AndroidJUnit4.class)
public class FullscreenEdgeSwipeTest {
  private final Instrumentation instrumentation = InstrumentationRegistry.getInstrumentation();
  private MainActivity activity;
  private final List<Integer> remoteEvents = new ArrayList<>();
  private Object originalViewer;
  private Object originalFrame;
  private Rect originalBounds;
  private int originalBars;
  private String localFrame;

  @Test public void nativePointerRejectsOutsideAndReentry() throws Exception {
    ui(() -> {
      View screen = (View) field("nativeScreen");
      float x = screen.getWidth() / 2f, y = screen.getHeight() / 2f;
      long now = SystemClock.uptimeMillis();
      int[] actions = {MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE, MotionEvent.ACTION_MOVE, MotionEvent.ACTION_UP};
      float[] xs = {x, -20, x, x};
      for (int i = 0; i < actions.length; i++) {
        MotionEvent event = MotionEvent.obtain(now, now + i, actions[i], xs[i], y, 0);
        try {
          invoke("handleDesktopTouch", new Class[]{View.class, MotionEvent.class}, screen, event);
          if (i == 0) assertSame("有效按下取得手勢", screen, field("desktopTouchOwner"));
          else assertNull("越界後不得重新取得手勢", field("desktopTouchOwner"));
        } finally { event.recycle(); }
      }
      MainActivity.Bridge bridge = activity.new Bridge();
      for (String type : new String[]{"move", "button", "wheel"}) {
        assertEquals("背景 WebView 不得送出滑鼠事件", "native pointer only",
            bridge.sendControlJSON("{\"type\":\"" + type + "\"}"));
      }
      return null;
    });
  }

  @Before public void showLocalDesktop() throws Exception {
    Intent intent = new Intent(instrumentation.getTargetContext(), MainActivity.class)
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
    activity = (MainActivity) instrumentation.startActivitySync(intent);
    Bitmap bitmap = Bitmap.createBitmap(1920, 1080, Bitmap.Config.ARGB_8888);
    Canvas canvas = new Canvas(bitmap);
    canvas.drawColor(Color.rgb(24, 52, 66));
    Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
    paint.setColor(Color.rgb(102, 220, 170));
    paint.setStyle(Paint.Style.STROKE);
    paint.setStrokeWidth(16);
    canvas.drawRect(8, 8, 1912, 1072, paint);
    paint.setStyle(Paint.Style.FILL);
    paint.setColor(Color.WHITE);
    paint.setTextSize(60);
    canvas.drawText("本機手勢 Smoke 測試（測試影像）", 160, 420, paint);
    paint.setTextSize(36);
    canvas.drawText("左側邊緣右滑 → 恢復原始介面，保持桌面連線", 160, 510, paint);
    ByteArrayOutputStream bytes = new ByteArrayOutputStream();
    bitmap.compress(Bitmap.CompressFormat.JPEG, 90, bytes);
    bitmap.recycle();
    localFrame = new JSONObject().put("sequence", 1).put("display", 0)
        .put("codec", 0).put("width", 1920).put("height", 1080).put("keyframe", true)
        .put("data", Base64.encodeToString(bytes.toByteArray(), Base64.NO_WRAP)).toString();
    ui(() -> {
      setField("inDesktop", true);
      assertEquals(true, invoke("applyFrameJSON", new Class[]{String.class}, localFrame));
      originalViewer = field("viewer");
      originalFrame = field("composedFrame");
      invoke("showDesktopPage", new Class[]{originalViewer.getClass(), int.class},
          originalViewer, field("generation"));
      // 記錄呈現器收到的操作，確認退出手勢不會進入傳送遠端控制的路徑。
      for (String name : new String[]{"nativeScreen", "nativeVideo"}) {
        ((View) field(name)).setOnTouchListener((v, event) -> {
          remoteEvents.add(event.getActionMasked());
          return true;
        });
      }
      return null;
    });
    await("桌面頁面載入", () -> ui(() -> {
      WebView web = (WebView) field("web");
      return web.getProgress() == 100 && web.getUrl().endsWith("/desktop.html")
          && ((View) field("nativeScreen")).getTop() > 0;
    }));
    originalBounds = ui(this::screenBounds);
    originalBars = ui(this::visibleBars);
  }

  @After public void finishLocalDesktop() {
    if (activity != null) {
      ui(() -> { activity.finish(); return null; });
      instrumentation.waitForIdleSync();
    }
  }

  @Test public void leftSwipeRestoresLayoutWithoutRemoteInput() throws Exception {
    screenshot("01-normal.png");
    enterFullscreen();
    screenshot("02-fullscreen.png");
    directGesture(dp(12), dp(180), dp(110), dp(182), MotionEvent.ACTION_UP);
    assertRestored();
    assertTrue("退出手勢不可傳入遠端控制", remoteEvents.isEmpty());
    screenshot("03-restored.png");
    // 再進出一次，避免只有初次狀態可用。
    enterFullscreen();
    directGesture(dp(12), dp(180), dp(110), dp(180), MotionEvent.ACTION_UP);
    assertRestored();
  }

  @Test public void normalGesturesAndEdgeTapsRemainUsable() {
    enterFullscreen();
    directGesture(dp(240), dp(180), dp(360), dp(180), MotionEvent.ACTION_UP);
    assertTrue(ui(() -> (boolean) field("desktopFullscreen")));
    assertEquals(List.of(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE, MotionEvent.ACTION_UP), remoteEvents);
    remoteEvents.clear();
    directGesture(dp(12), dp(170), dp(12), dp(250), MotionEvent.ACTION_UP);
    assertTrue(ui(() -> (boolean) field("desktopFullscreen")));
    assertEquals(List.of(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE, MotionEvent.ACTION_UP), remoteEvents);
    remoteEvents.clear();
    directGesture(dp(12), dp(180), dp(14), dp(180), MotionEvent.ACTION_UP);
    assertTrue(ui(() -> (boolean) field("desktopFullscreen")));
    assertEquals(List.of(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE, MotionEvent.ACTION_UP), remoteEvents);
    remoteEvents.clear();
    directGesture(dp(12), dp(180), dp(30), dp(180), MotionEvent.ACTION_CANCEL);
    assertTrue(ui(() -> (boolean) field("desktopFullscreen")));
    assertTrue("取消判斷中的手勢不可產生遠端點擊", remoteEvents.isEmpty());
  }

  @Test public void gestureAlsoWorksOverVideoAndOnlyInFullscreen() {
    ui(() -> {
      ((View) field("nativeScreen")).setVisibility(View.GONE);
      ((View) field("nativeVideo")).setVisibility(View.VISIBLE);
      return null;
    });
    enterFullscreen();
    directGesture(dp(12), dp(180), dp(110), dp(180), MotionEvent.ACTION_UP);
    assertRestored();
    assertTrue(remoteEvents.isEmpty());
    // 離開全螢幕後，最左側仍由 Android 手勢導覽保留；一般觸控從安全內縮區驗證。
    directGesture(dp(300), dp(180), dp(500), dp(180), MotionEvent.ACTION_UP);
    assertEquals(List.of(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE, MotionEvent.ACTION_UP), remoteEvents);
  }

  @Test public void systemLeftEdgeSwipeRestoresDesktop() {
    enterFullscreen();
    int height = ui(() -> activity.getWindow().getDecorView().getHeight());
    // 實際注入系統觸控；手勢導覽可先攔截邊緣，由 OnBackInvokedCallback 退出。
    long down = SystemClock.uptimeMillis();
    inject(down, MotionEvent.ACTION_DOWN, 1, height * 0.7f);
    for (int step = 1; step <= 20; step++) {
      SystemClock.sleep(12);
      inject(down, MotionEvent.ACTION_MOVE, 1 + dp(130) * step / 20f, height * 0.7f);
    }
    inject(down, MotionEvent.ACTION_UP, 1 + dp(130), height * 0.7f);
    assertRestored();
    assertTrue("系統返回手勢不可送出遠端操作", remoteEvents.isEmpty());
  }

  @Test public void systemBackAndDisconnectRestoreSystemBars() {
    enterFullscreen();
    instrumentation.sendKeyDownUpSync(KeyEvent.KEYCODE_BACK);
    assertRestored();
    enterFullscreen();
    ui(() -> { ((Button) field("nativeBack")).performClick(); return null; });
    await("返回站台列表並還原系統列", () -> ui(() -> !(boolean) field("inDesktop")
        && !(boolean) field("desktopFullscreen") && visibleBars() == originalBars));
  }

  @Test public void closeButtonReturnsToHeaderAfterReconnect() throws Exception {
    rotateDesktop(ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE);
    // 返回首頁時按鈕已隱藏，但仍保有上一輪的 View 尺寸；此時轉直向再重連。
    ui(() -> { invoke("leaveShell", new Class[]{}); return null; });
    rotateDesktop(ActivityInfo.SCREEN_ORIENTATION_PORTRAIT);
    await("首頁完成載入", () -> ui(() -> ((WebView) field("web")).getProgress() == 100
        && ((WebView) field("web")).getUrl().endsWith("/index.html")));
    ui(() -> {
      setField("inDesktop", true);
      assertEquals(true, invoke("applyFrameJSON", new Class[]{String.class}, localFrame));
      invoke("showDesktopPage", new Class[]{originalViewer.getClass(), int.class}, field("viewer"), field("generation"));
      return null;
    });
    rotateDesktop(ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE);
    await("重連桌面完成載入", () -> ui(() -> ((WebView) field("web")).getProgress() == 100
        && ((WebView) field("web")).getUrl().endsWith("/desktop.html")));
    assertCloseInHeader();
    await("旋轉動畫結束，關閉按鈕實際繪製到標題列", this::closeButtonIsRendered);
    screenshot("07-close-reconnected.png");
  }

  @Test public void closeButtonFollowsHeaderInsetsAndFullscreen() {
    rotateDesktop(ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE);
    assertCloseInHeader();
    // 模擬不同安全區與標題列高度；不能只使用某款手機的固定座標。
    ui(() -> {
      View root = (View) field("contentRoot");
      root.setPadding(Math.round(dp(24)), Math.round(dp(30)), Math.round(dp(48)), Math.round(dp(22)));
      return null;
    });
    instrumentation.waitForIdleSync();
    assertCloseInHeader();
    activity.new Bridge().desktopLayout(60, ui(() -> ((WebView) field("web")).getWidth() / (double) dp(1)));
    await("標題列高度更新", () -> ui(() -> Math.abs((int) field("desktopHeaderMargin") - dp(60)) <= 1));
    assertCloseInHeader();
    enterFullscreen();
    ui(() -> { invoke("setDesktopFullscreen", new Class[]{boolean.class}, false); return null; });
    await("退出全螢幕還原關閉按鈕", () -> ui(() -> ((View) field("nativeBack")).isShown()
        && visibleBars() == originalBars));
    assertCloseInHeader();
  }

  @Test public void draggedCloseButtonSurvivesRotationAndClosesOnTap() {
    rotateDesktop(ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE);
    Rect start = closeBounds();
    directGesture(start.exactCenterX(), start.exactCenterY(), dp(240), dp(140), MotionEvent.ACTION_UP);
    assertTrue("拖曳不可關閉桌面", ui(() -> (boolean) field("inDesktop")));
    Rect dragged = closeBounds();
    assertTrue("關閉按鈕可移到使用者選擇的位置", Math.abs(dragged.centerX() - start.centerX()) > dp(40));
    rotateDesktop(ActivityInfo.SCREEN_ORIENTATION_PORTRAIT);
    assertCloseWithinSafeArea();
    rotateDesktop(ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE);
    assertCloseWithinSafeArea();
    Rect restored = closeBounds();
    assertEquals("往返旋轉仍保留拖曳位置", dragged.centerX(), restored.centerX(), 2);
    assertEquals("往返旋轉仍保留拖曳位置", dragged.centerY(), restored.centerY(), 2);
    long down = SystemClock.uptimeMillis();
    // 與原生選單 Smoke 相同，經 Instrumentation 同步系統輸入視窗後點擊。
    for (int action : new int[]{MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP}) {
      MotionEvent event = MotionEvent.obtain(down, SystemClock.uptimeMillis(), action,
          restored.exactCenterX(), restored.exactCenterY(), 0);
      event.setSource(InputDevice.SOURCE_TOUCHSCREEN);
      try { instrumentation.sendPointerSync(event); } finally { event.recycle(); }
    }
    await("點擊關閉按鈕返回首頁", () -> ui(() -> !(boolean) field("inDesktop")));
    assertTrue("拖曳及關閉按鈕不得送出遠端觸控", remoteEvents.isEmpty());
  }

  private void rotateDesktop(int orientation) {
    ui(() -> { activity.setRequestedOrientation(orientation); return null; });
    await("畫面旋轉完成", () -> ui(() -> {
      View root = (View) field("contentRoot");
      return activity.hasWindowFocus() && !root.isLayoutRequested()
          && ((root.getWidth() > root.getHeight()) == (orientation == ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE));
    }));
    try { instrumentation.getUiAutomation().waitForIdle(150, 3000); }
    catch (java.util.concurrent.TimeoutException e) { throw new AssertionError("旋轉後介面未穩定", e); }
  }

  private Rect closeBounds() {
    return ui(() -> { Rect rect = new Rect(); assertTrue(((View) field("nativeBack")).getGlobalVisibleRect(rect)); return rect; });
  }

  private boolean closeButtonIsRendered() {
    Rect bounds = closeBounds();
    Bitmap shot = instrumentation.getUiAutomation().takeScreenshot();
    if (shot == null) return false;
    try {
      for (int x : new int[]{bounds.left + bounds.width() / 4, bounds.right - bounds.width() / 4}) {
        if (x < 0 || x >= shot.getWidth() || bounds.centerY() < 0 || bounds.centerY() >= shot.getHeight()) return false;
        int color = shot.getPixel(x, bounds.centerY());
        if (Math.abs(Color.red(color) - 46) > 8 || Math.abs(Color.green(color) - 125) > 8
            || Math.abs(Color.blue(color) - 91) > 8) return false;
      }
      return true;
    } finally { shot.recycle(); }
  }

  private void assertCloseWithinSafeArea() {
    ui(() -> {
      View root = (View) field("contentRoot"), button = (View) field("nativeBack");
      assertTrue("關閉按鈕不能超出左側安全區", button.getX() >= root.getPaddingLeft());
      assertTrue("關閉按鈕不能超出頂端安全區", button.getY() >= root.getPaddingTop());
      assertTrue("關閉按鈕不能超出右側安全區", button.getX() + button.getWidth() <= root.getWidth() - root.getPaddingRight() + 1);
      assertTrue("關閉按鈕不能超出底端安全區", button.getY() + button.getHeight() <= root.getHeight() - root.getPaddingBottom() + 1);
      return null;
    });
  }

  private void assertCloseInHeader() {
    instrumentation.waitForIdleSync();
    assertCloseWithinSafeArea();
    ui(() -> {
      View root = (View) field("contentRoot"), button = (View) field("nativeBack");
      assertEquals("關閉按鈕應位於標題列右端", root.getWidth() - root.getPaddingRight() - dp(6),
          button.getX() + button.getWidth(), 1);
      assertEquals("關閉按鈕應在標題列垂直置中", root.getPaddingTop() + (int) field("desktopHeaderMargin") / 2f,
          button.getY() + button.getHeight() / 2f, 1);
      return null;
    });
  }

  @Test public void jpegFullscreenPreservesAspectAndIgnoresLateHeaderReports() throws Exception {
    enterFullscreen();
    // 模擬隱藏 WebView 前已排入佇列的配置回報，不能把標題列高度加回全螢幕。
    double cssWidth = ui(() -> ((WebView) field("web")).getWidth() / (double) dp(1));
    activity.new Bridge().desktopLayout(44, cssWidth);
    instrumentation.waitForIdleSync();
    ui(() -> { assertFullscreenViewport(); return null; });
    await("JPEG 保持比例且完整呈現四邊", this::screenshotHasGreenBorder);
    assertTouchCorners(false);
    screenshot("04-jpeg-fullscreen.png");
    directGesture(dp(12), dp(180), dp(110), dp(180), MotionEvent.ACTION_UP);
    assertRestored();
    ui(() -> {
      View screen = (View) field("nativeScreen");
      View parent = (View) screen.getParent();
      FrameLayout.LayoutParams lp = (FrameLayout.LayoutParams) screen.getLayoutParams();
      assertEquals("一般模式影像緊接標題列", parent.getPaddingTop() + lp.topMargin, screen.getTop());
      assertEquals("底部不可超出可用區域", parent.getHeight() - parent.getPaddingBottom(), screen.getBottom());
      return null;
    });
  }

  @Test public void decodedH264FullscreenPreservesAspect() throws Exception {
    byte[] payload;
    try (InputStream stream = instrumentation.getContext().getAssets().open("fullscreen-h264-1080.bin")) {
      ByteArrayOutputStream output = new ByteArrayOutputStream();
      byte[] buffer = new byte[4096];
      for (int n; (n = stream.read(buffer)) != -1;) output.write(buffer, 0, n);
      payload = output.toByteArray();
    }
    EncodedVideoFrame frame = EncodedVideoFrame.parse(1, 1920, 1080, payload, true);
    assertNotNull(frame);
    ui(() -> {
      ((View) field("nativeScreen")).setVisibility(View.GONE);
      ((View) field("nativeVideo")).setVisibility(View.VISIBLE);
      return null;
    });
    await("影片 Surface 就緒", () -> ui(() -> field("videoSurface") != null));
    await("MediaCodec 實際輸出 H.264 測試影格", () -> ui(() -> (boolean)
        invoke("presentVideoFrame", new Class[]{EncodedVideoFrame.class}, frame)));
    enterFullscreen();
    await("H.264 保持比例且完整呈現四邊", this::screenshotHasGreenBorder);
    assertTouchCorners(true);
    screenshot("05-h264-fullscreen.png");
    directGesture(dp(12), dp(180), dp(110), dp(180), MotionEvent.ACTION_UP);
    assertRestored();
  }

  @Test public void keyboardButtonTogglesRealIme() {
    ui(() -> { ((Button) field("keyboardButton")).performClick(); return null; });
    await("鍵盤實際顯示", () -> ui(() -> (boolean) field("keyboardVisible") && imeVisible()));
    ui(() -> { assertTrue((boolean) field("inDesktop")); return null; });
    ui(() -> { ((Button) field("keyboardButton")).performClick(); return null; });
    await("鍵盤實際隱藏", () -> ui(() -> !(boolean) field("keyboardVisible") && !imeVisible()));
  }

  @Test public void jpegPinchAndPanMatchRenderedPixelsAndRemoteCoordinates() throws Exception {
    showQuadrants();
    ui(() -> {
      View screen = (View) field("nativeScreen");
      float cx = screen.getWidth() * .5f, cy = screen.getHeight() * .5f;
      float gap = screen.getWidth() / 12f;
      DesktopViewport viewport = (DesktopViewport) field("desktopViewport");
      float fit = Math.min(screen.getWidth() / 800f, screen.getHeight() / 400f);
      touch(screen, MotionEvent.ACTION_DOWN, new int[]{3}, cx - gap, cy);
      assertFalse("第一指待判斷，不可提前在遠端按下", (boolean) field("desktopPointerPressed"));
      touch(screen, MotionEvent.ACTION_POINTER_DOWN | (1 << MotionEvent.ACTION_POINTER_INDEX_SHIFT),
          new int[]{3, 7}, cx - gap, cy, cx + gap, cy);
      touch(screen, MotionEvent.ACTION_MOVE, new int[]{3, 7}, cx - 2 * gap, cy, cx + 2 * gap, cy);
      assertEquals(2f, viewport.zoom(), .01f);
      float dx = 800 * fit * 2 * .125f, dy = 400 * fit * 2 * .125f;
      touch(screen, MotionEvent.ACTION_MOVE, new int[]{3, 7}, cx - 2 * gap + dx, cy + dy, cx + 2 * gap + dx, cy + dy);
      touch(screen, MotionEvent.ACTION_POINTER_UP | (1 << MotionEvent.ACTION_POINTER_INDEX_SHIFT),
          new int[]{3, 7}, cx - 2 * gap + dx, cy + dy, cx + 2 * gap + dx, cy + dy);
      touch(screen, MotionEvent.ACTION_MOVE, new int[]{3}, cx - gap, cy);
      touch(screen, MotionEvent.ACTION_UP, new int[]{3}, cx - gap, cy);
      assertNull("多指手勢與剩餘單指不可取得遠端滑鼠", field("desktopTouchOwner"));
      assertFalse((boolean) field("desktopPointerPressed"));
      float[] point = jpegPoint(screen, cx, cy);
      assertEquals(.375f, point[0], .001f); assertEquals(.375f, point[1], .001f);
      Bitmap rendered = Bitmap.createBitmap(screen.getWidth(), screen.getHeight(), Bitmap.Config.ARGB_8888);
      screen.draw(new Canvas(rendered));
      try {
        int color = rendered.getPixel((int) cx, (int) cy);
        assertTrue("JPEG 繪製的中心應移到左上紅色象限", Color.red(color) > 180 && Color.green(color) < 80);
      } finally { rendered.recycle(); }
      // 放大後仍可正常點選遠端，按下座標必須對應目前可視區。
      touch(screen, MotionEvent.ACTION_DOWN, new int[]{3}, cx, cy);
      assertEquals(.375f, (float) field("desktopTouchX"), .001f);
      touch(screen, MotionEvent.ACTION_UP, new int[]{3}, cx, cy);
      assertFalse((boolean) field("desktopPointerPressed"));
      return null;
    });
  }

  @Test public void singleFingerPanAndResetDoNotMoveRemotePointer() throws Exception {
    showQuadrants();
    ui(() -> {
      View screen = (View) field("nativeScreen");
      DesktopViewport viewport = (DesktopViewport) field("desktopViewport");
      float cx = screen.getWidth() * .5f, cy = screen.getHeight() * .5f;
      invoke("transformViewport", new Class[]{View.class, float.class, float.class, float.class, float.class, float.class},
          screen, 2f, cx, cy, cx, cy);
      setField("viewportPanMode", true);
      touch(screen, MotionEvent.ACTION_DOWN, new int[]{0}, cx, cy);
      touch(screen, MotionEvent.ACTION_MOVE, new int[]{0}, cx + 100, cy + 50);
      touch(screen, MotionEvent.ACTION_UP, new int[]{0}, cx + 100, cy + 50);
      assertNull(field("desktopTouchOwner")); assertFalse((boolean) field("desktopPointerPressed"));
      assertTrue(jpegPoint(screen, cx, cy)[0] < .5f);
      // 回復遠端模式後，單指拖曳仍有完整按下／釋放配對。
      setField("viewportPanMode", false);
      touch(screen, MotionEvent.ACTION_DOWN, new int[]{0}, cx, cy);
      touch(screen, MotionEvent.ACTION_MOVE, new int[]{0}, cx + 100, cy);
      assertTrue((boolean) field("desktopPointerPressed"));
      touch(screen, MotionEvent.ACTION_CANCEL, new int[]{0}, cx + 100, cy);
      assertFalse((boolean) field("desktopPointerPressed"));
      viewport.reset(); invoke("refreshViewport", new Class[]{});
      assertEquals(1f, viewport.zoom(), 0);
      assertEquals(.5f, jpegPoint(screen, cx, cy)[0], .001f);
      assertEquals("100%", ((Button) field("viewportButton")).getText().toString());
      return null;
    });
  }

  @Test public void h264ZoomAndPanUseSameTouchAndDisplayTransform() throws Exception {
    byte[] payload;
    try (InputStream stream = instrumentation.getContext().getAssets().open("fullscreen-h264-1080.bin")) {
      ByteArrayOutputStream output = new ByteArrayOutputStream();
      byte[] bytes = new byte[4096];
      for (int n; (n = stream.read(bytes)) != -1;) output.write(bytes, 0, n);
      payload = output.toByteArray();
    }
    EncodedVideoFrame frame = EncodedVideoFrame.parse(1, 1920, 1080, payload, true);
    assertNotNull(frame);
    ui(() -> {
      ((View) field("nativeScreen")).setVisibility(View.GONE);
      ((View) field("nativeVideo")).setVisibility(View.VISIBLE);
      return null;
    });
    await("影片 Surface 就緒", () -> ui(() -> field("videoSurface") != null));
    await("H.264 實際解碼", () -> ui(() -> (boolean) invoke("presentVideoFrame", new Class[]{EncodedVideoFrame.class}, frame)));
    enterFullscreen();
    ui(() -> {
      View screen = (View) field("nativeVideo");
      float cx = screen.getWidth() * .5f, cy = screen.getHeight() * .5f, gap = screen.getWidth() / 12f;
      touch(screen, MotionEvent.ACTION_DOWN, new int[]{3}, cx - gap, cy);
      touch(screen, MotionEvent.ACTION_POINTER_DOWN | (1 << MotionEvent.ACTION_POINTER_INDEX_SHIFT), new int[]{3, 7}, cx - gap, cy, cx + gap, cy);
      touch(screen, MotionEvent.ACTION_MOVE, new int[]{3, 7}, cx - 2 * gap, cy, cx + 2 * gap, cy);
      touch(screen, MotionEvent.ACTION_MOVE, new int[]{3, 7}, cx - 2 * gap + 10000, cy + 10000, cx + 2 * gap + 10000, cy + 10000);
      touch(screen, MotionEvent.ACTION_CANCEL, new int[]{3, 7}, cx, cy, cx + gap, cy);
      assertEquals(2f, ((DesktopViewport) field("desktopViewport")).zoom(), .01f);
      assertNull(field("desktopTouchOwner"));
      float scale = Math.min(screen.getWidth() / 1920f, screen.getHeight() / 1080f) * 2;
      float[] point = (float[]) invoke("mapVideoTouch", new Class[]{float.class, float.class}, 8 * scale, 8 * scale);
      assertEquals(8 / 1920f, point[0], .001f); assertEquals(8 / 1080f, point[1], .001f);
      return null;
    });
    try { await("放大平移後 H.264 綠色左上邊框應貼齊可視區", () -> {
      Bitmap shot = instrumentation.getUiAutomation().takeScreenshot();
      if (shot == null) return false;
      int[] location = ui(() -> {
        View video = (View) field("nativeVideo"); int[] p = new int[2]; video.getLocationOnScreen(p);
        float scale = Math.min(video.getWidth() / 1920f, video.getHeight() / 1080f) * 2;
        p[0] += Math.round(8 * scale); p[1] += Math.round(8 * scale); return p;
      });
      try {
        int color = shot.getPixel(location[0], location[1]);
        return Color.green(color) > 160 && Color.green(color) > Color.red(color) + 40;
      } finally { shot.recycle(); }
    });
    } finally { screenshot("06-h264-zoom-pan.png"); }
    ui(() -> { invoke("clearComposedFrame", new Class[]{}); assertEquals(1f, ((DesktopViewport) field("desktopViewport")).zoom(), 0); return null; });
  }

  @Test public void viewportMenuOffersZoomPanAndFit() throws Exception {
    chooseViewportItem("放大 ＋");
    await("工具列可放大", () -> ui(() -> ((DesktopViewport) field("desktopViewport")).zoom() > 1f));
    chooseViewportItem("縮小 −");
    await("工具列可縮小", () -> ui(() -> ((DesktopViewport) field("desktopViewport")).zoom() == 1f));
    chooseViewportItem("放大 ＋");
    chooseViewportItem("單指拖移可視區");
    await("工具列啟用單指拖移", () -> ui(() -> (boolean) field("viewportPanMode")));
    assertTrue(ui(() -> ((Button)field("viewportButton")).getText().toString().contains("拖移")));
    chooseViewportItem("適合畫面（100%）");
    await("恢復完整畫面", () -> ui(() -> ((DesktopViewport) field("desktopViewport")).zoom() == 1f));
    chooseViewportItem("單指拖移可視區");
    await("切回單指操作遠端", () -> ui(() -> !(boolean) field("viewportPanMode")));
  }

  @Test public void h264DisplaySwitchRequiresNewKeyframeAndResetsViewport() throws Exception {
    byte[] payload;
    try(InputStream stream=instrumentation.getContext().getAssets().open("fullscreen-h264-1080.bin")) {
      ByteArrayOutputStream bytes=new ByteArrayOutputStream();byte[] buffer=new byte[4096];
      for(int n;(n=stream.read(buffer))!=-1;)bytes.write(buffer,0,n);
      payload=bytes.toByteArray();
    }
    String data=Base64.encodeToString(payload,Base64.NO_WRAP);
    ui(()->{
      View screen=(View)field("nativeScreen");float cx=screen.getWidth()*.5f,cy=screen.getHeight()*.5f;
      invoke("transformViewport",new Class[]{View.class,float.class,float.class,float.class,float.class,float.class},screen,2f,cx,cy,cx,cy);
      assertEquals(2f,((DesktopViewport)field("desktopViewport")).zoom(),0);
      setField("pumpRenderedFrames",0);
      setField("displayKnown",true);setField("hostDisplay",1);
      invoke("updateDisplayInputGate",new Class[]{});
      assertTrue((boolean)field("displayInputBlocked"));
      ((View)field("nativeVideo")).setVisibility(View.VISIBLE);
      return null;
    });
    await("切換影片 Surface 就緒",()->ui(()->field("videoSurface")!=null));
    JSONObject frame=new JSONObject().put("sequence",2).put("display",0).put("codec",1).put("width",1920).put("height",1080).put("keyframe",true).put("data",data);
    String old=frame.toString();
    ui(()->{assertEquals(false,invoke("applyFrameJSON",new Class[]{String.class},old));assertEquals(0,field("viewportDisplay"));return null;});
    String delta=frame.put("display",1).put("keyframe",false).toString();
    ui(()->{assertEquals(false,invoke("applyFrameJSON",new Class[]{String.class},delta));assertTrue((boolean)field("displayInputBlocked"));return null;});
    String keyframe=frame.put("keyframe",true).toString();
    ui(()->{invoke("applyFrameJSON",new Class[]{String.class},keyframe);return null;});
    await("新螢幕 H.264 實際解碼",()->ui(()->{
      return (int)invoke("drainVideoFrames",new Class[]{})>0 || (int)field("pumpRenderedFrames")>0;
    }));
    ui(()->{assertEquals(1,field("viewportDisplay"));assertFalse((boolean)field("displayInputBlocked"));assertEquals(1f,((DesktopViewport)field("desktopViewport")).zoom(),0);return null;});
  }

  private void chooseViewportItem(String title) throws Exception {
    NativeMenuTestHelper.choose(instrumentation,activity,ui(()->(View)field("viewportButton")),title);
  }

  private void showQuadrants() throws Exception {
    Bitmap bitmap = Bitmap.createBitmap(800, 400, Bitmap.Config.ARGB_8888);
    Canvas canvas = new Canvas(bitmap); Paint paint = new Paint();
    for (int i = 0; i < 4; i++) {
      paint.setColor(new int[]{Color.RED, Color.GREEN, Color.BLUE, Color.YELLOW}[i]);
      canvas.drawRect((i % 2) * 400, (i / 2) * 200, (i % 2 + 1) * 400, (i / 2 + 1) * 200, paint);
    }
    ByteArrayOutputStream output = new ByteArrayOutputStream(); bitmap.compress(Bitmap.CompressFormat.JPEG, 95, output); bitmap.recycle();
    String json = new JSONObject().put("sequence", 2).put("display", 0).put("codec", 0).put("width", 800).put("height", 400)
        .put("keyframe", true).put("data", Base64.encodeToString(output.toByteArray(), Base64.NO_WRAP)).toString();
    ui(() -> { assertEquals(true, invoke("applyFrameJSON", new Class[]{String.class}, json)); return null; });
  }

  private float[] jpegPoint(View screen, float x, float y) {
    try {
      Method method = screen.getClass().getDeclaredMethod("mapTouch", float.class, float.class); method.setAccessible(true);
      return (float[]) method.invoke(screen, x, y);
    } catch (ReflectiveOperationException error) { throw new AssertionError(error); }
  }

  private void touch(View screen, int action, int[] ids, float... xy) {
    MotionEvent.PointerProperties[] properties = new MotionEvent.PointerProperties[ids.length];
    MotionEvent.PointerCoords[] coordinates = new MotionEvent.PointerCoords[ids.length];
    for (int i = 0; i < ids.length; i++) {
      properties[i] = new MotionEvent.PointerProperties(); properties[i].id = ids[i]; properties[i].toolType = MotionEvent.TOOL_TYPE_FINGER;
      coordinates[i] = new MotionEvent.PointerCoords(); coordinates[i].x = xy[i * 2]; coordinates[i].y = xy[i * 2 + 1]; coordinates[i].pressure = 1;
    }
    long now = SystemClock.uptimeMillis();
    MotionEvent event = MotionEvent.obtain(now, now, action, ids.length, properties, coordinates, 0, 0, 1, 1, 0, 0, InputDevice.SOURCE_TOUCHSCREEN, 0);
    try { invoke("handleDesktopTouch", new Class[]{View.class, MotionEvent.class}, screen, event); }
    finally { event.recycle(); }
  }

  private void assertTouchCorners(boolean video) {
    ui(() -> {
      View screen = (View) field(video ? "nativeVideo" : "nativeScreen");
      float scale = Math.min(screen.getWidth() / 1920f, screen.getHeight() / 1080f);
      float left = (screen.getWidth() - 1920 * scale) / 2, top = (screen.getHeight() - 1080 * scale) / 2;
      float right = left + 1920 * scale, bottom = top + 1080 * scale;
      // contains 的右／下界不包含在影像內；黑邊不可被誤算成遠端四角。
      float[][] corners = {{left + .1f, top + .1f}, {right - .1f, top + .1f},
          {left + .1f, bottom - .1f}, {right - .1f, bottom - .1f}};
      for (int i = 0; i < corners.length; i++) {
        float[] point;
        if (video) point = (float[]) invoke("mapVideoTouch", new Class[]{float.class, float.class}, corners[i][0], corners[i][1]);
        else {
          try {
            Method map = screen.getClass().getDeclaredMethod("mapTouch", float.class, float.class);
            map.setAccessible(true);
            point = (float[]) map.invoke(screen, corners[i][0], corners[i][1]);
          } catch (ReflectiveOperationException e) { throw new AssertionError(e); }
        }
        assertEquals("來源 X 座標不可偏移", i % 2, point[0], 0.001f);
        assertEquals("來源 Y 座標不可偏移", i / 2, point[1], 0.001f);
      }
      return null;
    });
  }

  private boolean screenshotHasGreenBorder() {
    Bitmap shot = instrumentation.getUiAutomation().takeScreenshot();
    if (shot == null) return false;
    try {
      Rect screen = ui(() -> {
        View view = (View) field(((View) field("nativeVideo")).isShown() ? "nativeVideo" : "nativeScreen");
        int[] position = new int[2]; view.getLocationOnScreen(position);
        return new Rect(position[0], position[1], position[0] + view.getWidth(), position[1] + view.getHeight());
      });
      float scale = Math.min(screen.width() / 1920f, screen.height() / 1080f);
      float left = screen.left + (screen.width() - 1920 * scale) / 2;
      float top = screen.top + (screen.height() - 1080 * scale) / 2;
      for (int[] point : new int[][]{{8, 8}, {960, 8}, {1911, 8}, {8, 540},
          {1911, 540}, {8, 1071}, {960, 1071}, {1911, 1071}}) {
        int x = Math.round(left + point[0] * scale), y = Math.round(top + point[1] * scale);
        if (x < 0 || y < 0 || x >= shot.getWidth() || y >= shot.getHeight()) return false;
        int color = shot.getPixel(x, y);
        if (Color.green(color) < 160 || Color.green(color) < Color.red(color) + 40
            || Color.green(color) < Color.blue(color) + 20) return false;
      }
      // 不同比例手機應保留黑邊；不能為了滿版拉伸或裁掉來源桌面。
      if (left - screen.left > 4) {
        int color = shot.getPixel(screen.left + 2, screen.centerY());
        if (Color.red(color) > 30 || Color.green(color) > 30 || Color.blue(color) > 30) return false;
      }
      if (top - screen.top > 4) {
        int color = shot.getPixel(screen.centerX(), screen.top + 2);
        if (Color.red(color) > 30 || Color.green(color) > 30 || Color.blue(color) > 30) return false;
      }
      return true;
    } finally { shot.recycle(); }
  }

  private void assertFullscreenViewport() {
    View screen = (View) field(((View) field("nativeVideo")).isShown() ? "nativeVideo" : "nativeScreen");
    View parent = (View) screen.getParent();
    assertEquals("全螢幕影像必須从頂端開始", parent.getPaddingTop(), screen.getTop());
    assertEquals(parent.getPaddingLeft(), screen.getLeft());
    assertEquals(parent.getWidth() - parent.getPaddingRight(), screen.getRight());
    assertEquals(parent.getHeight() - parent.getPaddingBottom(), screen.getBottom());
    assertTrue("全螢幕必須隱藏狀態列", (activity.getWindow().getDecorView().getSystemUiVisibility()
        & View.SYSTEM_UI_FLAG_FULLSCREEN) != 0);
    for (String name : new String[]{"web", "desktopTools", "nativeBack"}) {
      assertEquals("全螢幕不可顯示控制列", View.GONE, ((View) field(name)).getVisibility());
    }
  }

  private void enterFullscreen() {
    ui(() -> { ((Button) field("fullscreenButton")).performClick(); return null; });
    await("系統列進入全螢幕", () -> ui(() -> (boolean) field("desktopFullscreen") && visibleBars() == 0));
    await("全螢幕影像配置完成", () -> ui(() -> {
      View screen = (View) field(((View) field("nativeVideo")).isShown() ? "nativeVideo" : "nativeScreen");
      return !screen.isLayoutRequested() && !((View) screen.getParent()).isLayoutRequested();
    }));
    ui(() -> { assertFullscreenViewport(); return null; });
  }

  private void assertRestored() {
    await("還原全螢幕前的位置與系統列", () -> ui(() -> !(boolean) field("desktopFullscreen")
        && visibleBars() == originalBars && screenBounds().equals(originalBounds)));
    ui(() -> {
      assertTrue("保持遠端桌面狀態", (boolean) field("inDesktop"));
      assertTrue((boolean) field("desktopPageReady"));
      assertSame("退出全螢幕不重建連線", originalViewer, field("viewer"));
      assertSame("退出全螢幕不清除影格", originalFrame, field("composedFrame"));
      assertEquals("全螢幕", ((Button) field("fullscreenButton")).getContentDescription());
      assertEquals("退出後恢復標題列", View.VISIBLE, ((WebView) field("web")).getVisibility());
      return null;
    });
  }

  private void directGesture(float x1, float y1, float x2, float y2, int end) {
    ui(() -> {
      long down = SystemClock.uptimeMillis();
      int[] actions = {MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE, end};
      for (int i = 0; i < actions.length; i++) {
        MotionEvent event = MotionEvent.obtain(down, down + 80L * i, actions[i], i == 0 ? x1 : x2, i == 0 ? y1 : y2, 0);
        event.setSource(InputDevice.SOURCE_TOUCHSCREEN);
        activity.dispatchTouchEvent(event);
        event.recycle();
      }
      return null;
    });
  }

  private void inject(long down, int action, float x, float y) {
    MotionEvent event = MotionEvent.obtain(down, SystemClock.uptimeMillis(), action, x, y, 0);
    event.setSource(InputDevice.SOURCE_TOUCHSCREEN);
    assertTrue(instrumentation.getUiAutomation().injectInputEvent(event, true));
    event.recycle();
  }

  private Rect screenBounds() {
    View screen = (View) field("nativeScreen");
    int[] position = new int[2];
    screen.getLocationOnScreen(position);
    return new Rect(position[0], position[1], position[0] + screen.getWidth(), position[1] + screen.getHeight());
  }

  private boolean imeVisible() {
    WindowInsetsCompat insets = ViewCompat.getRootWindowInsets(activity.getWindow().getDecorView());
    return insets != null && insets.isVisible(WindowInsetsCompat.Type.ime());
  }

  private int visibleBars() {
    WindowInsetsCompat insets = ViewCompat.getRootWindowInsets(activity.getWindow().getDecorView());
    if (insets == null) return -1;
    int visible = 0;
    for (int type : new int[]{WindowInsetsCompat.Type.statusBars(), WindowInsetsCompat.Type.navigationBars()}) {
      if (insets.isVisible(type)) visible |= type;
    }
    return visible;
  }

  private float dp(int value) { return value * activity.getResources().getDisplayMetrics().density; }

  private <T> T ui(Supplier<T> work) {
    AtomicReference<T> result = new AtomicReference<>();
    instrumentation.runOnMainSync(() -> result.set(work.get()));
    return result.get();
  }

  private void await(String message, BooleanSupplier predicate) {
    long deadline = SystemClock.uptimeMillis() + 6000;
    while (!predicate.getAsBoolean()) {
      if (SystemClock.uptimeMillis() >= deadline) fail(message);
      SystemClock.sleep(40);
    }
    instrumentation.waitForIdleSync();
  }

  private Object field(String name) {
    try { Field f = MainActivity.class.getDeclaredField(name); f.setAccessible(true); return f.get(activity); }
    catch (ReflectiveOperationException e) { throw new AssertionError(e); }
  }

  private void setField(String name, Object value) {
    try { Field f = MainActivity.class.getDeclaredField(name); f.setAccessible(true); f.set(activity, value); }
    catch (ReflectiveOperationException e) { throw new AssertionError(e); }
  }

  private Object invoke(String name, Class<?>[] types, Object... args) {
    try { Method m = MainActivity.class.getDeclaredMethod(name, types); m.setAccessible(true); return m.invoke(activity, args); }
    catch (ReflectiveOperationException e) { throw new AssertionError(e); }
  }

  private void screenshot(String name) throws Exception {
    Bitmap image = instrumentation.getUiAutomation().takeScreenshot();
    assertNotNull(image);
    File directory = new File(instrumentation.getTargetContext().getExternalFilesDir(null), "fullscreen-smoke");
    assertTrue(directory.isDirectory() || directory.mkdirs());
    try (FileOutputStream output = new FileOutputStream(new File(directory, name))) {
      assertTrue(image.compress(Bitmap.CompressFormat.PNG, 100, output));
    } finally { image.recycle(); }
  }
}
