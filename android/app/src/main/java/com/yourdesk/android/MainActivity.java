package com.yourdesk.android;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.content.Context;
import android.content.pm.ActivityInfo;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.graphics.Rect;
import android.graphics.RectF;
import android.graphics.drawable.GradientDrawable;
import android.graphics.Matrix;
import android.os.Bundle;
import android.util.Base64;
import android.util.Log;
import android.view.Gravity;
import android.view.MotionEvent;
import android.view.SurfaceView;
import android.view.Surface;
import android.view.TextureView;
import android.view.View;
import android.view.Window;
import android.view.WindowManager;
import android.view.inputmethod.InputMethodManager;
import android.webkit.JavascriptInterface;
import android.webkit.WebResourceRequest;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.PopupWindow;
import android.widget.PopupMenu;
import android.widget.TextView;

import androidx.webkit.WebViewAssetLoader;
import androidx.core.graphics.Insets;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowInsetsCompat;
import androidx.core.view.WindowCompat;
import androidx.core.view.WindowInsetsControllerCompat;

import com.yourdesk.androidcore.core.TerminalSession;
import com.yourdesk.androidcore.core.Viewer;
import com.yourdesk.android.video.DecoderSupport;
import com.yourdesk.android.video.EncodedVideoFrame;
import com.yourdesk.android.video.FfmpegRuntime;
import com.yourdesk.android.video.MediaCodecVideoDecoder;

/** Android Viewer：WebView 負責介面，原生畫面元件負責合成 JPEG 差分影格。 */
public final class MainActivity extends Activity {
  private WebView web;
  private DeltaImageView nativeScreen;
  private TextureView nativeVideo;
  private Surface videoSurface;
  private final MediaCodecVideoDecoder videoDecoder = new MediaCodecVideoDecoder();
  private EncodedVideoFrame pendingVideoFrame;
  private int videoWidth;
  private int videoHeight;
  private boolean videoMode;
  private Button nativeBack;
  private LinearLayout desktopTools;
  private Button scaleButton;
  private Button fullscreenButton;
  private Button keyboardButton;
  private boolean keyboardVisible;
  private int keyboardRequestGeneration;
  private boolean desktopFullscreen;
  private int normalSystemUiVisibility;
  private int desktopHeaderMargin;
  private int normalVisibleBars;
  private int normalBarsBehavior;
  private FullscreenExitGesture fullscreenExitGesture;
  private android.window.OnBackInvokedCallback systemBackCallback;
  private android.graphics.Typeface faTypeface;
  private String selectedScale = "1/1", selectedQuality = "標準";
  private final android.os.Handler uiHandler = new android.os.Handler(android.os.Looper.getMainLooper());
  private volatile Viewer viewer = new Viewer();
  private TerminalSession terminal;
  private boolean inTerminal;
  private boolean inDesktop;
  private boolean desktopPageReady;
  // 返回鍵位於所有 WebView/影像層之上；記住按下狀態，避免子 View 在 DOWN/UP
  // 之間切換時吞掉事件，造成需要連按多次才返回。
  private boolean backGesture;
  private boolean backDragging;
  private float backDownX, backDownY, backStartX, backStartY;

  private int dp(float value) {
    return Math.round(value * getResources().getDisplayMetrics().density);
  }

  private void clampBackPosition() {
    if (nativeBack == null || nativeBack.getWidth() == 0) return;
    View parent = (View) nativeBack.getParent();
    nativeBack.setX(Math.max(parent.getPaddingLeft(), Math.min(nativeBack.getX(),
        parent.getWidth() - parent.getPaddingRight() - nativeBack.getWidth())));
    nativeBack.setY(Math.max(parent.getPaddingTop(), Math.min(nativeBack.getY(),
        parent.getHeight() - parent.getPaddingBottom() - nativeBack.getHeight())));
  }

  // 只在 UI thread 存取；每個非 keyframe JPEG 會貼回這張完整畫面。
  private Bitmap composedFrame;
  private int composedWidth;
  private int composedHeight;
  private int composedDisplay = Integer.MIN_VALUE;
  private long composedSequence;
  private long statsLastSampleNanos;
  private long statsLastTrafficNanos;
  private long statsFrameCount;
  private long statsLastSent;
  private long statsLastReceived;
  private double statsFps;
  private double statsTx;
  private double statsRx;

