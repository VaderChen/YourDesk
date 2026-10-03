package com.yourdesk.android.update;

import android.app.AlertDialog;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.net.Uri;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;
import android.util.Log;
import android.widget.Toast;
import androidx.activity.ComponentActivity;
import androidx.activity.result.ActivityResultLauncher;
import androidx.activity.result.contract.ActivityResultContracts;
import androidx.core.content.FileProvider;
import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.StandardCopyOption;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import org.json.JSONObject;

/** 首頁前景自動檢查／下載；離開首頁取消網路工作，安裝必須由使用者確認。 */
public final class AppUpdater implements AutoCloseable {
  private static final long CHECK_INTERVAL = 6 * 60 * 60 * 1000L;
  private static final long RETRY_INTERVAL = 15 * 60 * 1000L;
  private final ComponentActivity activity;
  private final android.content.Context context;
  private final SharedPreferences preferences;
  private final File directory, apk, record;
  private final Handler main = new Handler(Looper.getMainLooper());
  private final ExecutorService worker = Executors.newSingleThreadExecutor(r -> new Thread(r, "YourDesk-AppUpdate"));
  private final ActivityResultLauncher<Intent> permissionLauncher, installLauncher;
  private final Runnable tick = this::check;
  private volatile boolean active, closed;
  private volatile UpdateHttp http;
  private Future<?> task;
  private ReleaseUpdate ready;
  private AlertDialog dialog;
  private boolean loaded, enabled, waitingPermission, pendingInstall, installing;
  private volatile int generation;
  private long nextCheck;
  private String language = "zh-Hant";

  public AppUpdater(ComponentActivity activity) {
    this.activity = activity; context = activity.getApplicationContext();
    preferences = context.getSharedPreferences("app-updates", 0);
    directory = new File(context.getFilesDir(), "updates");
    apk = new File(directory, "update.apk"); record = new File(directory, "update.json");
    enabled = "com.yourdesk.android".equals(context.getPackageName())
        && (context.getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) == 0;
    nextCheck = preferences.getLong("nextCheck", 0);
    if (nextCheck > System.currentTimeMillis() + CHECK_INTERVAL) nextCheck = 0;
    permissionLauncher = activity.registerForActivityResult(new ActivityResultContracts.StartActivityForResult(), result -> {
      waitingPermission = false;
      pendingInstall = context.getPackageManager().canRequestPackageInstalls();
      if (!pendingInstall) toast(text(4));
      main.post(tick);
    });
    installLauncher = activity.registerForActivityResult(new ActivityResultContracts.StartActivityForResult(), result -> {
      installing = false;
      // 安裝成功會替換程序；取消則保留已驗證 APK，稍後可重新提示。
      main.post(tick);
    });
  }

  /** 僅由 Activity 的主執行緒呼叫；頁面與原生工作階段需同時處於閒置。 */
  public void setState(boolean idle, String selectedLanguage) {
    language = selectedLanguage == null ? "zh-Hant" : selectedLanguage;
    if (closed || !enabled) return;
    if (active == idle) { if (idle) check(); return; }
    active = idle;
    main.removeCallbacks(tick);
    if (!idle) {
      generation++;
      UpdateHttp connection = http;
      if (connection != null) connection.cancel();
      if (task != null) { task.cancel(true); task = null; loaded = false; }
      if (dialog != null) { dialog.dismiss(); dialog = null; }
    } else check();
  }

