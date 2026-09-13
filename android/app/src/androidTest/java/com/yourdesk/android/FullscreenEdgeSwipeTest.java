package com.yourdesk.android;

import static org.junit.Assert.*;

import android.app.Instrumentation;
import android.content.Intent;
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
    String frame = new JSONObject().put("sequence", 1).put("display", 0)
        .put("codec", 0).put("width", 1920).put("height", 1080).put("keyframe", true)
        .put("data", Base64.encodeToString(bytes.toByteArray(), Base64.NO_WRAP)).toString();
    ui(() -> {
      setField("inDesktop", true);
      assertEquals(true, invoke("applyFrameJSON", new Class[]{String.class}, frame));
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

  @Test public void jpegFullscreenFillsViewportAndIgnoresLateHeaderReports() throws Exception {
    enterFullscreen();
    // 模擬隱藏 WebView 前已排入佇列的配置回報，不能把標題列高度加回全螢幕。
    double cssWidth = ui(() -> ((WebView) field("web")).getWidth() / (double) dp(1));
    activity.new Bridge().desktopLayout(44, cssWidth);
    instrumentation.waitForIdleSync();
    ui(() -> { assertFullscreenViewport(); return null; });
    await("JPEG 四邊完整貼合顯示區", this::screenshotHasGreenBorder);
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

  @Test public void decodedH264FullscreenFillsViewport() throws Exception {
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
    await("H.264 四邊完整貼合顯示區", this::screenshotHasGreenBorder);
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

  private void assertTouchCorners(boolean video) {
    ui(() -> {
      View screen = (View) field(video ? "nativeVideo" : "nativeScreen");
      float[][] corners = {{0, 0}, {screen.getWidth(), 0},
          {0, screen.getHeight()}, {screen.getWidth(), screen.getHeight()}};
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
      int w = shot.getWidth(), h = shot.getHeight();
      for (int[] point : new int[][]{{8, 8}, {w/2, 8}, {w-9, 8}, {8, h/2},
          {w-9, h/2}, {8, h-9}, {w/2, h-9}, {w-9, h-9}}) {
        int color = shot.getPixel(point[0], point[1]);
        if (Color.green(color) < 160 || Color.green(color) < Color.red(color) + 40
            || Color.green(color) < Color.blue(color) + 20) return false;
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
