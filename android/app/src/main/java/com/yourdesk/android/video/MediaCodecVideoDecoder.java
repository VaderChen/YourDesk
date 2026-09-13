package com.yourdesk.android.video;

import android.media.MediaCodec;
import android.media.MediaFormat;
import android.view.Surface;

import java.nio.ByteBuffer;

/**
 * 長生命週期的 Android MediaCodec decoder。輸出直接送到 TextureView 的 Surface，
 * 不把 H.264／HEVC 每張影格轉成 Bitmap，避免 CPU 色彩轉換與 Java heap 複製。
 */
public final class MediaCodecVideoDecoder implements AutoCloseable {
  private MediaCodec codec;
  private Surface surface;
  private String codecName;
  private String mime;
  private int width;
  private int height;
  private long presentationUs;
  private boolean started;
  private boolean awaitingKeyframe;
  private String[] candidates = new String[0];
  private int consecutiveFailures;

  /** 將 access unit 送入硬體 decoder；輸出 frame 數量可作顯示統計。 */
  public synchronized int queue(EncodedVideoFrame frame, Surface target) {
    if (frame == null || target == null || !target.isValid()) return -1;
    if (awaitingKeyframe && !frame.keyframe) return 0;
    String wantedMime = frame.codec == 1 ? MediaFormat.MIMETYPE_VIDEO_AVC : MediaFormat.MIMETYPE_VIDEO_HEVC;
    try {
      if (!started || surface != target || !wantedMime.equals(mime)
          || width != frame.width || height != frame.height) {
        if (!configure(target, wantedMime, frame)) return -1;
      }
      int inputIndex = codec.dequeueInputBuffer(2000);
      if (inputIndex < 0) return 0;
      ByteBuffer input = codec.getInputBuffer(inputIndex);
      if (input == null || input.capacity() < frame.accessUnit.length) return -1;
      input.clear();
      input.put(frame.accessUnit);
      int flags = frame.keyframe ? MediaCodec.BUFFER_FLAG_KEY_FRAME : 0;
      codec.queueInputBuffer(inputIndex, 0, frame.accessUnit.length, presentationUs++, flags);
      int rendered = drain();
      awaitingKeyframe = false;
      consecutiveFailures = 0;
      return rendered;
    } catch (RuntimeException e) {
      closeLocked();
      awaitingKeyframe = true;
      consecutiveFailures++;
      return -1;
    }
  }

  /**
   * 排空尚未立即可用的 Surface 輸出。Surface 解碼通常比輸入晚一個或數個
   * 工作週期，因此由畫面 pump 週期性呼叫，避免只在收到下一張影格時才顯示。
   */
  public synchronized int drainOutput() {
    if (!started || codec == null) return 0;
    try {
      return drain();
    } catch (RuntimeException e) {
      closeLocked();
      awaitingKeyframe = true;
      consecutiveFailures++;
      return -1;
    }
  }

  public synchronized int consecutiveFailures() { return consecutiveFailures; }

  private boolean configure(Surface target, String wantedMime, EncodedVideoFrame frame) {
    closeLocked();
    candidates = DecoderSupport.hardwareDecoders(wantedMime);
    if (candidates.length == 0) return false;
    MediaFormat format = MediaFormat.createVideoFormat(wantedMime, frame.width, frame.height);
    format.setByteBuffer("csd-0", ByteBuffer.wrap(frame.csd0));
    format.setByteBuffer("csd-1", ByteBuffer.wrap(frame.csd1));
    if (frame.csd2 != null) format.setByteBuffer("csd-2", ByteBuffer.wrap(frame.csd2));
    for (String candidate : candidates) {
      MediaCodec next = null;
      try {
        next = MediaCodec.createByCodecName(candidate);
        next.configure(format, target, null, 0);
        next.start();
        codec = next;
        surface = target;
        codecName = candidate;
        mime = wantedMime;
        width = frame.width;
        height = frame.height;
        presentationUs = 0;
        started = true;
        awaitingKeyframe = true;
        return true;
      } catch (Exception ignored) {
        if (next != null) {
          try { next.stop(); } catch (Exception ignoredStop) { }
          try { next.release(); } catch (Exception ignoredRelease) { }
        }
      }
    }
    closeLocked();
    awaitingKeyframe = true;
    return false;
  }

  private int drain() {
    int rendered = 0;
    MediaCodec.BufferInfo info = new MediaCodec.BufferInfo();
    for (int i = 0; i < 8; i++) {
      int output = codec.dequeueOutputBuffer(info, 0);
      if (output == MediaCodec.INFO_TRY_AGAIN_LATER) break;
      if (output == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED) continue;
      if (output >= 0) {
        codec.releaseOutputBuffer(output, true);
        rendered++;
      }
    }
    return rendered;
  }

  public synchronized boolean isStarted() { return started; }
  public synchronized String backend() { return codecName == null ? "" : codecName; }
  public synchronized void reset() { closeLocked(); }
  @Override public synchronized void close() { closeLocked(); }

  private void closeLocked() {
    started = false;
    presentationUs = 0;
    if (codec != null) {
      try { codec.stop(); } catch (Exception ignored) { }
      try { codec.release(); } catch (Exception ignored) { }
    }
    codec = null;
    surface = null;
    codecName = null;
    mime = null;
    width = height = 0;
    candidates = new String[0];
    consecutiveFailures = 0;
    awaitingKeyframe = true;
  }
}
