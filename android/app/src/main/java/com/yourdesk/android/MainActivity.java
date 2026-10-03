package com.yourdesk.android;

import android.annotation.SuppressLint;
import android.content.Context;
import android.content.pm.ActivityInfo;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.ImageFormat;
import android.graphics.YuvImage;
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
import android.webkit.WebChromeClient;
import android.webkit.PermissionRequest;
import androidx.camera.core.CameraSelector;
import androidx.camera.core.ImageAnalysis;
import androidx.camera.core.ImageProxy;
import androidx.camera.core.Preview;
import androidx.camera.lifecycle.ProcessCameraProvider;
import androidx.core.content.ContextCompat;
import com.google.mlkit.vision.barcode.BarcodeScanning;
import com.google.mlkit.vision.barcode.BarcodeScanner;
import com.google.mlkit.vision.common.InputImage;
import android.widget.FrameLayout;
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
import androidx.activity.ComponentActivity;

import com.yourdesk.androidcore.core.TerminalSession;
import com.yourdesk.androidcore.core.Viewer;
import com.yourdesk.android.video.DecoderSupport;
import com.yourdesk.android.video.EncodedVideoFrame;
import com.yourdesk.android.video.MediaCodecVideoDecoder;
import com.yourdesk.android.audio.RemoteAudioPlayer;
import com.yourdesk.android.audio.RemoteAudioSession;
import com.yourdesk.android.update.AppUpdater;

/** Android Viewer：WebView 負責介面，原生畫面元件負責合成 JPEG 差分影格。 */
public final class MainActivity extends ComponentActivity {
  private WebView web;
  private View contentRoot;
  private DeltaImageView nativeScreen;
  private TextureView nativeVideo;
  private Surface videoSurface;
  private final MediaCodecVideoDecoder videoDecoder = new MediaCodecVideoDecoder();
  private EncodedVideoFrame pendingVideoFrame;
  private int videoWidth;
  private int videoHeight;
  private boolean videoMode;
  private boolean videoKeyframeRequested;
  private long lastKeyframeRequestMs;
  private final java.util.concurrent.atomic.AtomicBoolean keyframeCommandPending = new java.util.concurrent.atomic.AtomicBoolean();
  private int disabledVideoCodecs;
  private boolean capabilityUpdatePending;
  private boolean capabilityUpdateInFlight;
  private long lastCapabilityUpdateMs;
  private int pumpRenderedFrames;
  private long videoSequence;
  private Button nativeBack;
  private LinearLayout desktopTools;
  private Button scaleButton;
  private Button fullscreenButton;
  private Button keyboardButton;
  private Button viewportButton;
  private Button audioButton;
  private RemoteAudioSession remoteAudio;
  private android.media.AudioManager audioManager;
  private android.media.AudioFocusRequest audioFocus;
  private boolean audioEnabled, audioFocusGranted, audioFocusInterrupted;
  private String audioStatus = "聲音已關閉";
  private final android.content.BroadcastReceiver audioNoisyReceiver = new android.content.BroadcastReceiver() {
    @Override public void onReceive(Context context, android.content.Intent intent) {
      if (android.media.AudioManager.ACTION_AUDIO_BECOMING_NOISY.equals(intent.getAction()) && audioEnabled) {
        setRemoteAudioEnabled(false);
        android.widget.Toast.makeText(MainActivity.this, "音訊裝置已中斷，聲音已關閉", android.widget.Toast.LENGTH_SHORT).show();
      }
    }
  };
  private final DesktopViewport desktopViewport = new DesktopViewport();
  private ViewportGesture viewportGesture;
  private View desktopGestureView;
  private boolean viewportPanMode;
  private boolean remoteScrollMode;
  private int viewportDisplay = Integer.MIN_VALUE;
  private boolean displayKnown, displayPending;
  private boolean displayFrameReady;
  private int displayCount, hostDisplay = -1;
  private volatile boolean displayInputBlocked;
  private volatile int inputDisplay = -1;
  private String lastDisplayState = "", displayError = "";
  private boolean keyboardVisible;
  private int keyboardRequestGeneration;
  private boolean desktopFullscreen;
  private int normalSystemUiVisibility;
  private int desktopHeaderMargin;
  private int normalVisibleBars;
  private int normalBarsBehavior;
  private FullscreenExitGesture fullscreenExitGesture;
  private androidx.activity.OnBackPressedCallback backCallback;
  private android.graphics.Typeface faTypeface;
  private String selectedScale = "1/1", selectedQuality = "標準";
  private String appliedScale = "1/1", appliedQuality = "標準";
  private long streamRequestStartedMs;
  private final android.os.Handler uiHandler = new android.os.Handler(android.os.Looper.getMainLooper());
  private volatile Viewer viewer = new Viewer();
  private volatile TerminalSession terminal;
  private SiteStore siteStore;
  private AppUpdater appUpdater;
  private boolean updatePageIdle, updateConnecting;
  private String updateLanguage = "zh-Hant";
  private boolean foreground;
  private String homeMessage = "";
  private boolean inTerminal;
  private boolean inDesktop;
  private ProcessCameraProvider qrCameraProvider;
  private ImageAnalysis qrAnalysis;
  private BarcodeScanner qrScanner;
  private boolean qrScanning;
  private int qrGeneration;
  private boolean cameraPermissionPending;
  private long lastCameraJpegMs;
  private boolean desktopPageReady;
  // 返回鍵位於所有 WebView/影像層之上；記住按下狀態，避免子 View 在 DOWN/UP
  // 之間切換時吞掉事件，造成需要連按多次才返回。
  private boolean backGesture;
  private boolean backDragging;
  private float backDownX, backDownY, backStartX, backStartY;
  private boolean backPositionDragged;
  private float backRelativeX, backRelativeY;

  private int dp(float value) {
    return Math.round(value * getResources().getDisplayMetrics().density);
  }

  private void layoutBackPosition() {
    if (nativeBack == null || !nativeBack.isShown() || nativeBack.getWidth() == 0) return;
    // 預設位置交給 FrameLayout 錨定；隱藏或旋轉期間不可把舊邊界換成 translation。
    if (!backPositionDragged) {
      nativeBack.setTranslationX(0);
      nativeBack.setTranslationY(0);
      return;
    }
    View parent = (View) nativeBack.getParent();
    float width = Math.max(0, parent.getWidth() - parent.getPaddingLeft() - parent.getPaddingRight() - nativeBack.getWidth());
    float height = Math.max(0, parent.getHeight() - parent.getPaddingTop() - parent.getPaddingBottom() - nativeBack.getHeight());
    nativeBack.setX(parent.getPaddingLeft() + backRelativeX * width);
    nativeBack.setY(parent.getPaddingTop() + backRelativeY * height);
  }

  private void moveBackPosition(float x, float y) {
    View parent = (View) nativeBack.getParent();
    float width = parent.getWidth() - parent.getPaddingLeft() - parent.getPaddingRight() - nativeBack.getWidth();
    float height = parent.getHeight() - parent.getPaddingTop() - parent.getPaddingBottom() - nativeBack.getHeight();
    // 只在使用者拖曳時記錄安全區內的比例，重排不會累積偏移或捨入誤差。
    backPositionDragged = true;
    if (width > 0) backRelativeX = Math.max(0, Math.min(1, (x - parent.getPaddingLeft()) / width));
    if (height > 0) backRelativeY = Math.max(0, Math.min(1, (y - parent.getPaddingTop()) / height));
    layoutBackPosition();
  }

  private void resetBackPosition() {
    backGesture = backDragging = backPositionDragged = false;
    nativeBack.setPressed(false);
    nativeBack.setTranslationX(0);
    nativeBack.setTranslationY(0);
  }

  // 只在 UI thread 存取；每個非 keyframe JPEG 會貼回這張完整畫面。
  private Bitmap composedFrame;
  private int composedWidth;
  private int composedHeight;
  private int composedDisplay = Integer.MIN_VALUE;
  private long composedSequence;
  private final FramePolicy jpegPolicy = new FramePolicy();
  private long statsLastSampleNanos;
  private long statsLastTrafficNanos;
  private long statsFrameCount;
  private long statsLastSent;
  private long statsLastReceived;
  private double statsFps;
  private double statsTx;
  private double statsRx;
  private long lastStreamResultRevision;
  private String lastStreamResult = "";
  private String lastStreamSession = "";

