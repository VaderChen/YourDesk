package android.media;

public final class MediaCodecList {
  public static final int REGULAR_CODECS = 0;
  public static MediaCodecInfo[] infos = new MediaCodecInfo[0];
  public MediaCodecList(int ignored) { }
  public MediaCodecInfo[] getCodecInfos() { return infos; }
}
