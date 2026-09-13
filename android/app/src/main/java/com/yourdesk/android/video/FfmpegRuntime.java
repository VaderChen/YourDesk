package com.yourdesk.android.video;

import android.util.Log;

import com.arthenica.ffmpegkit.FFmpegKitConfig;

import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * FFmpeg 預編譯 runtime 的能力探測。這裡只載入共享庫並讀取版本，
 * 不在每張影格上啟動 ffmpeg 命令列程序；即時解碼會由專用 decoder session
 * 共用工作階段，失敗時再回退到 Android 軟體解碼。
 */
public final class FfmpegRuntime {
  private static final String TAG = "YourDeskVideo";
  private static final ExecutorService PROBE = Executors.newSingleThreadExecutor();
  private static volatile String version = "尚未探測";

  private FfmpegRuntime() { }

  public static void probeAsync() {
    PROBE.execute(() -> {
      try {
        String ffmpeg = FFmpegKitConfig.getFFmpegVersion();
        String wrapper = FFmpegKitConfig.getVersion();
        version = (ffmpeg == null || ffmpeg.isEmpty()) ? "FFmpeg runtime 不可用" : ffmpeg;
        Log.i(TAG, "FFmpeg 預編譯 runtime=" + version + "，wrapper=" + wrapper);
      } catch (Throwable e) {
        version = "FFmpeg runtime 載入失敗";
        Log.w(TAG, version, e);
      }
    });
  }

  public static String status() { return version; }
}