  @SuppressLint("SetJavaScriptEnabled")
  public void onCreate(Bundle b) {
    setRequestedOrientation(ActivityInfo.SCREEN_ORIENTATION_FULL_SENSOR);
    super.onCreate(b);
    siteStore = new SiteStore(getPreferences(0));
    appUpdater = new AppUpdater(this);
    audioEnabled = getPreferences(0).getBoolean("remoteAudioEnabled", false);
    audioManager = (android.media.AudioManager) getSystemService(Context.AUDIO_SERVICE);
    setVolumeControlStream(android.media.AudioManager.STREAM_MUSIC);
    audioFocus = new android.media.AudioFocusRequest.Builder(android.media.AudioManager.AUDIOFOCUS_GAIN)
        .setAudioAttributes(RemoteAudioPlayer.attributes()).setWillPauseWhenDucked(true)
        .setOnAudioFocusChangeListener(this::onAudioFocusChanged, uiHandler).build();
    remoteAudio = new RemoteAudioSession(status -> uiHandler.post(() -> {
      if (!isDestroyed()) {
        audioStatus = remoteAudio.status();
        if (audioEnabled && remoteAudio.unavailable()) {
          abandonAudioFocus(); audioFocusInterrupted = true;
          android.widget.Toast.makeText(this, audioStatus, android.widget.Toast.LENGTH_LONG).show();
        }
        updateAudioButton();
      }
    }));
    ContextCompat.registerReceiver(this, audioNoisyReceiver,
        new android.content.IntentFilter(android.media.AudioManager.ACTION_AUDIO_BECOMING_NOISY), ContextCompat.RECEIVER_NOT_EXPORTED);
    viewportGesture = new ViewportGesture(new ViewportGesture.Listener() {
      @Override public void onBegin() { releaseDesktopTouch(); }
      @Override public void onTransform(float factor, float fromX, float fromY, float toX, float toY) {
        transformViewport(desktopGestureView, factor, fromX, fromY, toX, toY);
      }
    });
    desktopHeaderMargin = dp(44);
    fullscreenExitGesture = new FullscreenExitGesture(this, this::dispatchContentTouchEvent,
        () -> setDesktopFullscreen(false));
    // AndroidX 統一舊版按鍵與 Android 16 預測返回手勢，生命週期結束會自動解除。
    backCallback = new androidx.activity.OnBackPressedCallback(true) {
      @Override public void handleOnBackPressed() { handleBackNavigation(); }
    };
    getOnBackPressedDispatcher().addCallback(this, backCallback);
    Log.i("YourDeskVideo", "Android 解碼器候選=" + DecoderSupport.probe());

    FrameLayout root = new FrameLayout(this);
    root.setMotionEventSplittingEnabled(false);
    contentRoot = root;
    // 所有圖層共用系統安全區，避免狀態列、瀏海與導覽列覆蓋內容。
    ViewCompat.setOnApplyWindowInsetsListener(root, (v, insets) -> {
      // 全螢幕的暫顯系統列應覆蓋畫面，不重新縮小影像；瀏海安全區仍保留。
      Insets bars = insets.getInsets((desktopFullscreen ? 0 : WindowInsetsCompat.Type.systemBars())
          | WindowInsetsCompat.Type.displayCutout());
      // Shell 必須讓出鍵盤空間；GUI 維持影像尺寸，讓鍵盤覆蓋。
      int bottom = !inDesktop
          ? Math.max(bars.bottom, insets.getInsets(WindowInsetsCompat.Type.ime()).bottom)
          : bars.bottom;
      root.setPadding(bars.left, bars.top, bars.right, bottom);
      return insets;
    });
    // 等所有子 View 完成配置再定位，也涵蓋只有 padding／標題列改變的重排。
    root.getViewTreeObserver().addOnGlobalLayoutListener(this::layoutBackPosition);
    SurfaceView surface = new SurfaceView(this);
    surface.setBackgroundColor(Color.BLACK);
    root.addView(surface, new FrameLayout.LayoutParams(-1, -1));

    if ((getApplicationInfo().flags & android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE) != 0) {
      WebView.setWebContentsDebuggingEnabled(true);
    }
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
        return !isLocalAsset(r.getUrl());
      }
      @Override public android.webkit.WebResourceResponse shouldInterceptRequest(WebView v, WebResourceRequest r) {
        if (isLocalAsset(r.getUrl())) return loader.shouldInterceptRequest(r.getUrl());
        return new android.webkit.WebResourceResponse("text/plain", "UTF-8", 403, "Forbidden",
            java.util.Collections.emptyMap(), new java.io.ByteArrayInputStream(new byte[0]));
      }
      @Override public void onPageFinished(WebView view, String url) {
        if (url.endsWith("/index.html") && !homeMessage.isEmpty()) {
          String message = homeMessage; homeMessage = "";
          view.evaluateJavascript("window.showNotice&&window.showNotice(" + org.json.JSONObject.quote(message) + ")", null);
        }
      }
      @Override public boolean onRenderProcessGone(WebView view, android.webkit.RenderProcessGoneDetail detail) {
        // 系統已終止 renderer；由 Activity 重建所有本地頁面與 native session。
        updatePageIdle = false;
        refreshAppUpdater();
        generation++;
        inDesktop = false;
        inTerminal = false;
        desktopPageReady = false;
        stopQrScannerOnMain();
        uiHandler.removeCallbacksAndMessages(null);
        new Thread(viewer::close).start();
        view.removeJavascriptInterface("YourDesk");
        if (view.getParent() instanceof android.view.ViewGroup) ((android.view.ViewGroup) view.getParent()).removeView(view);
        view.destroy(); web = null;
        recreate();
        return true;
      }
    });
    web.setWebChromeClient(new WebChromeClient() {
      @Override public void onPermissionRequest(PermissionRequest request) {
        runOnUiThread(request::deny);
      }
    });
    root.addView(web, new FrameLayout.LayoutParams(-1, -1));

    nativeVideo = new TextureView(this);
    // 等比例顯示可能留下黑邊，TextureView 不可宣告整個區域皆為不透明。
    nativeVideo.setOpaque(false);
    nativeVideo.setSurfaceTextureListener(new TextureView.SurfaceTextureListener() {
      @Override public void onSurfaceTextureAvailable(android.graphics.SurfaceTexture texture, int width, int height) {
        if (videoSurface != null) videoSurface.release();
        videoSurface = new Surface(texture);
        if (pendingVideoFrame != null) presentVideoFrame(pendingVideoFrame);
        if (videoMode && videoDecoder.needsKeyframe()) requestVideoKeyframe();
      }

      @Override public void onSurfaceTextureSizeChanged(android.graphics.SurfaceTexture texture, int width, int height) {
        updateVideoTransform();
      }

      @Override public boolean onSurfaceTextureDestroyed(android.graphics.SurfaceTexture texture) {
        videoDecoder.reset();
        if (videoMode) { displayFrameReady = false; updateDisplayInputGate(); }
        videoKeyframeRequested = false;
        lastKeyframeRequestMs = 0;
        if (videoSurface != null) {
          videoSurface.release();
          videoSurface = null;
        }
        return true;
      }

      @Override public void onSurfaceTextureUpdated(android.graphics.SurfaceTexture texture) { }
    });
    nativeVideo.setOnTouchListener(this::handleDesktopTouch);
    nativeVideo.setOnGenericMotionListener(this::handleDesktopMouse);
    nativeVideo.setClickable(true);
    nativeVideo.setVisibility(View.GONE);
    FrameLayout.LayoutParams videoLp = new FrameLayout.LayoutParams(-1, -1);
    videoLp.gravity = Gravity.TOP | Gravity.LEFT;
    videoLp.topMargin = desktopHeaderMargin;
    videoLp.bottomMargin = 0;
    root.addView(nativeVideo, videoLp);

    nativeScreen = new DeltaImageView(this, desktopViewport);
    nativeScreen.setBackgroundColor(Color.BLACK);
    // DeltaImageView 自行以等比例矩形繪製；不能使用 FIT_XY，否則來源畫面會被拉伸。
    nativeScreen.setOnTouchListener(this::handleDesktopTouch);
    nativeScreen.setOnGenericMotionListener(this::handleDesktopMouse);
    nativeScreen.setClickable(true);
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
    FrameLayout.LayoutParams backLp = new FrameLayout.LayoutParams(dp(36), dp(36), Gravity.TOP | Gravity.RIGHT);
    // 與 WebView 標題列共用高度，在系統安全區內置中。
    backLp.topMargin = Math.max(0, (desktopHeaderMargin - backLp.height) / 2);
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
    viewportButton = toolButton("100%", "檢視縮放與拖移");
    viewportButton.setTypeface(android.graphics.Typeface.DEFAULT);
    viewportButton.setTextSize(10);
    viewportButton.setOnClickListener(this::showViewportMenu);
    audioButton = toolButton("\uf6a9", "開啟遠端聲音");
    audioButton.setOnClickListener(v -> setRemoteAudioEnabled(!audioEnabled));
    audioButton.setOnLongClickListener(v -> {
      android.widget.Toast.makeText(this, audioStatus, android.widget.Toast.LENGTH_LONG).show(); return true;
    });
    desktopTools.addView(fullscreenButton); desktopTools.addView(scaleButton);
    desktopTools.addView(quality); desktopTools.addView(viewportButton); desktopTools.addView(audioButton); desktopTools.addView(keyboardButton);
    desktopTools.setVisibility(View.GONE);
    FrameLayout.LayoutParams toolsLp = new FrameLayout.LayoutParams(dp(46), dp(294), Gravity.START | Gravity.CENTER_VERTICAL);
    toolsLp.leftMargin = dp(8);
    root.addView(desktopTools, toolsLp);

    setContentView(root);
    ViewCompat.requestApplyInsets(contentRoot);
    web.loadUrl("https://appassets.androidplatform.net/assets/index.html");
  }

  private void setRemoteAudioEnabled(boolean enabled) {
    audioEnabled = enabled;
    audioFocusInterrupted = false;
    getPreferences(0).edit().putBoolean("remoteAudioEnabled", enabled).apply();
    refreshRemoteAudio();
  }

  private void onAudioFocusChanged(int change) {
    audioFocusGranted = change == android.media.AudioManager.AUDIOFOCUS_GAIN;
    audioFocusInterrupted = !audioFocusGranted;
    refreshRemoteAudio();
  }

  private void refreshRemoteAudio() {
    if (remoteAudio == null) return;
    boolean wanted = audioEnabled && foreground && inDesktop && desktopPageReady;
    if (wanted && !audioFocusGranted && !audioFocusInterrupted) {
      audioFocusGranted = audioManager.requestAudioFocus(audioFocus) == android.media.AudioManager.AUDIOFOCUS_REQUEST_GRANTED;
      if (!audioFocusGranted) audioFocusInterrupted = true;
    }
    if (!wanted) abandonAudioFocus();
    String profile = "低畫質".equals(selectedQuality) ? "fast" : "高畫質".equals(selectedQuality) ? "high" : "standard";
    remoteAudio.setPlayback(inDesktop ? viewer : null, wanted && audioFocusGranted, profile);
    updateAudioButton();
  }

  private void abandonAudioFocus() {
    if (audioManager != null && audioFocus != null) audioManager.abandonAudioFocusRequest(audioFocus);
    audioFocusGranted = false; audioFocusInterrupted = false;
  }

  private void updateAudioButton() {
    if (audioButton == null) return;
    String status = !audioEnabled ? "聲音已關閉" : remoteAudio.unavailable() ? audioStatus
        : !foreground || !audioFocusGranted ? "聲音已暫停" : audioStatus;
    audioButton.setText(audioEnabled ? "\uf028" : "\uf6a9");
    audioButton.setSelected(audioEnabled);
    audioButton.setTextColor(audioEnabled ? (status.contains("播放中") ? Color.rgb(102, 220, 170) : Color.rgb(255, 195, 80)) : Color.WHITE);
    audioButton.setContentDescription((audioEnabled ? "關閉" : "開啟") + "遠端聲音；" + status);
    audioButton.setTooltipText(status + "；點一下切換，使用手機音量鍵調整音量");
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

  // Shell 需要系統重新分配鍵盤上方的可用高度；GUI 保留完整畫面。
  private void updateKeyboardLayout() {
    getWindow().setSoftInputMode(inTerminal
        ? WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE
        : WindowManager.LayoutParams.SOFT_INPUT_ADJUST_NOTHING);
    ViewCompat.requestApplyInsets(contentRoot);
  }

  private void showDesktopKeyboard() {
    if (web == null) return;
    updateKeyboardLayout();
    final int request = ++keyboardRequestGeneration;
    web.setFocusableInTouchMode(true);
    web.requestFocus();
    web.postDelayed(() -> {
      if (request != keyboardRequestGeneration || (!inDesktop && !inTerminal)
          || (inDesktop && desktopFullscreen)) return;
      InputMethodManager imm = (InputMethodManager) getSystemService(Context.INPUT_METHOD_SERVICE);
      web.evaluateJavascript("document.getElementById('keyboard-proxy')?.focus();document.getElementById('input-proxy')?.focus()",
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
    cancelDesktopGesture();
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
      // View 填滿可用區，JPEG 與影片共用各自的 contain 矩形作繪製與命中判斷。
      lp.width = -1;
      lp.height = -1;
      lp.leftMargin = 0;
      lp.topMargin = top;
      {
        lp.gravity = gravity;
        screen.setLayoutParams(lp);
      }
    }
    if (nativeBack != null) {
      FrameLayout.LayoutParams lp = (FrameLayout.LayoutParams) nativeBack.getLayoutParams();
      int margin = Math.max(0, (desktopHeaderMargin - lp.height) / 2);
      if (lp.topMargin != margin) {
        lp.topMargin = margin;
        nativeBack.setLayoutParams(lp);
      }
    }
  }
  private boolean sendStreamPreference(String scale, String quality) {
    String actualQuality = quality == null ? selectedQuality : quality;
    String actualScale = scale == null ? selectedScale : scale;
    String profile = "低畫質".equals(actualQuality) ? "fast" : "高畫質".equals(actualQuality) ? "high" : "standard";
    int percent = "3/4".equals(actualScale) ? 75 : "3/5".equals(actualScale) ? 60 : "1/2".equals(actualScale) ? 50 : 100;
    try {
      viewer.sendStreamPreferences(profile, percent);
      selectedScale = actualScale;
      selectedQuality = actualQuality;
      streamRequestStartedMs = android.os.SystemClock.uptimeMillis();
      scaleButton.setText("\uf390  " + selectedScale + " …");
      refreshRemoteAudio();
      return true;
    }
    catch (Exception error) {
      android.widget.Toast.makeText(this, "Host 尚未接受串流設定", android.widget.Toast.LENGTH_SHORT).show();
      return false;
    }
  }

  private void showScaleMenu(View anchor) {
    PopupMenu menu = new PopupMenu(this, anchor);
    for (String value : new String[]{"1/1", "3/4", "3/5", "1/2"}) { android.view.MenuItem i = menu.getMenu().add(value); i.setCheckable(true).setChecked(value.equals(selectedScale)); }
    menu.setOnMenuItemClickListener(item -> { sendStreamPreference(item.getTitle().toString(), null); return true; });
    menu.show();
  }
  private void showQualityMenu(View anchor) {
    PopupMenu menu = new PopupMenu(this, anchor);
    for (String value : new String[]{"低畫質", "標準", "高畫質"}) { android.view.MenuItem i = menu.getMenu().add(value); i.setCheckable(true).setChecked(value.equals(selectedQuality)); }
    menu.setOnMenuItemClickListener(item -> { sendStreamPreference(null, item.getTitle().toString()); return true; });
    menu.show();
  }

  private void showViewportMenu(View anchor) {
    PopupMenu menu = new PopupMenu(this, anchor);
    menu.getMenu().add(0, 1, 0, "放大 ＋").setEnabled(desktopViewport.zoom() < DesktopViewport.MAX_ZOOM);
    menu.getMenu().add(0, 2, 1, "縮小 −").setEnabled(desktopViewport.zoom() > 1f);
    menu.getMenu().add(0, 3, 2, "適合畫面（100%）");
    menu.getMenu().add(0, 4, 3, "單指拖移可視區").setCheckable(true).setChecked(viewportPanMode);
    menu.getMenu().add(0, 6, 4, "單指捲動遠端內容").setCheckable(true).setChecked(remoteScrollMode);
    menu.setOnMenuItemClickListener(item -> {
      cancelDesktopGesture();
      if (item.getItemId() == 3) { desktopViewport.reset(); refreshViewport(); }
      else if (item.getItemId() == 4) { viewportPanMode = !viewportPanMode; remoteScrollMode = false; refreshViewport(); }
      else if (item.getItemId() == 6) { remoteScrollMode = !remoteScrollMode; viewportPanMode = false; refreshViewport(); }
      else {
        View view = videoMode ? nativeVideo : nativeScreen;
        transformViewport(view, item.getItemId() == 1 ? 1.25f : .8f,
            view.getWidth() * .5f, view.getHeight() * .5f, view.getWidth() * .5f, view.getHeight() * .5f);
      }
      return true;
    });
    menu.show();
  }

  private void transformViewport(View view, float factor, float fromX, float fromY, float toX, float toY) {
    if (view == null) return;
    int width = view == nativeVideo ? videoWidth : composedWidth;
    int height = view == nativeVideo ? videoHeight : composedHeight;
    desktopViewport.transform(width, height, view.getWidth(), view.getHeight(), factor, fromX, fromY, toX, toY);
    refreshViewport();
  }

  private void refreshViewport() {
    if (nativeScreen != null) nativeScreen.invalidate();
    updateVideoTransform();
    if (viewportButton != null) {
      String value = Math.round(desktopViewport.zoom() * 100) + "%";
      String mode = viewportPanMode ? "單指拖移可視區" : remoteScrollMode ? "單指捲動遠端內容" : "單指操作遠端，長按右鍵";
      viewportButton.setText(viewportPanMode ? value + "\n拖移" : remoteScrollMode ? value + "\n捲動" : value);
      viewportButton.setContentDescription("檢視縮放 " + value + "，" + mode);
      viewportButton.setTooltipText("檢視與操作模式：" + mode);
    }
  }

  private void updateViewportDisplay(int display) {
    if (viewportDisplay != Integer.MIN_VALUE && viewportDisplay != display) {
      cancelDesktopGesture();
      desktopViewport.reset();
      refreshViewport();
    }
    viewportDisplay = display;
    inputDisplay = display;
    updateDisplayInputGate();
  }

  private void updateDisplayInputGate() {
    displayInputBlocked = displayPending || (displayKnown && (hostDisplay < 0 || viewportDisplay != hostDisplay || !displayFrameReady));
  }

  private void markDisplayFramePresented() {
    if (!displayPending && (!displayKnown || viewportDisplay == hostDisplay)) displayFrameReady = true;
    updateDisplayInputGate();
  }

  private int drainVideoFrames() {
    int rendered = videoDecoder.drainOutput();
    if (rendered > 0) markDisplayFramePresented();
    return rendered;
  }

  private void updateDisplayState(Viewer session) {
    try {
      String json = session.displayStateJSON();
      org.json.JSONObject state = new org.json.JSONObject(json);
      boolean known = state.optBoolean("known"), pending = state.optBoolean("pending");
      int current = state.optInt("current", -1);
      if (pending != displayPending || (known && current != hostDisplay)) {
        cancelDesktopGesture();
        if (pending || current != hostDisplay) displayFrameReady = false;
      }
      displayKnown = known; displayPending = pending;
      hostDisplay = current; displayCount = state.optInt("count");
      updateDisplayInputGate();
      state.put("waitingFrame", known && !pending && current >= 0 && displayInputBlocked);
      json = state.toString();
      String error = state.optString("error");
      if (!error.isEmpty() && !error.equals(displayError))
        android.widget.Toast.makeText(this, error, android.widget.Toast.LENGTH_LONG).show();
      displayError = error;
      if (!json.equals(lastDisplayState)) {
        lastDisplayState = json;
        web.evaluateJavascript("window.desktopDisplays&&window.desktopDisplays(" + json + ")", null);
      }
    } catch (Exception ignored) { }
  }

  private void selectRemoteDisplay(int display) {
    if (!inDesktop || !desktopPageReady || !displayKnown || display < 0 || display >= displayCount) return;
    cancelDesktopGesture();
    web.evaluateJavascript("typeof releaseKeys==='function'&&releaseKeys()", null);
    try {
      viewer.selectDisplay(display);
      updateDisplayState(viewer);
    } catch (Exception error) {
      android.widget.Toast.makeText(this, "無法切換螢幕，請稍後重試", android.widget.Toast.LENGTH_SHORT).show();
    }
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
              moveBackPosition(backStartX + dx, backStartY + dy);
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

  private View desktopTouchOwner;
  private int desktopPointerId = -1;
  private float desktopTouchX, desktopTouchY;
  private float desktopDownX, desktopDownY;
  private boolean desktopPointerPressed;
  private boolean desktopRightClick, desktopScrolling;
  private float desktopScrollY, scrollRemainder, mouseScrollRemainder;
  private Runnable desktopLongPress;
  private int mouseButtons;
  private boolean mouseGestureCancelled;
  private float mouseX, mouseY;

  private void cancelDesktopGesture() {
    releaseDesktopTouch();
    releaseMouseButtons();
    if (viewportGesture != null) viewportGesture.reset();
    desktopGestureView = null;
  }

  private void releaseDesktopTouch() {
    cancelDesktopLongPress();
    if (desktopTouchOwner == null) return;
    desktopTouchOwner = null;
    desktopPointerId = -1;
    if (desktopPointerPressed) sendDesktopPointer("button", false, desktopTouchX, desktopTouchY);
    desktopPointerPressed = false;
    desktopRightClick = desktopScrolling = false;
    scrollRemainder = 0;
  }

  private void cancelDesktopLongPress() {
    if (desktopLongPress != null) uiHandler.removeCallbacks(desktopLongPress);
    desktopLongPress = null;
  }

  private void sendDesktopPointer(String type, boolean down, float x, float y) {
    sendDesktopPointer(type, 1, down, x, y, 0);
  }

  private void sendDesktopPointer(String type, int button, boolean down, float x, float y, int delta) {
    try {
      org.json.JSONObject c = new org.json.JSONObject().put("type", type).put("x", x).put("y", y);
      if (inputDisplay >= 0) c.put("display", inputDisplay);
      if (type.equals("button")) c.put("button", button).put("down", down);
      if (type.equals("wheel")) c.put("delta", delta);
      viewer.sendControlJSON(c.toString());
    } catch (Exception ignored) {
      // 連線中止時不讓 UI thread 崩潰。
    }
  }

  private float[] desktopPoint(View view, float x, float y) {
    if (!inDesktop || !view.isShown() || !hasWindowFocus() || displayInputBlocked) return null;
    Rect bounds = new Rect();
    if (!view.getLocalVisibleRect(bounds) || x < bounds.left || x >= bounds.right || y < bounds.top || y >= bounds.bottom) return null;
    int[] origin = new int[2]; view.getLocationOnScreen(origin);
    for (View overlay : new View[]{desktopTools, nativeBack}) {
      if (overlay != null && overlay.isShown() && overlay.getGlobalVisibleRect(bounds)
          && bounds.contains(Math.round(origin[0] + x), Math.round(origin[1] + y))) return null;
    }
    if (view instanceof DeltaImageView) return ((DeltaImageView) view).mapTouch(x, y);
    return view == nativeVideo ? mapVideoTouch(x, y) : null;
  }

  private void sendDesktopWheel(float x, float y, int delta) {
    if (delta == 0) return;
    // Host 的 wheel 作用於游標所在視窗，先定位到手勢起點再捲動。
    sendDesktopPointer("move", false, x, y);
    sendDesktopPointer("wheel", 0, false, x, y, Math.max(-12, Math.min(12, delta)));
  }

  private void syncMouseButtons(int buttons, float x, float y) {
    buttons &= MotionEvent.BUTTON_PRIMARY | MotionEvent.BUTTON_SECONDARY | MotionEvent.BUTTON_TERTIARY;
    int changed = mouseButtons ^ buttons;
    int[] masks = {MotionEvent.BUTTON_PRIMARY, MotionEvent.BUTTON_SECONDARY, MotionEvent.BUTTON_TERTIARY};
    for (int i = 0; i < masks.length; i++) if ((changed & masks[i]) != 0)
      sendDesktopPointer("button", i + 1, (buttons & masks[i]) != 0, x, y, 0);
    mouseButtons = buttons; mouseX = x; mouseY = y;
  }

  private void releaseMouseButtons() {
    if (mouseButtons != 0) { mouseGestureCancelled = true; syncMouseButtons(0, mouseX, mouseY); }
    mouseScrollRemainder = 0;
  }

  private boolean handleDesktopMouse(View view, MotionEvent event) {
    if (!event.isFromSource(android.view.InputDevice.SOURCE_MOUSE)) return false;
    int action = event.getActionMasked();
    float[] point = desktopPoint(view, event.getX(), event.getY());
    if (point == null || action == MotionEvent.ACTION_CANCEL) {
      releaseMouseButtons();
      mouseGestureCancelled |= event.getButtonState() != 0;
      return true;
    }
    if (action == MotionEvent.ACTION_DOWN) mouseGestureCancelled = false;
    if (mouseGestureCancelled) {
      if (event.getButtonState() == 0) mouseGestureCancelled = false;
      return true;
    }
    if (action == MotionEvent.ACTION_BUTTON_PRESS || action == MotionEvent.ACTION_BUTTON_RELEASE
        || action == MotionEvent.ACTION_DOWN || action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_MOVE) {
      int buttons = action == MotionEvent.ACTION_UP ? 0 : event.getButtonState();
      if (action == MotionEvent.ACTION_DOWN && buttons == 0) buttons = MotionEvent.BUTTON_PRIMARY;
      syncMouseButtons(buttons, point[0], point[1]);
    }
    if (action == MotionEvent.ACTION_MOVE || action == MotionEvent.ACTION_HOVER_MOVE)
      sendDesktopPointer("move", false, point[0], point[1]);
    if (action == MotionEvent.ACTION_SCROLL) {
      float delta = event.getAxisValue(MotionEvent.AXIS_VSCROLL);
      if (Float.isFinite(delta)) {
        mouseScrollRemainder += Math.max(-12, Math.min(12, delta));
        int steps = (int) mouseScrollRemainder; mouseScrollRemainder -= steps;
        sendDesktopWheel(point[0], point[1], steps);
      }
    }
    return true;
  }

  private boolean handleDesktopTouch(View v, MotionEvent e) {
    if (e.isFromSource(android.view.InputDevice.SOURCE_MOUSE)) return handleDesktopMouse(v, e);
    int action = e.getActionMasked();
    if (action == MotionEvent.ACTION_DOWN) { cancelDesktopGesture(); desktopGestureView = v; }
    if (!inDesktop || !v.isShown() || !hasWindowFocus() || desktopGestureView != v) {
      cancelDesktopGesture(); return true;
    }
    if (viewportGesture.onTouch(e, viewportPanMode)) {
      if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) desktopGestureView = null;
      return true;
    }
    int index = action == MotionEvent.ACTION_DOWN ? e.getActionIndex() : e.findPointerIndex(desktopPointerId);
    // 觸控、滑鼠與滾輪共用來源座標、遮罩邊界及螢幕切換檢查。
    float[] point = index >= 0 ? desktopPoint(v, e.getX(index), e.getY(index)) : null;
    if (point == null || action == MotionEvent.ACTION_CANCEL) {
      // 越界只在最後有效位置釋放；再次進入必須重新按下。
      releaseDesktopTouch();
      return true;
    }
    if (action == MotionEvent.ACTION_DOWN) {
      desktopTouchOwner = v;
      desktopPointerId = e.getPointerId(index);
      desktopTouchX = point[0]; desktopTouchY = point[1];
      desktopDownX = e.getX(index); desktopDownY = e.getY(index);
      desktopScrollY = desktopDownY;
      if (!remoteScrollMode) {
        final int epoch = generation;
        desktopLongPress = () -> {
          desktopLongPress = null;
          if (epoch != generation || desktopTouchOwner != v || desktopPointerPressed || displayInputBlocked
              || !hasWindowFocus() || !v.isShown()) return;
          desktopRightClick = true;
          sendDesktopPointer("button", 2, true, desktopTouchX, desktopTouchY, 0);
          sendDesktopPointer("button", 2, false, desktopTouchX, desktopTouchY, 0);
          v.performHapticFeedback(android.view.HapticFeedbackConstants.LONG_PRESS);
        };
        uiHandler.postDelayed(desktopLongPress, android.view.ViewConfiguration.getLongPressTimeout());
      }
    } else if (desktopTouchOwner == v) {
      float distance = (float) Math.hypot(e.getX(index) - desktopDownX, e.getY(index) - desktopDownY);
      if (distance > android.view.ViewConfiguration.get(this).getScaledTouchSlop()) cancelDesktopLongPress();
      if (desktopRightClick) {
        if (action == MotionEvent.ACTION_UP) releaseDesktopTouch();
        return true;
      }
      if (remoteScrollMode) {
        desktopScrolling |= distance > android.view.ViewConfiguration.get(this).getScaledTouchSlop();
        if (desktopScrolling && action == MotionEvent.ACTION_MOVE) {
          scrollRemainder += (e.getY(index) - desktopScrollY) / dp(24);
          int steps = (int) scrollRemainder; scrollRemainder -= steps;
          sendDesktopWheel(desktopTouchX, desktopTouchY, steps);
        }
        desktopScrollY = e.getY(index);
        if (action == MotionEvent.ACTION_UP) releaseDesktopTouch();
        return true;
      }
      if (!desktopPointerPressed && (action == MotionEvent.ACTION_UP
          || distance > android.view.ViewConfiguration.get(this).getScaledTouchSlop())) {
        desktopPointerPressed = true;
        sendDesktopPointer("button", true, desktopTouchX, desktopTouchY);
      }
      if (desktopPointerPressed) {
        desktopTouchX = point[0]; desktopTouchY = point[1];
        if (action == MotionEvent.ACTION_UP) releaseDesktopTouch();
        else if (action == MotionEvent.ACTION_MOVE) sendDesktopPointer("move", false, desktopTouchX, desktopTouchY);
      }
    }
    return true;
  }

  @Override public void onWindowFocusChanged(boolean hasFocus) {
    super.onWindowFocusChanged(hasFocus);
    if (!hasFocus) cancelDesktopGesture();
  }

  private final java.util.concurrent.ExecutorService io = java.util.concurrent.Executors.newSingleThreadExecutor();
  private volatile int generation;

  private void clearComposedFrame() {
    if (remoteAudio != null) remoteAudio.setPlayback(null, false, "standard");
    abandonAudioFocus();
    cancelDesktopGesture();
    desktopViewport.reset();
    viewportPanMode = false;
    remoteScrollMode = false;
    viewportDisplay = Integer.MIN_VALUE;
    inputDisplay = -1;
    displayKnown = displayPending = displayInputBlocked = false;
    displayFrameReady = false;
    displayCount = 0; hostDisplay = -1;
    lastDisplayState = displayError = "";
    refreshViewport();
    if (composedFrame != null && !composedFrame.isRecycled()) composedFrame.recycle();
    composedFrame = null;
    composedWidth = composedHeight = 0;
    composedDisplay = Integer.MIN_VALUE;
    composedSequence = 0;
    jpegPolicy.reset();
    resetStats();
    if (nativeScreen != null) nativeScreen.clearFrame();
    pendingVideoFrame = null;
    videoWidth = videoHeight = 0;
    videoMode = false;
    videoKeyframeRequested = false;
    lastKeyframeRequestMs = 0;
    disabledVideoCodecs = 0;
    capabilityUpdatePending = capabilityUpdateInFlight = false;
    lastCapabilityUpdateMs = 0;
    lastStreamResultRevision = 0;
    lastStreamResult = "";
    lastStreamSession = "";
    streamRequestStartedMs = 0;
    selectedScale = appliedScale = "1/1";
    selectedQuality = appliedQuality = "標準";
    if (scaleButton != null) scaleButton.setText("\uf390  " + selectedScale);
    videoSequence = 0;
    videoDecoder.resetSession();
    if (nativeVideo != null) nativeVideo.setVisibility(View.GONE);
  }

  /** TextureView 與 JPEG 使用相同可視區，影片與觸控共用縮放／平移轉換。 */
  private void updateVideoTransform() {
    if (nativeVideo == null || videoWidth <= 0 || videoHeight <= 0) return;
    int viewWidth = nativeVideo.getWidth();
    int viewHeight = nativeVideo.getHeight();
    if (viewWidth <= 0 || viewHeight <= 0) return;
    DesktopViewport.Geometry area = desktopViewport.geometry(videoWidth, videoHeight, viewWidth, viewHeight);
    Matrix matrix = new Matrix();
    matrix.setScale(area.width / viewWidth, area.height / viewHeight);
    matrix.postTranslate(area.left, area.top);
    nativeVideo.setTransform(matrix);
    // 靜止桌面可能沒有下一張解碼影格，仍須立即重繪新的可視區。
    nativeVideo.invalidate();
  }

  private float[] mapVideoTouch(float x, float y) {
    if (videoWidth <= 0 || videoHeight <= 0 || nativeVideo == null
        || nativeVideo.getWidth() <= 0 || nativeVideo.getHeight() <= 0) return null;
    return desktopViewport.geometry(videoWidth, videoHeight, nativeVideo.getWidth(), nativeVideo.getHeight()).mapTouch(x, y);
  }

  private boolean presentVideoFrame(EncodedVideoFrame frame) {
    if ((disabledVideoCodecs & (1 << frame.codec)) != 0) return false;
    pendingVideoFrame = frame;
    videoMode = true;
    videoWidth = frame.width;
    videoHeight = frame.height;
    updateVideoTransform();
    if (videoSurface == null || !videoSurface.isValid()) return false;
    int rendered = videoDecoder.queue(frame, videoSurface);
    if (rendered < 0) {
      withdrawFailedVideoCodec();
      requestVideoKeyframe();
      return false;
    }
    if (videoDecoder.needsKeyframe()) requestVideoKeyframe();
    pumpRenderedFrames += rendered;
    if (rendered > 0 && web != null) {
      String label = frame.codec == 1 ? "H.264" : "HEVC";
      web.evaluateJavascript("window.desktopFrameStatus&&window.desktopFrameStatus(" + frame.width + "," + frame.height + ",false,'" + label + "'," + viewportDisplay + ")", null);
    }
    if (rendered > 0) { videoKeyframeRequested = false; markDisplayFramePresented(); }
    return rendered > 0;
  }

  /** 解碼器重建或 Surface 回來後，請 Host 優先送出新的 IDR。 */
  private void requestVideoKeyframe() {
    long now = android.os.SystemClock.uptimeMillis();
    if (viewer == null || !inDesktop || isDestroyed() || io.isShutdown()
        || (lastKeyframeRequestMs != 0 && now - lastKeyframeRequestMs < 1500)
        || !keyframeCommandPending.compareAndSet(false, true)) return;
    final Viewer session = viewer;
    final int epoch = generation;
    lastKeyframeRequestMs = now;
    videoKeyframeRequested = true;
    io.execute(() -> {
      try {
        if (session == viewer && epoch == generation) session.callCommand("video.keyframe");
      } catch (Exception ignored) {
        // 有界節流重試；遺失一次要求不能永久卡在等待 IDR。
      } finally {
        keyframeCommandPending.set(false);
      }
    });
  }

  private void withdrawFailedVideoCodec() {
    int failed = videoDecoder.failedWireCodec();
    if (failed <= 0 || (disabledVideoCodecs & (1 << failed)) != 0) return;
    disabledVideoCodecs |= 1 << failed;
    pendingVideoFrame = null;
    capabilityUpdatePending = true;
    sendPendingVideoCapabilities();
  }

  private void sendPendingVideoCapabilities() {
    long now = android.os.SystemClock.uptimeMillis();
    if (!capabilityUpdatePending || capabilityUpdateInFlight || io.isShutdown()
        || (lastCapabilityUpdateMs != 0 && now - lastCapabilityUpdateMs < 1500)) return;
    capabilityUpdateInFlight = true;
    lastCapabilityUpdateMs = now;
    final int sentMask = disabledVideoCodecs;
    byte[] supported = filterVideoCodecs(DecoderSupport.supportedWireCodecs());
    byte[] hardware = filterVideoCodecs(DecoderSupport.hardwareWireCodecs());
    final Viewer session = viewer;
    final int epoch = generation;
    io.execute(() -> {
      boolean sent = false;
      try {
        if (session == viewer && epoch == generation) {
          session.sendVideoCapabilities(supported, hardware);
          sent = true;
        }
      } catch (Exception error) { Log.w("YourDeskVideo", "解碼降級協商失敗，稍後重試", error); }
      final boolean succeeded = sent;
      uiHandler.post(() -> {
        if (session != viewer || epoch != generation || isDestroyed()) return;
        capabilityUpdateInFlight = false;
        if (succeeded && sentMask == disabledVideoCodecs) capabilityUpdatePending = false;
      });
    });
  }

  private byte[] filterVideoCodecs(byte[] codecs) {
    java.io.ByteArrayOutputStream result = new java.io.ByteArrayOutputStream();
    for (byte codec : codecs) if ((disabledVideoCodecs & (1 << codec)) == 0) result.write(codec);
    return result.toByteArray();
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
    updatePageIdle = false;
    updateConnecting = false;
    refreshAppUpdater();
    updateKeyboardLayout();
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
    getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
    web.loadUrl("https://appassets.androidplatform.net/assets/index.html");
  }

  private void endSession(String message) {
    homeMessage = message;
    leaveShell();
  }

  private void startNativeFramePump(Viewer session, int epoch) {
    final long started = android.os.SystemClock.uptimeMillis();
    uiHandler.post(new Runnable() {
      @Override public void run() {
        if (epoch != generation || !inDesktop || session != viewer || isDestroyed()) return;
        if (!foreground) { uiHandler.postDelayed(this, 250); return; }
        if (!session.isConnected()) { endSession("遠端已斷線，請重新連線。"); return; }
        if (!desktopPageReady && android.os.SystemClock.uptimeMillis() - started > 30000) {
          endSession("連線已建立，但未收到桌面影像。請確認 Host 的螢幕擷取權限後重試。"); return;
        }
        pumpRenderedFrames = 0;
        updateDisplayState(session);
        sendPendingVideoCapabilities();
        if (session.consumeFrameRecoveryRequest()) {
          jpegPolicy.invalidate();
          videoDecoder.reset();
          requestVideoKeyframe();
        }
        if (videoMode) {
          int drained = drainVideoFrames();
          if (drained < 0) { withdrawFailedVideoCodec(); requestVideoKeyframe(); }
          else pumpRenderedFrames += drained;
          if (videoDecoder.needsKeyframe()) requestVideoKeyframe();
        } else if (jpegPolicy.needsKeyframe()) requestVideoKeyframe();
        // 限制單輪工作量，讓觸控／生命週期事件有機會執行；Go 端另有 bytes 上限。
        boolean presented = false;
        long deadline = System.nanoTime() + 8_000_000L;
        for (int i = 0; i < 4 && System.nanoTime() < deadline; i++) {
          String json;
          try {
            json = session.readFrameJSON();
          } catch (Exception ignored) {
            json = "";
          }
          if (json == null || json.isEmpty()) break;
          presented |= applyFrameJSON(json);
        }
        if ((presented || videoMode || composedFrame != null) && inDesktop && !desktopPageReady) showDesktopPage(session, epoch);
        updateDesktopStats(session, pumpRenderedFrames);
        uiHandler.postDelayed(this, 16);
      }
    });
  }

  private void showDesktopPage(Viewer session, int epoch) {
    if (epoch != generation || session != viewer || !inDesktop || desktopPageReady) return;
    desktopPageReady = true;
    nativeScreen.setVisibility(videoMode ? View.GONE : View.VISIBLE);
    nativeVideo.setVisibility(videoMode ? View.VISIBLE : View.GONE);
    updateVideoTransform();
    resetBackPosition();
    nativeBack.setVisibility(View.VISIBLE);
    desktopTools.setVisibility(View.VISIBLE);
    refreshRemoteAudio();
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

  private void updateDesktopStats(Viewer session, int renderedFrames) {
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
    statsFrameCount += renderedFrames;
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
    updateStreamResult(session);
    web.evaluateJavascript("window.desktopInputCapabilities&&window.desktopInputCapabilities("
        + session.supportsCommand("input.text-capabilities") + ")", null);
    web.evaluateJavascript("window.desktopStats&&window.desktopStats(" + statsFps + "," + statsTx + "," + statsRx + ")", null);
  }

  private void updateStreamResult(Viewer session) {
    try {
      org.json.JSONObject state = new org.json.JSONObject(session.streamPreferencesJSON());
      org.json.JSONObject result = state.optJSONObject("result");
      if (result != null && !result.optString("sessionId").equals(lastStreamSession)) {
        lastStreamSession = result.optString("sessionId");
        lastStreamResultRevision = 0;
        lastStreamResult = "";
      }
      if (state.optBoolean("pending") && streamRequestStartedMs != 0
          && android.os.SystemClock.uptimeMillis() - streamRequestStartedMs > 5000) {
        streamRequestStartedMs = 0;
        scaleButton.setText("\uf390  " + selectedScale + " ?");
        android.widget.Toast.makeText(this, "尚未收到 Host 串流設定確認", android.widget.Toast.LENGTH_SHORT).show();
      }
      if (result == null || result.optLong("revision") < lastStreamResultRevision
          || result.toString().equals(lastStreamResult)) return;
      lastStreamResultRevision = result.getLong("revision");
      lastStreamResult = result.toString();
      streamRequestStartedMs = 0;
      if (!result.optBoolean("accepted")) {
        selectedScale = appliedScale;
        selectedQuality = appliedQuality;
        android.widget.Toast.makeText(this, "Host 拒絕串流設定：" + result.optString("error"), android.widget.Toast.LENGTH_LONG).show();
      } else {
        org.json.JSONObject request = state.optJSONObject("request");
        if (request != null && request.optLong("revision") == lastStreamResultRevision) {
          String profile = request.optString("profile", "standard");
          org.json.JSONObject source = request.optJSONObject("source");
          org.json.JSONObject resolution = source == null ? null : source.optJSONObject("resolution");
          int percent = resolution == null ? 100 : resolution.optInt("scalePercent", 100);
          appliedScale = percent == 75 ? "3/4" : percent == 60 ? "3/5" : percent == 50 ? "1/2" : "1/1";
          appliedQuality = "fast".equals(profile) ? "低畫質" : "high".equals(profile) ? "高畫質" : "標準";
          selectedScale = appliedScale;
          selectedQuality = appliedQuality;
        }
        org.json.JSONObject effective = result.optJSONObject("effective");
        if (effective != null && scaleButton != null)
          scaleButton.setContentDescription("來源解析度 " + effective.optInt("width") + " × " + effective.optInt("height"));
      }
      scaleButton.setText("\uf390  " + selectedScale);
      refreshRemoteAudio();
    } catch (Exception ignored) { }
  }

  /** 依 PC 版 compositeDecodedFrame 的規則，把 JPEG 區塊貼回完整 Bitmap。 */
  private boolean applyFrameJSON(String json) {
    try {
      org.json.JSONObject o = new org.json.JSONObject(json);
      int codec = o.optInt("codec", -1);
      long sequence = o.optLong("sequence", 0);
      int display = o.optInt("display", -1);
      if (displayPending || (displayKnown && (hostDisplay < 0 || display != hostDisplay))) return false;
      int fullWidth = o.optInt("width", 0);
      int fullHeight = o.optInt("height", 0);
      int x = o.optInt("x", 0);
      int y = o.optInt("y", 0);
      boolean keyframe = o.optBoolean("keyframe", false);
      if (sequence <= 0 || fullWidth <= 0 || fullHeight <= 0 || x < 0 || y < 0) return false;
      if (!FramePolicy.dimensions(fullWidth, fullHeight)) return false;
      String encoded = o.getString("data");
      if (encoded.length() > ((long) FramePolicy.MAX_ENCODED_BYTES + 2) / 3 * 4) {
        jpegPolicy.invalidate();
        requestVideoKeyframe();
        return false;
      }
      if (codec == 1 || codec == 2) {
        // H.264／HEVC 影格由 MediaCodec 直接輸出到 TextureView；這類影格不參與
        // JPEG 差分基底，避免兩種 renderer 互相覆蓋。
        if (x != 0 || y != 0) return false;
        if (viewportDisplay != display) {
          if (!keyframe) { requestVideoKeyframe(); return false; }
          videoDecoder.reset();
          pendingVideoFrame = null;
        }
        if (videoSequence > 0 && sequence <= videoSequence) return false;
        if ((disabledVideoCodecs & (1 << codec)) != 0) return false;
        byte[] payload = Base64.decode(encoded, Base64.DEFAULT);
        EncodedVideoFrame frame = EncodedVideoFrame.parse(codec, fullWidth, fullHeight, payload, keyframe);
        if (frame == null) {
          videoDecoder.reset();
          requestVideoKeyframe();
          return false;
        }
        if (composedFrame != null) {
          if (!composedFrame.isRecycled()) composedFrame.recycle();
          composedFrame = null;
        }
        composedSequence = sequence;
        jpegPolicy.invalidate();
        if (videoSequence > 0 && sequence != videoSequence + 1) {
          videoDecoder.reset();
          requestVideoKeyframe();
        }
        videoSequence = sequence;
        updateViewportDisplay(display);
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
      if (!jpegPolicy.accept(sequence, fullWidth, fullHeight, display, keyframe)) {
        if (jpegPolicy.needsKeyframe()) requestVideoKeyframe();
        return false;
      }
      byte[] data = Base64.decode(encoded, Base64.DEFAULT);
      Bitmap patch = decodeJpeg(data, fullWidth, fullHeight, x, y, keyframe);
      if (patch == null) {
        jpegPolicy.invalidate();
        requestVideoKeyframe();
        return false;
      }
      updateViewportDisplay(display);

      boolean replace = keyframe || composedFrame == null || composedWidth != fullWidth || composedHeight != fullHeight
          || composedDisplay != display;
      if (replace) {
        Bitmap old = composedFrame;
        // 完整 keyframe 本身就是可變基底，避免同時配置兩張全尺寸 bitmap。
        composedFrame = patch;
        composedWidth = fullWidth;
        composedHeight = fullHeight;
        composedDisplay = display;
        if (old != null && !old.isRecycled()) old.recycle();
      } else {
        Canvas canvas = new Canvas(composedFrame);
        canvas.drawBitmap(patch, x, y, null);
      }
      int patchWidth = patch.getWidth();
      int patchHeight = patch.getHeight();
      if (patch != composedFrame) patch.recycle();
      composedSequence = sequence;
      jpegPolicy.commit(sequence, fullWidth, fullHeight, display);
      videoKeyframeRequested = false;
      pumpRenderedFrames++;
      Rect dirty = new Rect(x, y, x + patchWidth, y + patchHeight);
      nativeScreen.setFrame(composedFrame, dirty, replace);
      markDisplayFramePresented();
      if (keyframe) {
        web.evaluateJavascript("window.desktopFrameStatus&&window.desktopFrameStatus(" + fullWidth + "," + fullHeight + ",true,'JPEG'," + display + ")", null);
      }
      return true;
    } catch (Exception ignored) {
      jpegPolicy.invalidate();
      if (videoMode) videoDecoder.reset();
      requestVideoKeyframe();
      return false;
    } catch (OutOfMemoryError error) {
      // 即使 dimensions 合法，低記憶體手機仍可能拒絕配置；丟棄基底並恢復。
      if (nativeScreen != null) nativeScreen.clearFrame();
      if (composedFrame != null && !composedFrame.isRecycled()) composedFrame.recycle();
      composedFrame = null;
      jpegPolicy.invalidate();
      requestVideoKeyframe();
      return false;
    }
  }

  /**
   * 差分區塊需要貼回可變 Bitmap，因此使用軟體 Bitmap 解碼。
   * ALLOCATOR_HARDWARE 是儲存位置選項，不代表 JPEG 硬解，且不能作為
   * 這個軟體 Canvas 的來源。影片硬解另走 MediaCodec／Surface 路徑。
   */
  private static Bitmap decodeJpeg(byte[] data, int width, int height, int x, int y, boolean keyframe) {
    if (data.length == 0 || data.length > FramePolicy.MAX_ENCODED_BYTES) return null;
    BitmapFactory.Options options = new BitmapFactory.Options();
    options.inJustDecodeBounds = true;
    BitmapFactory.decodeByteArray(data, 0, data.length, options);
    if (!"image/jpeg".equals(options.outMimeType)
        || !FramePolicy.patch(width, height, x, y, options.outWidth, options.outHeight, keyframe)) return null;
    options.inJustDecodeBounds = false;
    options.inMutable = true;
    options.inPreferredConfig = Bitmap.Config.ARGB_8888;
    return BitmapFactory.decodeByteArray(data, 0, data.length, options);
  }

  /** 原生 View 只 invalidate 變動區域；合成資料仍保留完整桌面基底。 */
  private static final class DeltaImageView extends View {
    private final Paint paint = new Paint();
    private final Rect source = new Rect();
    private final RectF destination = new RectF();
    private final DesktopViewport viewport;
    private Bitmap frame;

    DeltaImageView(Context context, DesktopViewport viewport) {
      super(context);
      this.viewport = viewport;
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

    /** 將觸控座標換算回等比例畫面；黑邊不接受控制事件。 */
    float[] mapTouch(float x, float y) {
      Bitmap bitmap = frame;
      if (bitmap == null || bitmap.isRecycled() || getWidth() <= 0 || getHeight() <= 0) {
        return null;
      }
      return viewport.geometry(bitmap.getWidth(), bitmap.getHeight(), getWidth(), getHeight()).mapTouch(x, y);
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
      DesktopViewport.Geometry area = viewport.geometry(bitmap.getWidth(), bitmap.getHeight(), getWidth(), getHeight());
      out.set(area.left, area.top, area.left + area.width, area.top + area.height);
    }
  }

  private void requestCameraPermissionOnMain() {
    if (isDestroyed() || cameraPermissionPending
        || checkSelfPermission(android.Manifest.permission.CAMERA) == android.content.pm.PackageManager.PERMISSION_GRANTED) return;
    cameraPermissionPending = true;
    requestPermissions(new String[]{android.Manifest.permission.CAMERA}, 7001);
  }

  @Override public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
    super.onRequestPermissionsResult(requestCode, permissions, grantResults);
    if (requestCode != 7001) return;
    cameraPermissionPending = false;
    if (!qrScanning || isDestroyed()) return;
    if (grantResults.length > 0 && grantResults[0] == android.content.pm.PackageManager.PERMISSION_GRANTED)
      startQrScannerOnMain();
    else { stopQrScannerOnMain(); cameraStatus("未取得相機權限，可改用貼上 QR 連結或手動輸入。"); }
  }

  private void startQrScannerOnMain() {
    if (isDestroyed() || isFinishing()) return;
    qrScanning = true;
    refreshAppUpdater();
    if (checkSelfPermission(android.Manifest.permission.CAMERA) != android.content.pm.PackageManager.PERMISSION_GRANTED) {
      requestCameraPermissionOnMain();
      return;
    }
    if (qrScanner != null) return;
    final int epoch = ++qrGeneration;
    try {
      qrScanner = BarcodeScanning.getClient();
      final com.google.common.util.concurrent.ListenableFuture<ProcessCameraProvider> future = ProcessCameraProvider.getInstance(this);
      future.addListener(() -> {
        if (!qrScanning || epoch != qrGeneration || isDestroyed() || isFinishing()) return;
        try {
          qrCameraProvider = future.get();
          qrAnalysis = new ImageAnalysis.Builder().setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST).build();
          qrAnalysis.setAnalyzer(ContextCompat.getMainExecutor(this), image -> analyzeQr(image, epoch));
          qrCameraProvider.bindToLifecycle(this, CameraSelector.DEFAULT_BACK_CAMERA, qrAnalysis);
        } catch (Exception error) {
          Log.w("YourDeskCamera", "相機初始化失敗", error);
          stopQrScannerOnMain();
          cameraStatus("相機無法啟動，可改用貼上 QR 連結或手動輸入。");
        }
      }, ContextCompat.getMainExecutor(this));
    } catch (Exception error) {
      stopQrScannerOnMain();
      cameraStatus("相機無法啟動，可改用貼上 QR 連結或手動輸入。");
    }
  }

  private void cameraStatus(String message) {
    if (web != null && !isDestroyed()) web.evaluateJavascript("window.cameraStatus&&window.cameraStatus("
        + org.json.JSONObject.quote(message) + ")", null);
  }

  private void stopQrScannerOnMain() {
    qrScanning = false;
    refreshAppUpdater();
    qrGeneration++;
    if (qrAnalysis != null) {
      qrAnalysis.clearAnalyzer();
      if (qrCameraProvider != null) qrCameraProvider.unbind(qrAnalysis);
      qrAnalysis = null;
    }
    qrCameraProvider = null;
    if (qrScanner != null) { qrScanner.close(); qrScanner = null; }
  }

  @androidx.annotation.OptIn(markerClass = androidx.camera.core.ExperimentalGetImage.class)
  private void analyzeQr(ImageProxy proxy, int epoch) {
    final BarcodeScanner scanner = qrScanner;
    if (!qrScanning || epoch != qrGeneration || scanner == null || proxy.getImage() == null || isDestroyed()) {
      proxy.close(); return;
    }
    long nowMs = android.os.SystemClock.uptimeMillis();
    if (nowMs - lastCameraJpegMs > 350) {
      lastCameraJpegMs = nowMs;
      Bitmap source = null, thumbnail = null;
      try {
        source = proxy.toBitmap();
        float scale = Math.min(1f, 640f / Math.max(source.getWidth(), source.getHeight()));
        Matrix transform = new Matrix();
        transform.postScale(scale, scale);
        transform.postRotate(proxy.getImageInfo().getRotationDegrees());
        thumbnail = Bitmap.createBitmap(source, 0, 0, source.getWidth(), source.getHeight(), transform, true);
        java.io.ByteArrayOutputStream output = new java.io.ByteArrayOutputStream();
        thumbnail.compress(Bitmap.CompressFormat.JPEG, 45, output);
        String data = "data:image/jpeg;base64," + Base64.encodeToString(output.toByteArray(), Base64.NO_WRAP);
        web.evaluateJavascript("window.cameraFrame&&window.cameraFrame(" + org.json.JSONObject.quote(data) + ")", null);
      } catch (RuntimeException error) {
        Log.w("YourDeskCamera", "相機縮圖轉換失敗", error);
      } finally {
        if (thumbnail != null && thumbnail != source) thumbnail.recycle();
        if (source != null) source.recycle();
      }
    }
    try {
      InputImage image = InputImage.fromMediaImage(proxy.getImage(), proxy.getImageInfo().getRotationDegrees());
      scanner.process(image).addOnSuccessListener(ContextCompat.getMainExecutor(this), codes -> {
        if (!qrScanning || epoch != qrGeneration || isDestroyed()) return;
        for (com.google.mlkit.vision.barcode.common.Barcode code : codes) {
          String value = code.getRawValue();
          if (value != null && value.toLowerCase(java.util.Locale.ROOT).contains("yourdesk:")) {
            web.evaluateJavascript("window.qrCodeDetected&&window.qrCodeDetected(" + org.json.JSONObject.quote(value) + ")", null);
            stopQrScannerOnMain();
            break;
          }
        }
      }).addOnCompleteListener(t -> proxy.close());
    } catch (RuntimeException error) { proxy.close(); }
  }

  final class Bridge {
    @JavascriptInterface public void setUpdateState(boolean idle, String language) {
      runOnUiThread(() -> {
        if (isDestroyed()) return;
        updatePageIdle = idle;
        updateLanguage = language == null ? "zh-Hant" : language;
        refreshAppUpdater();
      });
    }
    @JavascriptInterface public String loadSites() { return siteStore.loadSites(); }
    @JavascriptInterface public boolean saveSites(String json) { return siteStore.saveSites(json); }
    @JavascriptInterface public String saveSite(String json, String original, String secret, boolean forget) {
      return siteStore.saveSite(json, original, secret, forget);
    }
    @JavascriptInterface public boolean deleteSite(String key) { return siteStore.deleteSite(key); }
    @JavascriptInterface public String rememberedSecret(String room, String signal) {
      return siteStore.rememberedSecret(room, signal);
    }
    @JavascriptInterface public boolean rememberCredentialsFor(String room, String signal, String secret) {
      return siteStore.remember(room, signal, secret);
    }
    @JavascriptInterface public boolean rememberCredentials(String room, String secret) {
      return siteStore.remember(room, "", secret);
    }
    @JavascriptInterface public String appVersion() {
      try { return getPackageManager().getPackageInfo(getPackageName(), 0).versionName; }
      catch (Exception error) { return ""; }
    }

    @JavascriptInterface public void startQrScanner() {
      runOnUiThread(MainActivity.this::startQrScannerOnMain);
    }
    @JavascriptInterface public void stopQrScanner() { runOnUiThread(MainActivity.this::stopQrScannerOnMain); }
    @JavascriptInterface public void setSiteDialogVisible(boolean visible) {
      runOnUiThread(() -> {
        if (nativeScreen == null || nativeVideo == null || isDestroyed()) return;
        if (visible) { nativeScreen.setVisibility(View.GONE); nativeVideo.setVisibility(View.GONE); }
      });
    }
    @JavascriptInterface public void requestCameraPermission() {
      runOnUiThread(MainActivity.this::requestCameraPermissionOnMain);
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
      if (scalePercent != 100 && scalePercent != 75 && scalePercent != 60 && scalePercent != 50) return false;
      final int epoch = generation;
      try {
        viewer.sendStreamPreferences(profile == null ? "standard" : profile, scalePercent);
        runOnUiThread(() -> {
          if (epoch != generation || isDestroyed()) return;
          selectedScale = scalePercent == 75 ? "3/4" : scalePercent == 60 ? "3/5" : scalePercent == 50 ? "1/2" : "1/1";
          selectedQuality = "fast".equals(profile) ? "低畫質" : "high".equals(profile) ? "高畫質" : "標準";
          streamRequestStartedMs = android.os.SystemClock.uptimeMillis();
          scaleButton.setText("\uf390  " + selectedScale + " …");
        });
        return true;
      } catch (Exception e) { return false; }
    }

    @JavascriptInterface public String sendControlJSON(String payload) {
      try {
        org.json.JSONObject control = new org.json.JSONObject(payload);
        String type = control.optString("type");
        // WebView 僅處理鍵盤；指標事件必須由原生影像邊界檢查後送出。
        if (type.equals("move") || type.equals("button") || type.equals("wheel")) return "native pointer only";
        if (type.equals("display-select") || type.equals("display-next")) return "native display selection only";
        if (displayInputBlocked) return "螢幕切換中，請等待新畫面";
        if (inputDisplay >= 0) control.put("display", inputDisplay);
        viewer.sendControlJSON(control.toString());
        return "ok";
      }
      catch (Exception e) { return e.getMessage() == null ? "send failed" : e.getMessage(); }
    }

    @JavascriptInterface public String displayStateJSON() {
      return viewer.displayStateJSON();
    }

    @JavascriptInterface public void selectDisplay(int display) {
      runOnUiThread(() -> selectRemoteDisplay(display));
    }

    @JavascriptInterface public boolean supportsTextInput() {
      return viewer.supportsCommand("input.text-capabilities");
    }

    @JavascriptInterface public String readFrameJSON() {
      try { return viewer.readFrameJSON(); } catch (Exception e) { return ""; }
    }

    @JavascriptInterface public void toggleKeyboard() {
      runOnUiThread(MainActivity.this::toggleDesktopKeyboard);
    }

    @JavascriptInterface public void showKeyboard() {
      runOnUiThread(MainActivity.this::showDesktopKeyboard);
    }

    @JavascriptInterface public void hideKeyboard() {
      runOnUiThread(MainActivity.this::hideDesktopKeyboard);
    }

    @JavascriptInterface public void abortConnection() {
      runOnUiThread(() -> {
        updateConnecting = false;
        refreshAppUpdater();
        setDesktopFullscreen(false);
        hideDesktopKeyboard();
        generation++;
        inTerminal = false;
        inDesktop = false;
        updateKeyboardLayout();
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
        if (isDestroyed() || io.isShutdown()) return;
        updateConnecting = true;
        refreshAppUpdater();
        stopQrScannerOnMain();
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
            session.connect(SignalingAddress.validate(in.optString("signal", "")), in.getString("room"), in.getString("secret"));
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
              updateConnecting = false;
              refreshAppUpdater();
              updateKeyboardLayout();
              getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
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
            runOnUiThread(() -> { if (epoch == generation && !isDestroyed()) {
              updateConnecting = false;
              refreshAppUpdater();
              web.evaluateJavascript("window.connectionFailed&&window.connectionFailed()", null);
            } });
          }
        });
      });
    }

    @JavascriptInterface public void lockLandscape() {
      runOnUiThread(() -> setRequestedOrientation(ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE));
    }

    @JavascriptInterface public void closeTerminal() { runOnUiThread(MainActivity.this::leaveShell); }
    @JavascriptInterface public void sessionFailed() {
      runOnUiThread(() -> { if (inTerminal || inDesktop) endSession("遠端工作階段已結束，請重新連線。"); });
    }

    @JavascriptInterface public void request(String payload) {
      final int epoch = generation;
      final TerminalSession requested = terminal;
      if (io.isShutdown()) return;
      io.execute(() -> {
        if (epoch != generation) return;
        org.json.JSONObject reply = new org.json.JSONObject();
        try {
          org.json.JSONObject in = new org.json.JSONObject(payload);
          reply.put("id", in.getInt("id"));
          String method = in.getString("method");
          if (requested == null) throw new Exception("尚未建立 Shell 連線");
          switch (method) {
            case "read": reply.put("result", new org.json.JSONObject(requested.readJSON(in.getLong("ack")))); break;
            case "write": requested.write(Base64.decode(in.getString("data"), Base64.DEFAULT)); break;
            case "resize": requested.resize(in.getInt("columns"), in.getInt("rows")); break;
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

  private static boolean isLocalAsset(android.net.Uri uri) {
    return "https".equals(uri.getScheme()) && "appassets.androidplatform.net".equals(uri.getHost())
        && uri.getPort() == -1 && uri.getUserInfo() == null && uri.getPath() != null
        && uri.getPath().startsWith("/assets/");
  }

  private void refreshAppUpdater() {
    if (appUpdater != null) appUpdater.setState(foreground && updatePageIdle && !updateConnecting
        && !inDesktop && !inTerminal && !qrScanning, updateLanguage);
  }

  @Override protected void onResume() {
    super.onResume();
    foreground = true;
    refreshAppUpdater();
    if (web != null) web.onResume();
    if (inDesktop) {
      jpegPolicy.invalidate();
      videoKeyframeRequested = false;
      lastKeyframeRequestMs = 0;
      requestVideoKeyframe();
    }
    refreshRemoteAudio();
  }

  @Override protected void onPause() {
    foreground = false;
    refreshAppUpdater();
    refreshRemoteAudio();
    cancelDesktopGesture();
    videoDecoder.reset();
    if (web != null) web.onPause();
    super.onPause();
  }

  @Override protected void onStop() {
    if (qrScanning && !cameraPermissionPending) {
      stopQrScannerOnMain();
      cameraStatus("掃描已暫停，請按重新掃描。");
    }
    super.onStop();
  }

  @Override protected void onDestroy() {
    if (appUpdater != null) appUpdater.close();
    stopQrScannerOnMain();
    uiHandler.removeCallbacksAndMessages(null);
    if (fullscreenExitGesture != null) fullscreenExitGesture.reset();
    generation++;
    clearComposedFrame();
    remoteAudio.close();
    unregisterReceiver(audioNoisyReceiver);
    new Thread(viewer::close).start();
    io.shutdown();
    if (web != null) { web.removeJavascriptInterface("YourDesk"); web.destroy(); }
    super.onDestroy();
  }

  private void handleBackNavigation() {
    if (desktopFullscreen) setDesktopFullscreen(false);
    else if (inTerminal || inDesktop) leaveShell();
    else if (web != null) web.evaluateJavascript("window.dismissOverlay ? window.dismissOverlay() : false", result -> {
      if (!"true".equals(result) && !isDestroyed()) dispatchDefaultBack();
    });
    else dispatchDefaultBack();
  }

  private void dispatchDefaultBack() {
    backCallback.setEnabled(false);
    try { getOnBackPressedDispatcher().onBackPressed(); }
    finally { backCallback.setEnabled(true); }
  }
}
