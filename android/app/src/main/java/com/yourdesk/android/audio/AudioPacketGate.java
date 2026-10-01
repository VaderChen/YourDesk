package com.yourdesk.android.audio;

/** 每個播放世代獨立排序；0 表示丟棄，1 表示連續，2 表示缺包後須清除播放歷史。 */
public final class AudioPacketGate {
  private final long generation;
  private final int codec;
  private long sequence;
  public AudioPacketGate(long generation, int codec) { this.generation = generation; this.codec = codec; }
  public int accept(AudioPacket packet) {
    if (packet == null || packet.generation != generation || packet.codec != codec || packet.sequence <= sequence) return 0;
    boolean gap = sequence != 0 && packet.sequence != sequence + 1;
    sequence = packet.sequence;
    return gap ? 2 : 1;
  }
}
