package com.yourdesk.android.video;

import java.io.ByteArrayOutputStream;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;

/**
 * Android 端的 H.264／HEVC wire 封包。Host 以「參數集數量 + 4-byte NAL 長度」
 * 傳送，每筆影格都帶參數集，讓 decoder 在遺失影格後可以從下一個 keyframe 恢復。
 */
public final class EncodedVideoFrame {
  public final int codec;
  public final int width;
  public final int height;
  public final byte[] csd0;
  public final byte[] csd1;
  public final byte[] csd2;
  public final byte[] accessUnit;
  public final boolean keyframe;

  private EncodedVideoFrame(int codec, int width, int height, byte[] csd0, byte[] csd1,
      byte[] csd2, byte[] accessUnit, boolean keyframe) {
    this.codec = codec;
    this.width = width;
    this.height = height;
    this.csd0 = csd0;
    this.csd1 = csd1;
    this.csd2 = csd2;
    this.accessUnit = accessUnit;
    this.keyframe = keyframe;
  }

  /** 解析 core p2p Frame 的 payload；無法完整解析時回傳 null。 */
  public static EncodedVideoFrame parse(int codec, int width, int height, byte[] payload,
      boolean keyframeHint) {
    if ((codec != 1 && codec != 2) || width <= 0 || height <= 0 || payload == null
        || payload.length < 1 || payload.length > 32 * 1024 * 1024) return null;
    int parameterCount = codec == 1 ? 2 : 3;
    if ((payload[0] & 0xff) != parameterCount) return null;
    byte[][] parameters = new byte[parameterCount][];
    ByteArrayOutputStream access = new ByteArrayOutputStream(payload.length);
    boolean picture = false;
    int offset = 1;
    int index = 0;
    try {
      while (offset < payload.length) {
        if (payload.length - offset < 4) return null;
        int length = ByteBuffer.wrap(payload, offset, 4).order(ByteOrder.BIG_ENDIAN).getInt();
        offset += 4;
        if (length < 1 || length > payload.length - offset) return null;
        byte[] nal = new byte[length];
        System.arraycopy(payload, offset, nal, 0, length);
        offset += length;
        if (index < parameterCount) {
          parameters[index++] = nal;
        } else {
          access.writeBytes(new byte[]{0, 0, 0, 1});
          access.writeBytes(nal);
          int type = codec == 1 ? (nal[0] & 0x1f) : ((nal[0] & 0x7e) >> 1);
          picture |= codec == 1 ? (type == 1 || type == 5) : (type >= 0 && type <= 31);
        }
      }
    } catch (RuntimeException e) {
      return null;
    }
    for (byte[] p : parameters) if (p == null || p.length == 0) return null;
    if (access.size() == 0 || !picture) return null;
    // 部分 Android MediaCodec 不會僅依賴 csd-* 重新注入參數集；
    // 將 SPS/PPS（及 HEVC VPS）放在每個 access unit 前，確保遺失影格後可恢復。
    ByteArrayOutputStream withParameters = new ByteArrayOutputStream(payload.length);
    for (byte[] p : parameters) {
      withParameters.writeBytes(new byte[]{0, 0, 0, 1});
      withParameters.writeBytes(p);
    }
    withParameters.writeBytes(access.toByteArray());
    boolean keyframe = keyframeHint || containsKeyframe(codec, access.toByteArray());
    return new EncodedVideoFrame(codec, width, height, parameters[0], parameters[1],
        parameterCount == 3 ? parameters[2] : null, withParameters.toByteArray(), keyframe);
  }

  private static boolean containsKeyframe(int codec, byte[] annexB) {
    int at = 0;
    while (at + 4 < annexB.length) {
      int start = -1;
      for (int i = at; i + 3 < annexB.length; i++) {
        if (annexB[i] == 0 && annexB[i + 1] == 0 && annexB[i + 2] == 0 && annexB[i + 3] == 1) {
          start = i + 4;
          break;
        }
      }
      if (start < 0 || start >= annexB.length) break;
      int type = codec == 1 ? (annexB[start] & 0x1f) : ((annexB[start] & 0x7e) >> 1);
      if ((codec == 1 && type == 5) || (codec == 2 && (type == 19 || type == 20 || type == 21))) return true;
      at = start;
    }
    return false;
  }
}
