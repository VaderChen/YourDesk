package com.yourdesk.android.video;

import java.io.ByteArrayOutputStream;
import java.util.Arrays;

/** 有界的 Host H.264／HEVC wire parser；NAL 以四位元組 big-endian 長度分隔。 */
public final class EncodedVideoFrame {
  public static final int MAX_INPUT_BYTES = 32 * 1024 * 1024;
  private static final int MAX_PARAMETER_BYTES = 64 * 1024;
  private static final int MAX_NALS = 4096;
  private static final byte[] START_CODE = {0, 0, 0, 1};

  public final int codec;
  public final int width;
  public final int height;
  /** MediaCodec CSD：AVC 為 SPS／PPS；HEVC 為單一 VPS＋SPS＋PPS。均含 Annex B 起始碼。 */
  public final byte[] csd0;
  public final byte[] csd1;
  public final byte[] accessUnit;
  public final boolean keyframe;
  public final int profileIdc;

  private EncodedVideoFrame(int codec, int width, int height, byte[] csd0, byte[] csd1,
      byte[] accessUnit, boolean keyframe, int profileIdc) {
    this.codec = codec;
    this.width = width;
    this.height = height;
    this.csd0 = csd0;
    this.csd1 = csd1;
    this.accessUnit = accessUnit;
    this.keyframe = keyframe;
    this.profileIdc = profileIdc;
  }

  /** 不信任外層 keyframeHint；恢復必須真的含有 IDR／IRAP picture。 */
  public static EncodedVideoFrame parse(int codec, int width, int height, byte[] payload,
      boolean keyframeHint) {
    if ((codec != 1 && codec != 2) || width <= 0 || height <= 0
        || width > 8192 || height > 8192 || (long) width * height > 32L * 1024L * 1024L
        || payload == null || payload.length < 1 || payload.length > MAX_INPUT_BYTES) return null;
    int parameterCount = codec == 1 ? 2 : 3;
    if ((payload[0] & 0xff) != parameterCount) return null;
    byte[][] parameters = new byte[parameterCount][];
    boolean picture = false;
    boolean keyframe = false;
    int offset = 1;
    int index = 0;
    // wire 長度欄位會逐一換成同長 Annex B 起始碼，不再重複配置／掃描完整 AU。
    ByteArrayOutputStream access = new ByteArrayOutputStream(payload.length - 1);
    while (offset < payload.length) {
      if (payload.length - offset < 4 || index >= MAX_NALS) return null;
      int length = ((payload[offset] & 0xff) << 24) | ((payload[offset + 1] & 0xff) << 16)
          | ((payload[offset + 2] & 0xff) << 8) | (payload[offset + 3] & 0xff);
      offset += 4;
      if (length < (codec == 1 ? 1 : 2) || length > payload.length - offset
          || (payload[offset] & 0x80) != 0) return null;
      if (codec == 2 && (payload[offset + 1] & 7) == 0) return null;
      int type = codec == 1 ? (payload[offset] & 0x1f) : ((payload[offset] & 0x7e) >> 1);
      if (index < parameterCount) {
        int expected = codec == 1 ? 7 + index : 32 + index;
        if (type != expected || length > MAX_PARAMETER_BYTES
            || (codec == 1 && index == 0 && length < 4)
            || (codec == 2 && index == 1 && length < 4)) return null;
        parameters[index] = Arrays.copyOfRange(payload, offset, offset + length);
      } else {
        picture |= codec == 1 ? (type == 1 || type == 5) : type <= 31;
        keyframe |= codec == 1 ? type == 5 : type >= 16 && type <= 21;
      }
      access.write(START_CODE, 0, START_CODE.length);
      access.write(payload, offset, length);
      offset += length;
      index++;
    }
    if (index <= parameterCount || !picture) return null;
    byte[] csd0;
    byte[] csd1 = null;
    int profileIdc;
    if (codec == 1) {
      csd0 = annexB(parameters[0]);
      csd1 = annexB(parameters[1]);
      profileIdc = parameters[0][1] & 0xff;
    } else {
      ByteArrayOutputStream csd = new ByteArrayOutputStream();
      for (byte[] p : parameters) {
        csd.write(START_CODE, 0, START_CODE.length);
        csd.write(p, 0, p.length);
      }
      csd0 = csd.toByteArray();
      profileIdc = parameters[1][3] & 0x1f;
    }
    return new EncodedVideoFrame(codec, width, height, csd0, csd1, access.toByteArray(),
        keyframe, profileIdc);
  }

  private static byte[] annexB(byte[] nal) {
    byte[] result = new byte[START_CODE.length + nal.length];
    System.arraycopy(START_CODE, 0, result, 0, START_CODE.length);
    System.arraycopy(nal, 0, result, START_CODE.length, nal.length);
    return result;
  }
}
