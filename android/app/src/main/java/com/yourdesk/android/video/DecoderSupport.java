package com.yourdesk.android.video;

import android.media.MediaCodecInfo;
import android.media.MediaCodecList;
import android.media.MediaFormat;
import android.os.Build;

import java.util.ArrayList;
import java.util.Locale;

/** 協商與實際播放共用硬體候選；目前的可靠軟體備援是 JPEG，而非 FFmpeg 版本探測。 */
public final class DecoderSupport {
  private DecoderSupport() { }

  public static String probe() {
    StringBuilder result = new StringBuilder();
    append(result, "image/jpeg", "JPEG");
    append(result, "video/avc", "H.264");
    append(result, "video/hevc", "HEVC");
    return result.toString();
  }

  /** 只公告 queue() 真正會使用的解碼路徑；JPEG 永遠保留。 */
  public static byte[] supportedWireCodecs() {
    java.io.ByteArrayOutputStream out = new java.io.ByteArrayOutputStream();
    out.write(0);
    for (byte codec : hardwareWireCodecs()) out.write(codec);
    return out.toByteArray();
  }

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

  public static String findHardwareDecoder(String mime) {
    String[] candidates = hardwareDecoders(mime);
    return candidates.length == 0 ? null : candidates[0];
  }

  public static String[] hardwareDecoders(String mime) { return hardwareDecoders(mime, null); }

  /** 此影格的尺寸與 profile 必須受支援，configure 才是最後的廠商實作驗證。 */
  public static String[] hardwareDecoders(String mime, MediaFormat format) {
    ArrayList<String> result = new ArrayList<>();
    try {
      for (MediaCodecInfo info : new MediaCodecList(MediaCodecList.REGULAR_CODECS).getCodecInfos()) {
        try {
          if (info.isEncoder() || !isHardware(info)) continue;
          for (String type : info.getSupportedTypes()) {
            if (!mime.equalsIgnoreCase(type)) continue;
            if (format != null && !info.getCapabilitiesForType(type).isFormatSupported(format)) break;
            if (!result.contains(info.getName())) result.add(info.getName());
            break;
          }
        } catch (RuntimeException ignored) {
          // 單一損壞的 codec descriptor 不可遮蔽清單中的其他候選。
        }
      }
    } catch (RuntimeException ignored) { }
    return result.toArray(new String[0]);
  }

  static int androidProfile(EncodedVideoFrame frame) {
    if (frame.codec == 1) {
      switch (frame.profileIdc) {
        case 66: return MediaCodecInfo.CodecProfileLevel.AVCProfileBaseline;
        case 77: return MediaCodecInfo.CodecProfileLevel.AVCProfileMain;
        case 88: return MediaCodecInfo.CodecProfileLevel.AVCProfileExtended;
        case 100: return MediaCodecInfo.CodecProfileLevel.AVCProfileHigh;
        case 110: return MediaCodecInfo.CodecProfileLevel.AVCProfileHigh10;
        case 122: return MediaCodecInfo.CodecProfileLevel.AVCProfileHigh422;
        case 244: return MediaCodecInfo.CodecProfileLevel.AVCProfileHigh444;
        default: return -1;
      }
    }
    if (frame.profileIdc == 1) return MediaCodecInfo.CodecProfileLevel.HEVCProfileMain;
    if (frame.profileIdc == 2) return MediaCodecInfo.CodecProfileLevel.HEVCProfileMain10;
    return -1;
  }

  private static boolean isHardware(MediaCodecInfo info) {
    if (Build.VERSION.SDK_INT >= 29) return !info.isSoftwareOnly() && info.isHardwareAccelerated();
    String name = info.getName().toLowerCase(Locale.ROOT);
    return !name.startsWith("omx.google.") && !name.startsWith("c2.android.")
        && !name.contains("ffmpeg") && !name.contains("software") && !name.contains("sw.");
  }
}