  @SuppressLint("SetJavaScriptEnabled")
  public void onCreate(Bundle b) {
    setRequestedOrientation(ActivityInfo.SCREEN_ORIENTATION_FULL_SENSOR);
    super.onCreate(b);
    desktopHeaderMargin = dp(44);
    fullscreenExitGesture = new FullscreenExitGesture(this, this::dispatchContentTouchEvent,
        () -> setDesktopFullscreen(false));
    if (android.os.Build.VERSION.SDK_INT >= 33) {
      // 系統手勢導覽會先攔截最左緣；沿用同一個返回入口，優先退出全螢幕。
      systemBackCallback = this::onBackPressed;
      getOnBackInvokedDispatcher().registerOnBackInvokedCallback(
          android.window.OnBackInvokedDispatcher.PRIORITY_DEFAULT, systemBackCallback);
    }
    FfmpegRuntime.probeAsync();
    Log.i("YourDeskVideo", "Android 解碼器候選=" + DecoderSupport.probe());

    FrameLayout root = new FrameLayout(this);
    // 所有圖層共用系統安全區，避免狀態列、瀏海與導覽列覆蓋內容。
    ViewCompat.setOnApplyWindowInsetsListener(root, (v, insets) -> {
      // 全螢幕的暫顯系統列應覆蓋畫面，不重新縮小影像；瀏海安全區仍保留。
      Insets bars = insets.getInsets((desktopFullscreen ? 0 : WindowInsetsCompat.Type.systemBars())
          | WindowInsetsCompat.Type.displayCutout());
      root.setPadding(bars.left, bars.top, bars.right, bars.bottom);
      root.post(this::clampBackPosition);
      return insets;
    });
    root.addOnLayoutChangeListener((v, l, t, r, btm, ol, ot, or, ob) -> clampBackPosition());
    SurfaceView surface = new SurfaceView(this);
    surface.setBackgroundColor(Color.BLACK);
    root.addView(surface, new FrameLayout.LayoutParams(-1, -1));

    web = new WebView(this);
    web.setFocusable(true);
    web.setFocusableInTouchMode(true);
    web.getSettings().setJavaScriptEnabled(true);
    web.getSettings().setSupportZoom(false);
    web.getSettings().setBuiltInZoomControls(false);
    web.getSettings().setDisplayZoomControls(false);
    web.getSettings().setTextZoom(100);
    web.getSettings().setAllowFileAccess(false);
    web.getSettings().setAllowContentAccess(false);
    web.addJavascriptInterface(new Bridge(), "YourDesk");
    WebViewAssetLoader loader = new WebViewAssetLoader.Builder()
        .addPathHandler("/assets/", new WebViewAssetLoader.AssetsPathHandler(this))
        .build();
    web.setWebViewClient(new WebViewClient() {
      @Override public boolean shouldOverrideUrlLoading(WebView v, WebResourceRequest r) {
        return !"appassets.androidplatform.net".equals(r.getUrl().getHost());
      }
      @Override public android.webkit.WebResourceResponse shouldInterceptRequest(WebView v, WebResourceRequest r) {
        return loader.shouldInterceptRequest(r.getUrl());
      }
    });
    root.addView(web, new FrameLayout.LayoutParams(-1, -1));

    nativeVideo = new TextureView(this);
    nativeVideo.setOpaque(true);
    nativeVideo.setSurfaceTextureListener(new TextureView.SurfaceTextureListener() {
      @Override public void onSurfaceTextureAvailable(android.graphics.SurfaceTexture texture, int width, int height) {
        if (videoSurface != null) videoSurface.release();
        videoSurface = new Surface(texture);
        if (pendingVideoFrame != null) presentVideoFrame(pendingVideoFrame);
      }

      @Override public void onSurfaceTextureSizeChanged(android.graphics.SurfaceTexture texture, int width, int height) {
        updateVideoTransform();
      }

      @Override public boolean onSurfaceTextureDestroyed(android.graphics.SurfaceTexture texture) {
        videoDecoder.reset();
        if (videoSurface != null) {
          videoSurface.release();
          videoSurface = null;
        }
        return true;
      }

      @Override public void onSurfaceTextureUpdated(android.graphics.SurfaceTexture texture) { }
    });
    nativeVideo.setOnTouchListener(this::handleDesktopTouch);
    nativeVideo.setVisibility(View.GONE);
    FrameLayout.LayoutParams videoLp = new FrameLayout.LayoutParams(-1, -1);
    videoLp.gravity = Gravity.TOP | Gravity.LEFT;
    videoLp.topMargin = desktopHeaderMargin;
    videoLp.bottomMargin = 0;
    root.addView(nativeVideo, videoLp);

    nativeScreen = new DeltaImageView(this);
    nativeScreen.setBackgroundColor(Color.BLACK);
    // DeltaImageView 自行以等比例矩形繪製；不能使用 FIT_XY，否則來源畫面會被拉伸。
    nativeScreen.setScaleType(ImageView.ScaleType.CENTER_INSIDE);
    nativeScreen.setOnTouchListener(this::handleDesktopTouch);
    nativeScreen.setVisibility(View.GONE);
    // 保留 WebView 上方的標題列與返回鍵；內容區才由原生畫面覆蓋。
    FrameLayout.LayoutParams screenLp = new FrameLayout.LayoutParams(-1, -1);
    screenLp.gravity = Gravity.TOP | Gravity.LEFT;
    screenLp.topMargin = desktopHeaderMargin;
    screenLp.bottomMargin = 0;
    root.addView(nativeScreen, screenLp);

    // 原生返回鍵置於影像層之上，避免 WebView 標題列被 overlay 攔截後無法返回。
    nativeBack = new Button(this);
    nativeBack.setText("×");
    nativeBack.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 21);
    nativeBack.setTextColor(Color.WHITE);
    nativeBack.setAllCaps(false);
    nativeBack.setGravity(Gravity.CENTER);
    nativeBack.setPadding(0, 0, 0, 0);
    nativeBack.setMinWidth(0);
    nativeBack.setMinHeight(0);
    nativeBack.setMinimumWidth(0);
    nativeBack.setMinimumHeight(0);
    nativeBack.setContentDescription("返回");
    GradientDrawable backCircle = new GradientDrawable();
    backCircle.setShape(GradientDrawable.OVAL);
    backCircle.setColor(Color.rgb(46, 125, 91));
    nativeBack.setBackground(backCircle);
    nativeBack.setStateListAnimator(null);
    nativeBack.setOnClickListener(v -> leaveShell());
    nativeBack.setVisibility(View.GONE);
    FrameLayout.LayoutParams backLp = new FrameLayout.LayoutParams(dp(36), dp(36), Gravity.TOP | Gravity.END);
    // 圓形 X 保留在狀態列下方；WindowInsets 會在不同裝置上修正上邊距。
    backLp.topMargin = dp(8);
    backLp.rightMargin = dp(6);
    root.addView(nativeBack, backLp);

    try { faTypeface = android.graphics.Typeface.createFromAsset(getAssets(), "fonts/fa-solid-900.ttf"); } catch (Exception ignored) { }
    desktopTools = new LinearLayout(this);
    desktopTools.setOrientation(LinearLayout.VERTICAL);
    desktopTools.setGravity(Gravity.CENTER);
    desktopTools.setPadding(dp(3), dp(3), dp(3), dp(3));
    GradientDrawable toolBg = new GradientDrawable();
    toolBg.setColor(0xcc111111); toolBg.setCornerRadius(dp(8));
    desktopTools.setBackground(toolBg);
    fullscreenButton = toolButton("\uf065", "全螢幕");
    fullscreenButton.setOnClickListener(v -> toggleFullscreen());
    scaleButton = toolButton("\uf390  1/1", "來源解析度");
    scaleButton.setOnClickListener(this::showScaleMenu);
    Button quality = toolButton("\uf1de", "畫質");
    quality.setOnClickListener(this::showQualityMenu);
    keyboardButton = toolButton("\uf11c", "鍵盤");
    keyboardButton.setOnClickListener(v -> toggleDesktopKeyboard());
    desktopTools.addView(fullscreenButton); desktopTools.addView(scaleButton);
    desktopTools.addView(quality); desktopTools.addView(keyboardButton);
    desktopTools.setVisibility(View.GONE);
    FrameLayout.LayoutParams toolsLp = new FrameLayout.LayoutParams(dp(46), dp(198), Gravity.START | Gravity.CENTER_VERTICAL);
    toolsLp.leftMargin = dp(8);
    root.addView(desktopTools, toolsLp);

