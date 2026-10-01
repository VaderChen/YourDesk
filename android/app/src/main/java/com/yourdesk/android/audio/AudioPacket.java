package com.yourdesk.android.audio;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.Arrays;

/** 與 Host 的 YDA1 封包共用格式；在交給系統解碼器前完成長度與世代檢查。 */
public final class AudioPacket {
  public final long generation, sequence;
  public final int codec;
  public final byte[] data;
  private AudioPacket(long generation, long sequence, int codec, byte[] data) {
    this.generation = generation; this.sequence = sequence; this.codec = codec; this.data = data;
  }
  public static AudioPacket parse(byte[] wire) {
    if (wire == null || wire.length <= 24 || wire.length > 8216 || wire[0] != 'Y' || wire[1] != 'D'
        || wire[2] != 'A' || wire[3] != '1' || wire[5] != 0 || wire[6] != 0 || wire[7] != 0) return null;
    int codec = wire[4] & 255;
    if (codec < 1 || codec > 3 || (codec == 2 && wire.length != 4120) || (codec == 3 && wire.length > 1299)) return null;
    ByteBuffer header = ByteBuffer.wrap(wire).order(ByteOrder.BIG_ENDIAN);
    long generation = header.getLong(8), sequence = header.getLong(16);
    if (generation <= 0 || sequence <= 0) return null;
    return new AudioPacket(generation, sequence, codec, Arrays.copyOfRange(wire, 24, wire.length));
  }
}