  private void check() {
    main.removeCallbacks(tick);
    if (closed || !enabled || !active || activity.isFinishing() || activity.isDestroyed()
        || waitingPermission || installing || task != null) return;
    if (pendingInstall && ready != null) { pendingInstall = false; install(); return; }
    long now = System.currentTimeMillis();
    if (ready != null) {
      long promptAfter = preferences.getLong("promptAfter", 0);
      if (promptAfter > now + CHECK_INTERVAL) promptAfter = 0;
      if (now >= promptAfter && dialog == null) prompt();
      else if (dialog == null) main.postDelayed(tick, Math.max(1000, promptAfter - now));
      return;
    }
    if (loaded && now < nextCheck) { main.postDelayed(tick, nextCheck - now); return; }
    final int epoch = ++generation;
    final boolean inspectCache = !loaded;
    final boolean networkDue = now >= nextCheck;
    task = worker.submit(() -> {
      ReleaseUpdate result = null;
      boolean checked = false, failed = false;
      try {
        PackageInfo installed = ApkVerifier.installed(context);
        if (!directory.isDirectory() && !directory.mkdirs()) throw new IOException("Cannot create update cache");
        removeParts();
        if (inspectCache) result = cached(installed, epoch);
        if (result == null && networkDue) {
          UpdateHttp connection = new UpdateHttp(); http = connection;
          try {
            UpdateFiles.check(() -> closed || !active || epoch != generation);
            result = ReleaseUpdate.find(connection, ApkVerifier.code(installed), installed.versionName);
            if (result != null) download(connection, result, epoch);
            checked = true;
          } finally { connection.close(); if (http == connection) http = null; }
        }
      } catch (Exception error) {
        failed = active && !closed && epoch == generation;
        if (failed) Log.w("YourDeskUpdate", "更新檢查／下載未完成，稍後重試", error);
      } finally { removeParts(); }
      final ReleaseUpdate downloaded = failed ? null : result;
      final boolean didCheck = checked, retry = failed;
      main.post(() -> {
        if (closed || epoch != generation) return;
        task = null; loaded = true;
        if (didCheck || retry) {
          nextCheck = System.currentTimeMillis() + (retry ? RETRY_INTERVAL : CHECK_INTERVAL);
          preferences.edit().putLong("nextCheck", nextCheck).apply();
        }
        ready = downloaded;
        check();
      });
    });
  }

  private ReleaseUpdate cached(PackageInfo installed, int epoch) {
    try {
      if (!record.isFile() || !apk.isFile() || record.length() > 64 * 1024 || apk.length() > ReleaseUpdate.MAX_APK_BYTES)
        throw new IOException("Incomplete update cache");
      JSONObject json;
      try (FileInputStream input = new FileInputStream(record)) {
        json = new JSONObject(new String(UpdateFiles.read(input, 64 * 1024, () -> closed || !active || epoch != generation), StandardCharsets.UTF_8));
      }
      ReleaseUpdate value = ReleaseUpdate.fromJson(json, json.getString("archiveUrl"));
      if (value.versionCode <= ApkVerifier.code(installed) || !UpdateFiles.sha256(apk, () -> closed || !active || epoch != generation).equals(value.apkHash)) throw new IOException("Expired update cache");
      ApkVerifier.verify(context, apk, value);
      return value;
    } catch (Exception ignored) {
      if (active && !closed && epoch == generation && !Thread.currentThread().isInterrupted()) { record.delete(); apk.delete(); }
      return null;
    }
  }

  private void download(UpdateHttp connection, ReleaseUpdate update, int epoch) throws Exception {
    File zipPart = new File(directory, "download.part"), apkPart = new File(directory, "apk.part"), jsonPart = new File(directory, "record.part");
    connection.download(update, zipPart);
    UpdateFiles.extract(zipPart, apkPart, update, () -> closed || !active || epoch != generation);
    ApkVerifier.verify(context, apkPart, update);
    UpdateFiles.check(() -> closed || !active || epoch != generation);
    Files.write(jsonPart.toPath(), update.toJson().toString().getBytes(StandardCharsets.UTF_8));
    Files.move(apkPart.toPath(), apk.toPath(), StandardCopyOption.REPLACE_EXISTING, StandardCopyOption.ATOMIC_MOVE);
    Files.move(jsonPart.toPath(), record.toPath(), StandardCopyOption.REPLACE_EXISTING, StandardCopyOption.ATOMIC_MOVE);
  }

