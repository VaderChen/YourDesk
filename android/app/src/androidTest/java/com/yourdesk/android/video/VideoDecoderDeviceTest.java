package com.yourdesk.android.video;

import static org.junit.Assert.*;
import static org.junit.Assume.assumeTrue;

import android.graphics.SurfaceTexture;
import android.os.SystemClock;
import android.view.Surface;

import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;

import org.junit.Test;
import org.junit.runner.RunWith;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;

/** Real MediaCodec smoke; requires an Android device/emulator with advertised AVC hardware support. */
@RunWith(AndroidJUnit4.class)
public final class VideoDecoderDeviceTest {
  @Test public void actualH264FixtureDecodesAndRestartsAfterSurfaceReset() throws Exception {
    assumeTrue("Device has no hardware AVC path", DecoderSupport.findHardwareDecoder("video/avc") != null);
    byte[] payload;
    try (InputStream input = InstrumentationRegistry.getInstrumentation().getContext().getAssets()
        .open("fullscreen-h264-1080.bin")) {
      ByteArrayOutputStream bytes = new ByteArrayOutputStream();
      byte[] chunk = new byte[8192];
      for (int n; (n = input.read(chunk)) != -1;) bytes.write(chunk, 0, n);
      payload = bytes.toByteArray();
    }
    EncodedVideoFrame frame = EncodedVideoFrame.parse(1, 1920, 1080, payload, true);
    assertNotNull(frame);
    assertTrue(frame.keyframe);
    try (MediaCodecVideoDecoder decoder = new MediaCodecVideoDecoder()) {
      // API 26 detached consumer: no Activity, network connection or real user imagery is needed.
      SurfaceTexture texture = new SurfaceTexture(false);
      Surface surface = new Surface(texture);
      try {
        assertRendered(decoder, frame, surface);
        decoder.reset();
        assertTrue(decoder.needsKeyframe());
        assertRendered(decoder, frame, surface);
      } finally {
        decoder.close();
        surface.release();
        texture.release();
      }
    }
  }

  private static void assertRendered(MediaCodecVideoDecoder decoder, EncodedVideoFrame frame, Surface surface) {
    int rendered = decoder.queue(frame, surface);
    long deadline = SystemClock.uptimeMillis() + 5000;
    while (rendered <= 0 && decoder.failedWireCodec() == 0 && SystemClock.uptimeMillis() < deadline) {
      SystemClock.sleep(10);
      // The production caller responds to needsKeyframe() with a new IDR. Emulate that
      // handshake for initial input starvation and bounded fallback to another candidate.
      rendered = decoder.needsKeyframe() ? decoder.queue(frame, surface) : decoder.drainOutput();
    }
    assertTrue("Real decoder must render the fixture; backend=" + decoder.backend()
        + ", failed codec=" + decoder.failedWireCodec(), rendered > 0);
    assertFalse(decoder.needsKeyframe());
  }
}
