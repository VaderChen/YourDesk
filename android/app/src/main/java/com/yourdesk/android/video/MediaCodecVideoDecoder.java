package com.yourdesk.android.video;

import android.media.MediaCodec;
import android.media.MediaFormat;
import android.view.Surface;

import java.nio.ByteBuffer;
import java.util.Arrays;
import java.util.HashSet;
import java.util.Set;

/** 有界候選嘗試的 Surface decoder；所有方法由同一同步鎖保護。 */
public final class MediaCodecVideoDecoder implements AutoCloseable {
  private MediaCodec codec;
  private Surface surface;
  private String codecName;
  private int wireCodec;
  private int width;
  private int height;
  private byte[] csd0;
  private byte[] csd1;
  private long presentationUs;
  private boolean started;
  private boolean awaitingKeyframe = true;
  private final Set<String> failedCandidates = new HashSet<>();
  private final boolean[] unavailableCodecs = new boolean[3];
  private int failedWireCodec;
  private int consecutiveFailures;
  private int inputMisses;

  /** >=0 為已顯示張數，-1 為失敗；0 時另查 needsKeyframe()，避免遺失相依幀後繼續。 */
  public synchronized int queue(EncodedVideoFrame frame, Surface target) {
    if (frame == null || target == null || !target.isValid()) return -1;
    if (unavailableCodecs[frame.codec]) { failedWireCodec = frame.codec; return -1; }
    boolean changed = !started || surface != target || frame.codec != wireCodec
        || width != frame.width || height != frame.height
        || !Arrays.equals(csd0, frame.csd0) || !Arrays.equals(csd1, frame.csd1);
    if (changed) { releaseCodec(); awaitingKeyframe = true; }
    if (awaitingKeyframe && !frame.keyframe) return 0;
    try {
      if (!started && !configure(target, frame)) return -1;
      int inputIndex = codec.dequeueInputBuffer(0);
      if (inputIndex < 0) {
        // queue 的呼叫者不會重送此 access unit；捨棄後必須從新 IDR 恢復。
        awaitingKeyframe = true;
        if (++inputMisses >= 3) return failCurrentDecoder(frame.codec);
        return drain();
      }
      ByteBuffer input = codec.getInputBuffer(inputIndex);
      if (input == null || input.capacity() < frame.accessUnit.length) {
        // 已取得 input index 就擁有該槽；不能直接返回把它永久扣住。
        return failCurrentDecoder(frame.codec);
      }
      input.clear();
      input.put(frame.accessUnit);
      int flags = frame.keyframe ? MediaCodec.BUFFER_FLAG_KEY_FRAME : 0;
      codec.queueInputBuffer(inputIndex, 0, frame.accessUnit.length, presentationUs++, flags);
      awaitingKeyframe = false;
      inputMisses = 0;
      int rendered = drain();
      if (rendered > 0) consecutiveFailures = 0;
      return rendered;
    } catch (RuntimeException e) {
      return failCurrentDecoder(frame.codec);
    }
  }

  public synchronized int drainOutput() {
    if (!started || codec == null) return 0;
    try {
      int rendered = drain();
      if (rendered > 0) consecutiveFailures = 0;
      return rendered;
    } catch (RuntimeException e) {
      return failCurrentDecoder(wireCodec);
    }
  }

  public synchronized int consecutiveFailures() { return consecutiveFailures; }
  public synchronized boolean needsKeyframe() { return awaitingKeyframe; }
  /** 1／2 表示該 codec 的候選已耗盡，Activity 應撤回能力並重新協商；0 表示未耗盡。 */
  public synchronized int failedWireCodec() { return failedWireCodec; }

  private boolean configure(Surface target, EncodedVideoFrame frame) {
    String mime = frame.codec == 1 ? MediaFormat.MIMETYPE_VIDEO_AVC : MediaFormat.MIMETYPE_VIDEO_HEVC;
    MediaFormat format = MediaFormat.createVideoFormat(mime, frame.width, frame.height);
    format.setByteBuffer("csd-0", ByteBuffer.wrap(frame.csd0));
    if (frame.csd1 != null) format.setByteBuffer("csd-1", ByteBuffer.wrap(frame.csd1));
    format.setInteger(MediaFormat.KEY_MAX_INPUT_SIZE, Math.max(1024 * 1024, frame.accessUnit.length));
    int profile = DecoderSupport.androidProfile(frame);
    if (profile >= 0) format.setInteger(MediaFormat.KEY_PROFILE, profile);
    for (String candidate : DecoderSupport.hardwareDecoders(mime, format)) {
      if (failedCandidates.contains(candidateKey(frame.codec, candidate))) continue;
      MediaCodec next = null;
      try {
        next = MediaCodec.createByCodecName(candidate);
        next.configure(format, target, null, 0);
        next.start();
        codec = next;
        surface = target;
        codecName = candidate;
        wireCodec = frame.codec;
        width = frame.width;
        height = frame.height;
        csd0 = frame.csd0;
        csd1 = frame.csd1;
        presentationUs = 0;
        started = true;
        awaitingKeyframe = true;
        failedWireCodec = 0;
        return true;
      } catch (Exception ignored) {
        failedCandidates.add(candidateKey(frame.codec, candidate));
        release(next);
      }
    }
    unavailableCodecs[frame.codec] = true;
    failedWireCodec = frame.codec;
    consecutiveFailures++;
    awaitingKeyframe = true;
    return false;
  }

  private int failCurrentDecoder(int failedCodec) {
    if (codecName != null) failedCandidates.add(candidateKey(failedCodec, codecName));
    releaseCodec();
    awaitingKeyframe = true;
    consecutiveFailures++;
    // 下一張 keyframe 可試下一個候選；所有候選失敗才撤回 codec。
    String mime = failedCodec == 1 ? MediaFormat.MIMETYPE_VIDEO_AVC : MediaFormat.MIMETYPE_VIDEO_HEVC;
    boolean remaining = false;
    for (String candidate : DecoderSupport.hardwareDecoders(mime)) {
      if (!failedCandidates.contains(candidateKey(failedCodec, candidate))) { remaining = true; break; }
    }
    if (!remaining && failedCodec > 0) {
      unavailableCodecs[failedCodec] = true;
      failedWireCodec = failedCodec;
    }
    return -1;
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
  /** Surface／缺幀恢復不清除候選黑名單，防止反覆碰到同一個壞 decoder。 */
  public synchronized void reset() { releaseCodec(); awaitingKeyframe = true; }
  public synchronized void resetSession() {
    reset();
    failedCandidates.clear();
    Arrays.fill(unavailableCodecs, false);
    failedWireCodec = consecutiveFailures = 0;
  }
  @Override public synchronized void close() { resetSession(); }

  private void releaseCodec() {
    release(codec);
    codec = null;
    surface = null;
    codecName = null;
    csd0 = csd1 = null;
    wireCodec = width = height = inputMisses = 0;
    presentationUs = 0;
    started = false;
  }

  private static void release(MediaCodec codec) {
    if (codec == null) return;
    try { codec.stop(); } catch (Exception ignored) { }
    try { codec.release(); } catch (Exception ignored) { }
  }

  private static String candidateKey(int codec, String name) { return codec + ":" + name; }
}
