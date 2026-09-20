package android.media;

import android.view.Surface;
import java.nio.ByteBuffer;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.Map;

/** Stateful fake records input ownership and rejects operations after release. */
public final class MediaCodec {
  public static final int BUFFER_FLAG_KEY_FRAME = 1, INFO_TRY_AGAIN_LATER = -1,
      INFO_OUTPUT_FORMAT_CHANGED = -2;
  public static final class BufferInfo { }
  public static final class Plan {
    public int capacity = 1024 * 1024;
    public boolean configureThrows, inputNull, inputUnavailable, queueThrows, drainThrows;
  }
  public static final Map<String, Plan> plans = new HashMap<>();
  public static final ArrayList<MediaCodec> instances = new ArrayList<>();
  public final String name;
  public final Plan plan;
  public boolean released, started, ownsInput;
  public int queued, rendered;
  public MediaFormat configured;
  private int availableOutputs;
  private MediaCodec(String name) { this.name = name; plan = plans.computeIfAbsent(name, k -> new Plan()); }
  public static MediaCodec createByCodecName(String name) {
    MediaCodec codec = new MediaCodec(name); instances.add(codec); return codec;
  }
  public void configure(MediaFormat format, Surface target, Object crypto, int flags) {
    if (plan.configureThrows) throw new IllegalStateException("configure failure");
    configured = format;
  }
  public void start() { started = true; }
  public int dequeueInputBuffer(long timeout) {
    requireStarted();
    if (ownsInput) throw new IllegalStateException("input leaked");
    if (plan.inputUnavailable) return INFO_TRY_AGAIN_LATER;
    ownsInput = true; return 0;
  }
  public ByteBuffer getInputBuffer(int index) {
    requireStarted();
    return plan.inputNull ? null : ByteBuffer.allocate(plan.capacity);
  }
  public void queueInputBuffer(int index, int offset, int size, long pts, int flags) {
    requireStarted();
    if (plan.queueThrows) throw new IllegalStateException("queue failure");
    if (!ownsInput) throw new IllegalStateException("input not owned");
    ownsInput = false; queued++; availableOutputs++;
  }
  public int dequeueOutputBuffer(BufferInfo info, long timeout) {
    requireStarted();
    if (plan.drainThrows) throw new IllegalStateException("drain failure");
    if (availableOutputs == 0) return INFO_TRY_AGAIN_LATER;
    availableOutputs--; return 0;
  }
  public void releaseOutputBuffer(int index, boolean render) { requireStarted(); if (render) rendered++; }
  public void stop() { started = false; ownsInput = false; }
  public void release() { released = true; started = false; ownsInput = false; }
  private void requireStarted() {
    if (!started || released) throw new IllegalStateException("invalid codec lifecycle");
  }
}
