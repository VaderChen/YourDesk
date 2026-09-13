package com.yourdesk.android.video;

import android.media.MediaCodecInfo;
import android.media.MediaCodecList;
import android.os.Build;

import java.util.Locale;

/** Android 端硬體解碼器探測；結果只作協商依據，不把存在的 codec 當成可用。 */
public final class DecoderSupport {
  private DecoderSupport() { }

  public static String probe() {
    StringBuilder result = new StringBuilder();
    append(result, "image/jpeg", "JPEG");
    append(result, "video/avc", "H.264");
    append(result, "video/hevc", "HEVC");
    return result.toString();
  }

  /** 回傳可供 Host 選擇的 wire codec；JPEG 永遠保留作可靠的軟體備援。 */
  public static byte[] supportedWireCodecs() {
    java.io.ByteArrayOutputStream out = new java.io.ByteArrayOutputStream();
    out.write(0);
    if (findDecoder("video/avc", false) != null) out.write(1);
    if (findDecoder("video/hevc", false) != null) out.write(2);
    return out.toByteArray();
  }

  /** 只公告已找到硬體 decoder 的 codec，供 Host 的編碼器優先排序。 */
  public static byte[] hardwareWireCodecs() {
    java.io.ByteArrayOutputStream out = new java.io.ByteArrayOutputStream();
    if (findHardwareDecoder("video/avc") != null) out.write(1);
    if (findHardwareDecoder("video/hevc") != null) out.write(2);
    return out.toByteArray();
  }

  private static void append(StringBuilder result, String mime, String label) {
    String name = findHardwareDecoder(mime);
    if (result.length() > 0) result.append("；");
    result.append(label).append('=').append(name == null ? "無" : name);
  }

  /** 回傳第一個非 software decoder 的名稱；null 表示沒有候選。 */
  public static String findHardwareDecoder(String mime) {
    String[] candidates = hardwareDecoders(mime);
    return candidates.length == 0 ? null : candidates[0];
  }

  /**
   * 回傳所有硬體候選，讓 runtime configure 失敗時能嘗試下一個實際 codec。
   * MediaCodecList 的順序不是所有裝置都可靠，因此保留完整清單而不只取第一個。
   */
  public static String[] hardwareDecoders(String mime) {
    java.util.ArrayList<String> result = new java.util.ArrayList<>();
    try {
      MediaCodecList list = new MediaCodecList(MediaCodecList.REGULAR_CODECS);
      for (MediaCodecInfo info : list.getCodecInfos()) {
        if (info.isEncoder()) continue;
        boolean supports = false;
        for (String type : info.getSupportedTypes()) {
          if (mime.equalsIgnoreCase(type)) {
            supports = true;
            break;
          }
        }
        if (!supports || !isHardware(info)) continue;
        if (!result.contains(info.getName())) result.add(info.getName());
      }
    } catch (RuntimeException ignored) {
      // codec 清單不是所有裝置都能在背景啟動階段讀取。
    }
    return result.toArray(new String[0]);
  }

  private static boolean isHardware(MediaCodecInfo info) {
    if (Build.VERSION.SDK_INT >= 29) {
      return !info.isSoftwareOnly() && info.isHardwareAccelerated();
    }
    String name = info.getName().toLowerCase(Locale.ROOT);
    return !name.contains("google") && !name.contains("software") && !name.contains("sw.");
  }

  private static String findDecoder(String mime, boolean hardwareOnly) {
    try {
      MediaCodecList list = new MediaCodecList(MediaCodecList.REGULAR_CODECS);
      for (MediaCodecInfo info : list.getCodecInfos()) {
        if (info.isEncoder()) continue;
        boolean supports = false;
        for (String type : info.getSupportedTypes()) {
          if (mime.equalsIgnoreCase(type)) {
            supports = true;
            break;
          }
        }
        if (!supports) continue;
        if (hardwareOnly && !isHardware(info)) continue;
        return info.getName();
      }
    } catch (RuntimeException ignored) {
      // codec 清單不是所有裝置都能在背景啟動階段讀取。
    }
    return null;
  }
}
