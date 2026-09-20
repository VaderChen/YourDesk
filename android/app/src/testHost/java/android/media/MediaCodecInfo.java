package android.media;

public final class MediaCodecInfo {
  private final String name, mime;
  private final boolean hardware;
  public int maxWidth = 8192, maxHeight = 8192, onlyProfile = -1;
  public boolean descriptorThrows;
  public MediaCodecInfo(String name, String mime, boolean hardware) {
    this.name = name; this.mime = mime; this.hardware = hardware;
  }
  public boolean isEncoder() { return false; }
  public String getName() { return name; }
  public String[] getSupportedTypes() {
    if (descriptorThrows) throw new IllegalStateException("broken descriptor");
    return new String[]{mime};
  }
  public boolean isSoftwareOnly() { return !hardware; }
  public boolean isHardwareAccelerated() { return hardware; }
  public CodecCapabilities getCapabilitiesForType(String ignored) { return new CodecCapabilities(); }
  public final class CodecCapabilities {
    public boolean isFormatSupported(MediaFormat format) {
      return format.getInteger("width") <= maxWidth && format.getInteger("height") <= maxHeight
          && (onlyProfile < 0 || (format.containsKey(MediaFormat.KEY_PROFILE)
              && format.getInteger(MediaFormat.KEY_PROFILE) == onlyProfile));
    }
  }
  public static final class CodecProfileLevel {
    public static final int AVCProfileBaseline = 1, AVCProfileMain = 2, AVCProfileExtended = 4,
        AVCProfileHigh = 8, AVCProfileHigh10 = 16, AVCProfileHigh422 = 32, AVCProfileHigh444 = 64;
    public static final int HEVCProfileMain = 1, HEVCProfileMain10 = 2;
  }
}