  private void removeParts() {
    for (String name : new String[]{"download.part", "apk.part", "record.part"}) new File(directory, name).delete();
  }

  private void prompt() {
    dialog = new AlertDialog.Builder(activity).setTitle(text(0))
        .setMessage("YourDesk " + ready.version + "\n\n" + text(1))
        .setPositiveButton(text(2), (value, which) -> { postpone(); requestInstall(); })
        .setNegativeButton(text(3), (value, which) -> postpone())
        .setOnCancelListener(value -> postpone()).create();
    dialog.setOnDismissListener(value -> dialog = null);
    dialog.show();
  }

  private void postpone() {
    preferences.edit().putLong("promptAfter", System.currentTimeMillis() + CHECK_INTERVAL).apply();
    main.postDelayed(tick, CHECK_INTERVAL);
  }

  private void requestInstall() {
    if (!active || closed || ready == null) return;
    if (!context.getPackageManager().canRequestPackageInstalls()) {
      try {
        waitingPermission = true;
        permissionLauncher.launch(new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:" + context.getPackageName())));
      } catch (RuntimeException error) { waitingPermission = false; toast(text(4)); }
    } else install();
  }

  @SuppressWarnings("deprecation")
  private void install() {
    if (!active || closed || ready == null) return;
    try {
      Uri uri = FileProvider.getUriForFile(context, context.getPackageName() + ".updates", apk);
      Intent intent = new Intent(Intent.ACTION_INSTALL_PACKAGE).setDataAndType(uri, "application/vnd.android.package-archive")
          .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION).putExtra(Intent.EXTRA_RETURN_RESULT, true);
      installing = true; installLauncher.launch(intent);
    } catch (RuntimeException error) { installing = false; toast(text(5)); }
  }

  private void toast(String message) {
    if (!closed && !activity.isDestroyed()) Toast.makeText(activity, message, Toast.LENGTH_LONG).show();
  }

  private static final String[][] MESSAGES = {
    {"新版已下載", "更新套件已驗證。安裝會由 Android 再次確認，並保留原有站台與設定。", "安裝更新", "稍後", "請允許 YourDesk 安裝更新；已下載的套件會保留。", "無法開啟系統安裝程式；更新套件已保留，稍後重試。"},
    {"Update downloaded", "The update has been verified. Android will confirm installation. Your sites and settings will be kept.", "Install update", "Later", "Allow YourDesk to install updates. The downloaded package is kept.", "Cannot open the system installer. The update is kept for a later retry."},
    {"更新をダウンロードしました", "更新パッケージは検証済みです。Android でインストールを確認してください。接続先と設定は保持されます。", "更新をインストール", "後で", "YourDesk に更新のインストールを許可してください。パッケージは保持されます。", "システムのインストーラーを開けません。更新は保持され、後で再試行できます。"},
    {"업데이트 다운로드 완료", "업데이트 패키지를 검증했습니다. Android에서 설치를 확인하세요. 접속 대상과 설정은 유지됩니다.", "업데이트 설치", "나중에", "YourDesk의 업데이트 설치를 허용하세요. 다운로드한 패키지는 보관됩니다.", "시스템 설치 프로그램을 열 수 없습니다. 업데이트는 보관되며 나중에 다시 시도할 수 있습니다."}
  };

  private String text(int index) {
    return MESSAGES[language.startsWith("en") ? 1 : language.startsWith("ja") ? 2 : language.startsWith("ko") ? 3 : 0][index];
  }

  @Override public void close() {
    closed = true; active = false; generation++;
    main.removeCallbacksAndMessages(null);
    UpdateHttp connection = http; if (connection != null) connection.cancel();
    if (task != null) task.cancel(true);
    worker.shutdownNow();
    if (dialog != null) { dialog.dismiss(); dialog = null; }
    permissionLauncher.unregister(); installLauncher.unregister();
  }
}
