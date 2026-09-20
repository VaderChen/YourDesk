package android.media;

import java.nio.ByteBuffer;
import java.util.HashMap;
import java.util.Map;

/** Host-only API fake. This directory is deliberately outside Gradle's Android source sets. */
public final class MediaFormat {
  public static final String MIMETYPE_VIDEO_AVC = "video/avc", MIMETYPE_VIDEO_HEVC = "video/hevc";
  public static final String KEY_MAX_INPUT_SIZE = "max-input-size", KEY_PROFILE = "profile";
  private final Map<String, Object> values = new HashMap<>();
  public static MediaFormat createVideoFormat(String mime, int width, int height) {
    MediaFormat f = new MediaFormat();
    f.values.put("mime", mime); f.setInteger("width", width); f.setInteger("height", height);
    return f;
  }
  public void setInteger(String key, int value) { values.put(key, value); }
  public void setByteBuffer(String key, ByteBuffer value) { values.put(key, value); }
  public int getInteger(String key) { return (Integer) values.get(key); }
  public ByteBuffer getByteBuffer(String key) { return (ByteBuffer) values.get(key); }
  public boolean containsKey(String key) { return values.containsKey(key); }
}
