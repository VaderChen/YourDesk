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
      int inputIndex = codec.dequeueInputBuffer(0);
      if (inputIndex < 0) return 0;
      ByteBuffer input = codec.getInputBuffer(inputIndex);
      if (input == null || input.remaining() < frame.accessUnit.length) return -1;
      input.clear();
      input.put(frame.accessUnit);
      int flags = frame.keyframe ? MediaCodec.BUFFER_FLAG_KEY_FRAME : 0;
      codec.queueInputBuffer(inputIndex, 0, frame.accessUnit.length, presentationUs++, flags);
      int rendered = drain();
      awaitingKeyframe = false;
      return rendered;
    } catch (IllegalStateException e) {
      closeLocked();
      awaitingKeyframe = true;
      return -1;
    }
  }

  private boolean configure(Surface target, String wantedMime, EncodedVideoFrame frame) {
    closeLocked();
    String candidate = DecoderSupport.findHardwareDecoder(wantedMime);
    if (candidate == null) return false;
    try {
      MediaCodec next = MediaCodec.createByCodecName(candidate);
      MediaFormat format = MediaFormat.createVideoFormat(wantedMime, frame.width, frame.height);
      format.setByteBuffer("csd-0", ByteBuffer.wrap(frame.csd0));
      format.setByteBuffer("csd-1", ByteBuffer.wrap(frame.csd1));
      if (frame.csd2 != null) format.setByteBuffer("csd-2", ByteBuffer.wrap(frame.csd2));
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
      return true;
    } catch (Exception e) {
      closeLocked();
      awaitingKeyframe = true;
      return false;
    }
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
    awaitingKeyframe = true;
  }
}