    setContentView(root);
    ViewCompat.requestApplyInsets(root);
    web.loadUrl("https://appassets.androidplatform.net/assets/index.html");
  }

  private Button toolButton(String text, String description) {
    Button b = new Button(this); b.setText(text); b.setTextColor(Color.WHITE); b.setTextSize(12);
    b.setContentDescription(description); if (faTypeface != null) b.setTypeface(faTypeface); b.setAllCaps(false); b.setPadding(0,0,0,0); b.setMinWidth(0); b.setMinHeight(0);
    b.setBackgroundColor(Color.TRANSPARENT); return b;
  }
  private void toggleFullscreen() {
    setDesktopFullscreen(!desktopFullscreen);
  }

  private void toggleDesktopKeyboard() {
    if (keyboardVisible) hideDesktopKeyboard();
    else showDesktopKeyboard();
  }

  private void showDesktopKeyboard() {
    if (web == null) return;
    final int request = ++keyboardRequestGeneration;
    web.setFocusableInTouchMode(true);
    web.requestFocus();
    web.postDelayed(() -> {
      if (request != keyboardRequestGeneration || (!inDesktop && !inTerminal)
          || (inDesktop && desktopFullscreen)) return;
      InputMethodManager imm = (InputMethodManager) getSystemService(Context.INPUT_METHOD_SERVICE);
      web.evaluateJavascript("document.getElementById('keyboard-proxy')?.focus()",
          ignored -> { if (request == keyboardRequestGeneration)
            imm.showSoftInput(web, InputMethodManager.SHOW_FORCED); });
      // WebView 首次建立輸入連線時 callback 可能尚未完成，補一次同一 token 的顯示請求。
      web.postDelayed(() -> { if (request == keyboardRequestGeneration && keyboardVisible)
        imm.showSoftInput(web, InputMethodManager.SHOW_FORCED); }, 100);
      keyboardVisible = true;
      if (keyboardButton != null) keyboardButton.setSelected(true);
    }, 120);
  }

  private void hideDesktopKeyboard() {
    keyboardRequestGeneration++;
    if (web != null) {
      InputMethodManager imm = (InputMethodManager) getSystemService(Context.INPUT_METHOD_SERVICE);
      imm.hideSoftInputFromWindow(web.getWindowToken(), 0);
    }
    keyboardVisible = false;
    if (keyboardButton != null) keyboardButton.setSelected(false);
  }

  private void setDesktopFullscreen(boolean fullscreen) {
    if (desktopFullscreen == fullscreen || (fullscreen && (!inDesktop || !desktopPageReady))) return;
    if (fullscreen) hideDesktopKeyboard();
    View decor = getWindow().getDecorView();
    WindowInsetsControllerCompat controller = WindowCompat.getInsetsController(getWindow(), decor);
    int bars = WindowInsetsCompat.Type.systemBars();
    if (fullscreen) {
      normalSystemUiVisibility = decor.getSystemUiVisibility();
      WindowInsetsCompat insets = ViewCompat.getRootWindowInsets(decor);
      normalVisibleBars = 0;
      for (int type : new int[]{WindowInsetsCompat.Type.statusBars(),
          WindowInsetsCompat.Type.navigationBars(), WindowInsetsCompat.Type.captionBar()}) {
        if (insets == null || insets.isVisible(type)) normalVisibleBars |= type;
      }
      normalBarsBehavior = controller.getSystemBarsBehavior();
      controller.setSystemBarsBehavior(WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
      controller.hide(bars);
      getWindow().addFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN);
      // 部分 API/模擬器只套用 legacy flags，單靠 InsetsController 會留下透明狀態列圖示。
      decor.setSystemUiVisibility(View.SYSTEM_UI_FLAG_LAYOUT_STABLE
          | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
          | View.SYSTEM_UI_FLAG_FULLSCREEN | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
          | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY);
    } else {
      controller.setSystemBarsBehavior(normalBarsBehavior);
      controller.show(normalVisibleBars);
      controller.hide(bars & ~normalVisibleBars);
      getWindow().clearFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN);
      decor.setSystemUiVisibility(normalSystemUiVisibility);
    }
    desktopFullscreen = fullscreen;
    fullscreenButton.setText(fullscreen ? "\uf066" : "\uf065");
    fullscreenButton.setContentDescription(fullscreen ? "退出全螢幕" : "全螢幕");
    fullscreenButton.setSelected(fullscreen);
    // 全螢幕只留下遠端桌面影像；標題列、螢幕選擇器與工具列不佔顯示區。
    if (web != null) web.setVisibility(fullscreen ? View.GONE : View.VISIBLE);
    if (desktopTools != null) desktopTools.setVisibility(fullscreen ? View.GONE : View.VISIBLE);
    if (nativeBack != null) nativeBack.setVisibility(fullscreen ? View.GONE : View.VISIBLE);
    updateDesktopSurfaceLayout();
    // 只恢復系統列與安全區，保留連線、解碼器和目前畫面。
    ViewCompat.requestApplyInsets(decor);
  }

  /** 兩種呈現器共用可用區域；畫面本身由各自的 contain 矩形置中。 */
  private void updateDesktopSurfaceLayout() {
    int top = desktopFullscreen ? 0 : desktopHeaderMargin;
    for (View screen : new View[]{nativeScreen, nativeVideo}) {
      if (screen == null) continue;
      FrameLayout.LayoutParams lp = (FrameLayout.LayoutParams) screen.getLayoutParams();
      int gravity = Gravity.TOP | Gravity.LEFT;
      // CENTER 加上非零 margin 會額外偏移半個 margin，甚至讓底部超出父容器。
      if (lp.topMargin != top || lp.gravity != gravity) {
        lp.topMargin = top;
        lp.gravity = gravity;
        screen.setLayoutParams(lp);
      }
    }
  }
  private void sendStreamPreference(String scale, String quality) {
    String profile = "standard"; if ("低畫質".equals(quality)) profile = "fast"; else if ("高畫質".equals(quality)) profile = "high";
    int percent = "3/4".equals(scale) ? 75 : "3/5".equals(scale) ? 60 : "1/2".equals(scale) ? 50 : 100;
    try { org.json.JSONObject c = new org.json.JSONObject().put("type", "quality-profile").put("profile", profile).put("viewWidth", 1920).put("viewHeight", 1080).put("imageEnhancement", percent < 100); viewer.sendControlJSON(c.toString()); } catch (Exception ignored) { }
  }

  private void showScaleMenu(View anchor) {
    PopupMenu menu = new PopupMenu(this, anchor);
    for (String value : new String[]{"1/1", "3/4", "3/5", "1/2"}) { android.view.MenuItem i = menu.getMenu().add(value); i.setCheckable(true).setChecked(value.equals(selectedScale)); }
    menu.setOnMenuItemClickListener(item -> { selectedScale = item.getTitle().toString(); scaleButton.setText("\uf390  " + selectedScale); sendStreamPreference(selectedScale, null); return true; });
    menu.show();
  }
  private void showQualityMenu(View anchor) {
    PopupMenu menu = new PopupMenu(this, anchor);
    for (String value : new String[]{"低畫質", "標準", "高畫質"}) { android.view.MenuItem i = menu.getMenu().add(value); i.setCheckable(true).setChecked(value.equals(selectedQuality)); }
    menu.setOnMenuItemClickListener(item -> { selectedQuality = item.getTitle().toString(); sendStreamPreference(null, selectedQuality); return true; });
    menu.show();
  }

  /**
   * 在 Activity 最外層處理原生返回鍵的觸控。影像層會攔截其範圍內的所有觸控，
   * 而旋轉或 WebView 重排時 DOWN/UP 可能被分派到不同子 View；在這裡配對完整
   * 手勢可以確保一次點擊就執行返回。
   */
  @Override public boolean dispatchTouchEvent(MotionEvent event) {
    int[] location = new int[2];
    getWindow().getDecorView().getLocationOnScreen(location);
    if (fullscreenExitGesture != null && fullscreenExitGesture.dispatch(event,
        inDesktop && desktopFullscreen, location[0])) return true;
    return dispatchContentTouchEvent(event);
  }

  private boolean dispatchContentTouchEvent(MotionEvent event) {
    if (nativeBack != null && nativeBack.getVisibility() == View.VISIBLE && nativeBack.isShown()) {
      Rect bounds = new Rect();
      if (nativeBack.getGlobalVisibleRect(bounds)) {
        float rawX = event.getRawX();
        float rawY = event.getRawY();
        boolean inside = bounds.contains(Math.round(rawX), Math.round(rawY));
        int action = event.getActionMasked();
        if (action == MotionEvent.ACTION_DOWN) {
          backGesture = inside;
          if (backGesture) {
            backDragging = false;
            backDownX = rawX;
            backDownY = rawY;
            backStartX = nativeBack.getX();
            backStartY = nativeBack.getY();
            nativeBack.setPressed(true);
            return true;
          }
        } else if (backGesture) {
          if (action == MotionEvent.ACTION_MOVE) {
            float dx = rawX - backDownX, dy = rawY - backDownY;
            int slop = android.view.ViewConfiguration.get(this).getScaledTouchSlop();
            backDragging |= dx * dx + dy * dy > slop * slop;
            if (backDragging) {
              nativeBack.setX(backStartX + dx);
              nativeBack.setY(backStartY + dy);
              clampBackPosition();
            }
            nativeBack.setPressed(!backDragging && inside);
            return true;
          }
          if (action == MotionEvent.ACTION_UP) {
            backGesture = false;
            nativeBack.setPressed(false);
            if (!backDragging && inside) nativeBack.performClick();
            return true;
          }
          if (action == MotionEvent.ACTION_CANCEL) {
            backGesture = false;
            nativeBack.setPressed(false);
            return true;
          }
        }
      }
    }
    return super.dispatchTouchEvent(event);
  }

  private boolean handleDesktopTouch(View v, MotionEvent e) {
    if (!inDesktop) return false;
    float x;
    float y;
    if (v instanceof DeltaImageView) {
      float[] point = ((DeltaImageView) v).mapTouch(e.getX(), e.getY());
      x = point[0];
      y = point[1];
    } else if (v == nativeVideo) {
      float[] point = mapVideoTouch(e.getX(), e.getY());
      x = point[0];
      y = point[1];
    } else {
      float width = Math.max(1f, v.getWidth());
      float height = Math.max(1f, v.getHeight());
      x = clamp(e.getX() / width);
      y = clamp(e.getY() / height);
    }
    int action = e.getActionMasked();
    try {
      org.json.JSONObject c = new org.json.JSONObject();
      if (action == MotionEvent.ACTION_DOWN) {
        c.put("type", "button").put("button", 1).put("down", true).put("x", x).put("y", y);
      } else if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) {
        c.put("type", "button").put("button", 1).put("down", false).put("x", x).put("y", y);
      } else if (action == MotionEvent.ACTION_MOVE) {
        c.put("type", "move").put("x", x).put("y", y);
      } else {
        return true;
      }
      viewer.sendControlJSON(c.toString());
    } catch (Exception ignored) {
      // 連線中止時觸控事件不應讓 UI thread 崩潰。
    }
    return true;
  }

  private static float clamp(float value) {
    return Math.max(0f, Math.min(1f, value));
  }

  private final java.util.concurrent.ExecutorService io = java.util.concurrent.Executors.newSingleThreadExecutor();
  private volatile int generation;

  private void clearComposedFrame() {
    if (composedFrame != null && !composedFrame.isRecycled()) composedFrame.recycle();
    composedFrame = null;
    composedWidth = composedHeight = 0;
    composedDisplay = Integer.MIN_VALUE;
    composedSequence = 0;
    resetStats();
    if (nativeScreen != null) nativeScreen.clearFrame();
    pendingVideoFrame = null;
    videoWidth = videoHeight = 0;
    videoMode = false;
    videoDecoder.reset();
    if (nativeVideo != null) nativeVideo.setVisibility(View.GONE);
  }

  /** 讓 TextureView 以與 JPEG 相同的 contain 規則顯示影片，絕不拉伸來源。 */
  private void updateVideoTransform() {
    if (nativeVideo == null || videoWidth <= 0 || videoHeight <= 0) return;
    int viewWidth = nativeVideo.getWidth();
    int viewHeight = nativeVideo.getHeight();
    if (viewWidth <= 0 || viewHeight <= 0) return;
    float scale = Math.min(viewWidth / (float) videoWidth, viewHeight / (float) videoHeight);
    Matrix matrix = new Matrix();
    matrix.setScale(videoWidth * scale / viewWidth, videoHeight * scale / viewHeight,
        viewWidth * 0.5f, viewHeight * 0.5f);
    nativeVideo.setTransform(matrix);
  }

  private float[] mapVideoTouch(float x, float y) {
    if (videoWidth <= 0 || videoHeight <= 0 || nativeVideo == null) {
      return new float[]{clamp(x / Math.max(1f, nativeVideo == null ? 1 : nativeVideo.getWidth())),
          clamp(y / Math.max(1f, nativeVideo == null ? 1 : nativeVideo.getHeight()))};
    }
    float scale = Math.min(nativeVideo.getWidth() / (float) videoWidth,
        nativeVideo.getHeight() / (float) videoHeight);
    float drawnWidth = videoWidth * scale;
    float drawnHeight = videoHeight * scale;
    float left = (nativeVideo.getWidth() - drawnWidth) * 0.5f;
    float top = (nativeVideo.getHeight() - drawnHeight) * 0.5f;
    return new float[]{clamp((x - left) / Math.max(1f, drawnWidth)),
        clamp((y - top) / Math.max(1f, drawnHeight))};
  }

  private boolean presentVideoFrame(EncodedVideoFrame frame) {
    pendingVideoFrame = frame;
    videoMode = true;
    videoWidth = frame.width;
    videoHeight = frame.height;
    updateVideoTransform();
    if (videoSurface == null || !videoSurface.isValid()) return true;
    int rendered = videoDecoder.queue(frame, videoSurface);
    if (rendered < 0) {
      Log.w("YourDeskVideo", "MediaCodec 無法解碼 " + frame.codec + "，等待下一個 keyframe");
      return false;
    }
    if (rendered > 0 && web != null) {
      String label = frame.codec == 1 ? "H.264" : "HEVC";
      web.evaluateJavascript("window.desktopFrameStatus&&window.desktopFrameStatus(" + frame.width + "," + frame.height + ",false,'" + label + "'," + 0 + ")", null);
    }
    return rendered > 0;
  }

  private void resetStats() {
    statsLastSampleNanos = 0;
    statsLastTrafficNanos = 0;
    statsFrameCount = 0;
    statsLastSent = statsLastReceived = 0;
    statsFps = statsTx = statsRx = 0;
  }

  private void leaveShell() {
    setDesktopFullscreen(false);
    hideDesktopKeyboard();
    inTerminal = false;
    inDesktop = false;
    if (nativeScreen != null) nativeScreen.setVisibility(View.GONE);
    if (nativeBack != null) nativeBack.setVisibility(View.GONE);
    if (desktopTools != null) desktopTools.setVisibility(View.GONE);
    desktopPageReady = false;
    clearComposedFrame();
    generation++;
    Viewer previous = viewer;
    viewer = new Viewer();
    terminal = null;
    new Thread(previous::close).start();
    setRequestedOrientation(ActivityInfo.SCREEN_ORIENTATION_FULL_SENSOR);
    web.loadUrl("https://appassets.androidplatform.net/assets/index.html");
  }

  private void startNativeFramePump(Viewer session, int epoch) {
    uiHandler.post(new Runnable() {
      @Override public void run() {
        if (epoch != generation || !inDesktop || session != viewer) return;
        // 一次排空佇列，避免一個輪詢週期只取一塊而造成差分延遲。
        boolean presented = false;
        for (int i = 0; i < 32; i++) {
          String json;
          try {
            json = session.readFrameJSON();
          } catch (Exception ignored) {
            json = "";
          }
          if (json == null || json.isEmpty()) break;
          presented |= applyFrameJSON(json);
        }
        if (presented && inDesktop && !desktopPageReady) showDesktopPage(session, epoch);
        updateDesktopStats(session, presented);
        uiHandler.postDelayed(this, 50);
      }
    });
  }

  private void showDesktopPage(Viewer session, int epoch) {
    if (epoch != generation || session != viewer || !inDesktop || desktopPageReady) return;
    desktopPageReady = true;
    nativeScreen.setVisibility(videoMode ? View.GONE : View.VISIBLE);
    nativeVideo.setVisibility(videoMode ? View.VISIBLE : View.GONE);
    updateVideoTransform();
    nativeBack.setVisibility(View.VISIBLE);
    desktopTools.setVisibility(View.VISIBLE);
    setRequestedOrientation(ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE);
    web.loadUrl("https://appassets.androidplatform.net/assets/desktop.html");
    // 影格已先在原生基底合成；頁面載入完成後補上尺寸提示與目前統計。
    web.postDelayed(() -> {
      if (epoch != generation || session != viewer || !desktopPageReady) return;
      if (composedWidth > 0 && composedHeight > 0)
        web.evaluateJavascript("window.desktopFrameStatus&&window.desktopFrameStatus(" + composedWidth + "," + composedHeight + ",true)", null);
      web.evaluateJavascript("window.desktopStats&&window.desktopStats(" + statsFps + "," + statsTx + "," + statsRx + ")", null);
    }, 250);
  }

  private void updateDesktopStats(Viewer session, boolean presented) {
    long now = System.nanoTime();
    if (statsLastSampleNanos == 0) {
      statsLastSampleNanos = now;
      statsLastTrafficNanos = now;
      try {
        org.json.JSONObject t = new org.json.JSONObject(session.trafficJSON());
        statsLastSent = t.optLong("sentBytes", 0);
        statsLastReceived = t.optLong("receivedBytes", 0);
      } catch (Exception ignored) { }
    }
    if (presented) statsFrameCount++;
    long elapsedNanos = now - statsLastSampleNanos;
    if (elapsedNanos < 500_000_000L) return;
    double elapsed = elapsedNanos / 1_000_000_000.0;
    statsFps = statsFrameCount / elapsed;
    statsFrameCount = 0;
    try {
      org.json.JSONObject t = new org.json.JSONObject(session.trafficJSON());
      long sent = t.optLong("sentBytes", statsLastSent);
      long received = t.optLong("receivedBytes", statsLastReceived);
      double trafficElapsed = Math.max(0.001, (now - statsLastTrafficNanos) / 1_000_000_000.0);
      statsTx = Math.max(0, sent - statsLastSent) / trafficElapsed;
      statsRx = Math.max(0, received - statsLastReceived) / trafficElapsed;
      statsLastSent = sent;
      statsLastReceived = received;
      statsLastTrafficNanos = now;
    } catch (Exception ignored) { }
    statsLastSampleNanos = now;
    web.evaluateJavascript("window.desktopStats&&window.desktopStats(" + statsFps + "," + statsTx + "," + statsRx + ")", null);
  }

  /** 依 PC 版 compositeDecodedFrame 的規則，把 JPEG 區塊貼回完整 Bitmap。 */
  private boolean applyFrameJSON(String json) {
    try {
      org.json.JSONObject o = new org.json.JSONObject(json);
      int codec = o.optInt("codec", -1);
      long sequence = o.optLong("sequence", 0);
      int display = o.optInt("display", -1);
      int fullWidth = o.optInt("width", 0);
      int fullHeight = o.optInt("height", 0);
      int x = o.optInt("x", 0);
      int y = o.optInt("y", 0);
      boolean keyframe = o.optBoolean("keyframe", false);
      if (sequence <= 0 || fullWidth <= 0 || fullHeight <= 0 || x < 0 || y < 0) return false;
      if (fullWidth > 8192 || fullHeight > 8192) return false;
      if ((long) fullWidth * (long) fullHeight > 32L * 1024L * 1024L) return false;
      if (codec == 1 || codec == 2) {
        // H.264／HEVC 影格由 MediaCodec 直接輸出到 TextureView；這類影格不參與
        // JPEG 差分基底，避免兩種 renderer 互相覆蓋。
        if (x != 0 || y != 0) return false;
        byte[] payload = Base64.decode(o.getString("data"), Base64.DEFAULT);
        EncodedVideoFrame frame = EncodedVideoFrame.parse(codec, fullWidth, fullHeight, payload, keyframe);
        if (frame == null) return false;
        if (composedFrame != null) {
          if (!composedFrame.isRecycled()) composedFrame.recycle();
          composedFrame = null;
        }
        composedSequence = sequence;
        boolean rendered = presentVideoFrame(frame);
        if (desktopPageReady) {
          nativeScreen.setVisibility(View.GONE);
          nativeVideo.setVisibility(View.VISIBLE);
          updateVideoTransform();
        }
        return rendered;
      }
      if (codec != 0) return false;
      if (videoMode) {
        // Host 可能在硬解失敗後重新協商 JPEG；切換 renderer 時先釋放舊 Surface
        // decoder，避免兩個畫面層同時攔截觸控或持續佔用硬體解碼資源。
        videoMode = false;
        pendingVideoFrame = null;
        videoDecoder.reset();
        nativeVideo.setVisibility(View.GONE);
        if (desktopPageReady) nativeScreen.setVisibility(View.VISIBLE);
      }
      if (composedSequence > 0 && sequence <= composedSequence) return false;
      // 遺失差分時不能拿錯誤基底繼續畫；等下一個完整 keyframe 重建。
      if (!keyframe && (composedFrame == null || composedWidth != fullWidth || composedHeight != fullHeight
          || composedDisplay != display || (composedSequence > 0 && sequence != composedSequence + 1))) {
        composedSequence = sequence;
        return false;
      }
      byte[] data = Base64.decode(o.getString("data"), Base64.DEFAULT);
      Bitmap patch = decodeJpeg(data);
      if (patch == null || patch.getWidth() <= 0 || patch.getHeight() <= 0) return false;
      if (x + patch.getWidth() > fullWidth || y + patch.getHeight() > fullHeight) {
        patch.recycle();
        return false;
      }

      boolean replace = keyframe || composedFrame == null || composedWidth != fullWidth || composedHeight != fullHeight
          || composedDisplay != display;
      if (replace) {
        Bitmap old = composedFrame;
        composedFrame = Bitmap.createBitmap(fullWidth, fullHeight, Bitmap.Config.ARGB_8888);
        composedWidth = fullWidth;
        composedHeight = fullHeight;
        composedDisplay = display;
        if (old != null && !old.isRecycled()) old.recycle();
      }
      Canvas canvas = new Canvas(composedFrame);
      canvas.drawBitmap(patch, x, y, null);
      int patchWidth = patch.getWidth();
      int patchHeight = patch.getHeight();
      patch.recycle();
      composedSequence = sequence;
      Rect dirty = new Rect(x, y, x + patchWidth, y + patchHeight);
      nativeScreen.setFrame(composedFrame, dirty, replace);
      if (keyframe) {
        web.evaluateJavascript("window.desktopFrameStatus&&window.desktopFrameStatus(" + fullWidth + "," + fullHeight + ",true)", null);
      }
      return true;
    } catch (Exception ignored) {
      // 收到損壞的單一影格時丟棄該影格，後續 keyframe 仍可恢復。
      return false;
    }
  }

  /**
   * 差分區塊需要貼回可變 Bitmap，因此使用軟體 Bitmap 解碼。
   * ALLOCATOR_HARDWARE 是儲存位置選項，不代表 JPEG 硬解，且不能作為
   * 這個軟體 Canvas 的來源。影片硬解另走 MediaCodec／Surface 路徑。
   */
  private static Bitmap decodeJpeg(byte[] data) {
    BitmapFactory.Options options = new BitmapFactory.Options();
    options.inPreferredConfig = Bitmap.Config.ARGB_8888;
    return BitmapFactory.decodeByteArray(data, 0, data.length, options);
  }

  /** 原生 View 只 invalidate 變動區域；合成資料仍保留完整桌面基底。 */
  private static final class DeltaImageView extends ImageView {
    private final Paint paint = new Paint();
    private final Rect source = new Rect();
    private final RectF destination = new RectF();
    private Bitmap frame;

    DeltaImageView(Context context) {
      super(context);
      paint.setFilterBitmap(false);
      setWillNotDraw(false);
    }

    void setFrame(Bitmap bitmap, Rect dirty, boolean full) {
      frame = bitmap;
      if (full || dirty == null) {
        invalidate();
        return;
      }
      Rect mapped = mapToView(dirty, bitmap);
      if (mapped.isEmpty()) invalidate();
      else invalidate(mapped);
    }

    void clearFrame() {
      frame = null;
      invalidate();
    }

    private Rect mapToView(Rect dirty, Bitmap bitmap) {
      if (getWidth() <= 0 || getHeight() <= 0 || bitmap == null) return new Rect();
      computeDestination(bitmap, destination);
      float scale = destination.width() / bitmap.getWidth();
      return new Rect((int) Math.floor(destination.left + dirty.left * scale),
          (int) Math.floor(destination.top + dirty.top * scale),
          (int) Math.ceil(destination.left + dirty.right * scale),
          (int) Math.ceil(destination.top + dirty.bottom * scale));
    }

    /** 將觸控座標換算回等比例畫面；黑邊會對應到最近的畫面邊界。 */
    float[] mapTouch(float x, float y) {
      Bitmap bitmap = frame;
      if (bitmap == null || bitmap.isRecycled() || getWidth() <= 0 || getHeight() <= 0) {
        return new float[]{clamp(x / Math.max(1f, getWidth())), clamp(y / Math.max(1f, getHeight()))};
      }
      computeDestination(bitmap, destination);
      return new float[]{clamp((x - destination.left) / Math.max(1f, destination.width())),
          clamp((y - destination.top) / Math.max(1f, destination.height()))};
    }

    @Override protected void onSizeChanged(int w, int h, int oldw, int oldh) {
      super.onSizeChanged(w, h, oldw, oldh);
      invalidate();
    }

    @Override protected void onDraw(Canvas canvas) {
      canvas.drawColor(Color.BLACK);
      Bitmap bitmap = frame;
      if (bitmap == null || bitmap.isRecycled() || getWidth() <= 0 || getHeight() <= 0) return;
      source.set(0, 0, bitmap.getWidth(), bitmap.getHeight());
      computeDestination(bitmap, destination);
      // Canvas 的 clip 是系統要求的 dirty region，這裡只會實際填補該區域。
      canvas.drawBitmap(bitmap, source, destination, paint);
    }

    /** 將來源完整畫面以統一縮放係數置中，剩餘區域保留黑邊。 */
    private void computeDestination(Bitmap bitmap, RectF out) {
      float scale = Math.min(getWidth() / (float) bitmap.getWidth(),
          getHeight() / (float) bitmap.getHeight());
      float width = bitmap.getWidth() * scale;
      float height = bitmap.getHeight() * scale;
      out.set((getWidth() - width) * 0.5f, (getHeight() - height) * 0.5f,
          (getWidth() + width) * 0.5f, (getHeight() + height) * 0.5f);
    }
  }

  final class Bridge {
    private final Object credentialLock = new Object();

    private javax.crypto.SecretKey key() throws Exception {
      java.security.KeyStore ks = java.security.KeyStore.getInstance("AndroidKeyStore");
      ks.load(null);
      if (!ks.containsAlias("shell")) {
        javax.crypto.KeyGenerator g = javax.crypto.KeyGenerator.getInstance("AES", "AndroidKeyStore");
        g.init(new android.security.keystore.KeyGenParameterSpec.Builder("shell", 3)
            .setBlockModes("GCM").setEncryptionPaddings("NoPadding").build());
        g.generateKey();
      }
      return (javax.crypto.SecretKey) ks.getKey("shell", null);
    }

    @JavascriptInterface public String remembered() {
      synchronized (credentialLock) {
        try {
          String value = getPreferences(0).getString("credentials", "");
          if (value.isEmpty()) return "{}";
          String[] parts = value.split(":", -1);
          if (parts.length != 2) return "{}";
          javax.crypto.Cipher c = javax.crypto.Cipher.getInstance("AES/GCM/NoPadding");
          c.init(2, key(), new javax.crypto.spec.GCMParameterSpec(128, Base64.decode(parts[0], Base64.NO_WRAP)));
          return new String(c.doFinal(Base64.decode(parts[1], Base64.NO_WRAP)), java.nio.charset.StandardCharsets.UTF_8);
        } catch (Exception e) { return "{}"; }
      }
    }

    /**
     * 以同步 commit 確保使用者按下連接後，即使頁面立即切換或程序被回收，
     * 加密憑證也已落盤。回傳值讓 JS 可在清除輸入框前確認寫入成功。
     */
    @JavascriptInterface public boolean rememberCredentials(String room, String secret) {
      synchronized (credentialLock) {
        try {
          if (room == null || room.trim().isEmpty()) return false;
          String normalized = room.trim();
          org.json.JSONObject stored = new org.json.JSONObject(remembered());
          org.json.JSONObject sites = stored.optJSONObject("sites");
          if (sites == null) {
            sites = new org.json.JSONObject();
            if (stored.optString("room", "").equals(normalized)) sites.put(normalized, stored.optString("secret", ""));
          }
          if (secret == null || secret.isEmpty()) sites.remove(normalized); else sites.put(normalized, secret);
          if (sites.length() == 0) return getPreferences(0).edit().remove("credentials").commit();
          javax.crypto.Cipher c = javax.crypto.Cipher.getInstance("AES/GCM/NoPadding");
          c.init(1, key());
          String json = new org.json.JSONObject().put("sites", sites).toString();
          String encrypted = Base64.encodeToString(c.getIV(), Base64.NO_WRAP) + ":" + Base64.encodeToString(
              c.doFinal(json.getBytes(java.nio.charset.StandardCharsets.UTF_8)), Base64.NO_WRAP);
          boolean committed = getPreferences(0).edit().remove("room").putString("credentials", encrypted).commit();
          return committed && getPreferences(0).contains("credentials");
        } catch (Exception e) {
          runOnUiThread(() -> android.widget.Toast.makeText(MainActivity.this, "密碼保存失敗", 1).show());
          return false;
        }
      }
    }

    // 使用 WebView 實際比例換算 CSS 座標，旋轉與密度變更共用同一配置。
    @JavascriptInterface public void desktopLayout(double top, double viewportWidth) {
      if (!Double.isFinite(top) || !Double.isFinite(viewportWidth) || top < 0 || viewportWidth <= 0) return;
      runOnUiThread(() -> {
        if (!inDesktop || !desktopPageReady) return;
        if (web.getWidth() <= 0 || web.getHeight() <= 0) return;
        desktopHeaderMargin = Math.min(web.getHeight(), (int) Math.round(top * web.getWidth() / viewportWidth));
        // 隱藏 WebView 後仍可能收到排隊的回報；全螢幕必須始終維持零上邊距。
        updateDesktopSurfaceLayout();
      });
    }

    @JavascriptInterface public boolean setStreamPreferences(String profile, int scalePercent) {
      try {
        org.json.JSONObject c = new org.json.JSONObject();
        c.put("type", "quality-profile").put("profile", profile == null ? "standard" : profile);
        c.put("viewWidth", 1920).put("viewHeight", 1080);
        c.put("imageEnhancement", scalePercent < 100);
        viewer.sendControlJSON(c.toString());
        return true;
      } catch (Exception e) { return false; }
    }

    @JavascriptInterface public String sendControlJSON(String payload) {
      try { viewer.sendControlJSON(payload); return "ok"; }
      catch (Exception e) { return e.getMessage() == null ? "send failed" : e.getMessage(); }
    }

    @JavascriptInterface public String readFrameJSON() {
      try { return viewer.readFrameJSON(); } catch (Exception e) { return ""; }
    }

    @JavascriptInterface public void showKeyboard() {
      runOnUiThread(MainActivity.this::showDesktopKeyboard);
    }

    @JavascriptInterface public void hideKeyboard() {
      runOnUiThread(MainActivity.this::hideDesktopKeyboard);
    }

    @JavascriptInterface public void abortConnection() {
      runOnUiThread(() -> {
        setDesktopFullscreen(false);
        hideDesktopKeyboard();
        generation++;
        inTerminal = false;
        inDesktop = false;
        desktopPageReady = false;
        if (nativeScreen != null) nativeScreen.setVisibility(View.GONE);
        if (nativeBack != null) nativeBack.setVisibility(View.GONE);
        clearComposedFrame();
        Viewer previous = viewer;
        viewer = new Viewer();
        new Thread(previous::close).start();
        setRequestedOrientation(ActivityInfo.SCREEN_ORIENTATION_FULL_SENSOR);
      });
    }

    // 主畫面完成認證與通道開啟後才切頁；憑證不放入 URL。
    @JavascriptInterface public void connectSession(String payload) {
      runOnUiThread(() -> {
        final int epoch = ++generation;
        final Viewer previous = viewer;
        final Viewer session = new Viewer();
        viewer = session;
        terminal = null;
        new Thread(previous::close).start();
        io.execute(() -> {
          try {
            org.json.JSONObject in = new org.json.JSONObject(payload);
            String mode = in.getString("mode");
            if (!mode.equals("shell") && !mode.equals("desktop")) throw new Exception("模式無效");
            if (epoch != generation) return;
            session.connect("wss://desktop.mars-cloud.com:8080/ws", in.getString("room"), in.getString("secret"));
            if (epoch != generation) { session.close(); return; }
            if (mode.equals("desktop")) {
              // 讓 Host 只選擇 Android 已實際找到的 decoder；JPEG 永遠保留備援。
              session.sendVideoCapabilities(DecoderSupport.supportedWireCodecs(),
                  DecoderSupport.hardwareWireCodecs());
            }
            TerminalSession opened = null;
            if (mode.equals("shell")) { opened = session.terminal(); opened.open(80, 24); }
            final TerminalSession ready = opened;
            runOnUiThread(() -> {
              if (epoch != generation || isDestroyed()) { new Thread(session::close).start(); return; }
              terminal = ready;
              inTerminal = mode.equals("shell");
              inDesktop = mode.equals("desktop");
              if (inDesktop) {
                clearComposedFrame();
                desktopPageReady = false;
                startNativeFramePump(session, epoch);
              } else {
                setRequestedOrientation(ActivityInfo.SCREEN_ORIENTATION_FULL_SENSOR);
                web.loadUrl("https://appassets.androidplatform.net/assets/terminal.html");
              }
              ((InputMethodManager) getSystemService(Context.INPUT_METHOD_SERVICE))
                  .hideSoftInputFromWindow(web.getWindowToken(), 0);
            });
          } catch (Exception e) {
            session.close();
            runOnUiThread(() -> { if (epoch == generation && !isDestroyed())
              web.evaluateJavascript("window.connectionFailed&&window.connectionFailed()", null); });
          }
        });
      });
    }

    @JavascriptInterface public void lockLandscape() {
      runOnUiThread(() -> setRequestedOrientation(ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE));
    }

    @JavascriptInterface public void closeTerminal() { runOnUiThread(MainActivity.this::leaveShell); }

    @JavascriptInterface public void request(String payload) {
      final int epoch = generation;
      io.execute(() -> {
        if (epoch != generation) return;
        org.json.JSONObject reply = new org.json.JSONObject();
        try {
          org.json.JSONObject in = new org.json.JSONObject(payload);
          reply.put("id", in.getInt("id"));
          String method = in.getString("method");
          if (terminal == null) throw new Exception("尚未建立 Shell 連線");
          switch (method) {
            case "read": reply.put("result", new org.json.JSONObject(terminal.readJSON(in.getLong("ack")))); break;
            case "write": terminal.write(Base64.decode(in.getString("data"), Base64.DEFAULT)); break;
            case "resize": terminal.resize(in.getInt("columns"), in.getInt("rows")); break;
            default: throw new Exception("不支援的操作");
          }
        } catch (Exception e) {
          try { reply.put("error", "無法連線到遠端，請確認 Host 已啟動且網路可用。"); } catch (Exception ignored) { }
        }
        final String json = reply.toString();
        runOnUiThread(() -> { if (epoch == generation && !isDestroyed())
          web.evaluateJavascript("window.shellReply&&window.shellReply(" + json + ")", null); });
      });
    }
  }

  @Override protected void onDestroy() {
    if (fullscreenExitGesture != null) fullscreenExitGesture.reset();
    if (android.os.Build.VERSION.SDK_INT >= 33 && systemBackCallback != null) {
      getOnBackInvokedDispatcher().unregisterOnBackInvokedCallback(systemBackCallback);
    }
    generation++;
    clearComposedFrame();
    new Thread(viewer::close).start();
    io.shutdown();
    web.destroy();
    super.onDestroy();
  }

  @Override public void onBackPressed() {
    if (desktopFullscreen) setDesktopFullscreen(false);
    else if (inTerminal || inDesktop) leaveShell();
    else super.onBackPressed();
  }
}
