package com.yourdesk.android.video;

import android.media.MediaCodec;
import android.media.MediaCodecInfo;
import android.media.MediaCodecList;
import android.media.MediaFormat;
import android.os.Build;
import android.view.Surface;

import java.io.ByteArrayOutputStream;
import java.nio.ByteBuffer;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Arrays;
import java.util.Random;

/** JVM regression tests of real application classes with explicit Android API fakes, not device decoding. */
public final class VideoDecoderHostTest {
  private static int assertions;
  private static final byte[] SPS = {0x67, 100, 0, 40}, PPS = {0x68, 1}, IDR = {0x65, 1}, DELTA = {0x41, 1};
  public static void main(String[] args) throws Exception {
    avcAndHevcCsd();
    parserBoundsAndMalformed();
    fixture(Files.readAllBytes(Path.of(args[0])));
    advertisedPaths();
    formatFilteringAndCandidateFallback();
    bufferOwnershipAndBoundedFailure();
    resetAndKeyframeRecovery();
    parserFuzz();
    largeAccessUnitStress();
    System.out.println("VideoDecoderHostTest PASS: " + assertions + " assertions (API fakes, no hardware decode)");
  }

  private static void avcAndHevcCsd() {
    EncodedVideoFrame avc = frame(true);
    check(avc != null && avc.keyframe && avc.profileIdc == 100, "AVC IDR/profile parsed");
    equal(avc.csd0, annex(SPS), "AVC SPS starts with Annex B code");
    equal(avc.csd1, annex(PPS), "AVC PPS starts with Annex B code");
    equal(avc.accessUnit, annex(SPS, PPS, IDR), "AU contains one copy of parameter sets");
    byte[] vps = {0x40, 1, 1}, sps = {0x42, 1, 1, 2}, pps = {0x44, 1, 1}, irap = {0x26, 1, 1};
    EncodedVideoFrame hevc = EncodedVideoFrame.parse(2, 1920, 1080, wire(3, vps, sps, pps, irap), false);
    check(hevc != null && hevc.keyframe && hevc.profileIdc == 2, "HEVC IDR/profile parsed");
    equal(hevc.csd0, annex(vps, sps, pps), "HEVC all parameters in one csd-0");
    check(hevc.csd1 == null, "HEVC must not use csd-1");
    check(!EncodedVideoFrame.parse(1, 1920, 1080, wire(2, SPS, PPS, DELTA), true).keyframe,
        "keyframe hint cannot turn a delta into an IDR");
    for (int type = 16; type <= 21; type++) {
      hevc = EncodedVideoFrame.parse(2, 1920, 1080,
          wire(3, vps, sps, pps, new byte[]{(byte) (type << 1), 1, 1}), false);
      check(hevc != null && hevc.keyframe, "HEVC random-access type " + type);
    }
  }

  private static void parserBoundsAndMalformed() {
    byte[] valid = wire(2, SPS, PPS, IDR);
    check(EncodedVideoFrame.parse(3, 1920, 1080, valid, true) == null, "unknown codec");
    check(EncodedVideoFrame.parse(1, 8193, 1080, valid, true) == null, "dimension bound");
    check(EncodedVideoFrame.parse(1, 8192, 8192, valid, true) == null, "pixel bound");
    check(EncodedVideoFrame.parse(1, 1, 1, wire(2, PPS, SPS, IDR), true) == null, "wrong parameter order");
    check(EncodedVideoFrame.parse(1, 1, 1, wire(2, SPS, PPS), true) == null, "no picture");
    check(EncodedVideoFrame.parse(1, 1, 1, wire(2, SPS, PPS, new byte[]{(byte) 0xe5}), true) == null,
        "forbidden NAL bit");
    check(EncodedVideoFrame.parse(1, 1, 1, wire(2, new byte[]{0x67}, PPS, IDR), true) == null,
        "truncated SPS");
    byte[] tooLargeParameter = new byte[65537]; tooLargeParameter[0] = 0x67;
    check(EncodedVideoFrame.parse(1, 1, 1, wire(2, tooLargeParameter, PPS, IDR), true) == null,
        "parameter byte budget");
    for (int i = 0; i < valid.length; i++) {
      check(EncodedVideoFrame.parse(1, 1, 1, Arrays.copyOf(valid, i), true) == null,
          "truncated wire at " + i);
    }
    byte[] malformed = valid.clone(); Arrays.fill(malformed, 1, 5, (byte) 0xff);
    check(EncodedVideoFrame.parse(1, 1, 1, malformed, true) == null, "negative length");
    byte[][] many = new byte[4097][];
    many[0] = SPS; many[1] = PPS; Arrays.fill(many, 2, many.length, IDR);
    check(EncodedVideoFrame.parse(1, 1, 1, wire(2, many), true) == null, "NAL count budget");
  }

  private static void fixture(byte[] payload) {
    EncodedVideoFrame frame = EncodedVideoFrame.parse(1, 1920, 1080, payload, true);
    check(frame != null && frame.keyframe, "real 1080p fixture remains parseable");
    check(frame.csd0.length == 26 && frame.csd1.length == 9, "real SPS/PPS lengths plus start code");
    check(frame.accessUnit.length == payload.length - 1, "no AU duplication");
  }

  private static void advertisedPaths() {
    resetFakes();
    MediaCodecList.infos = new MediaCodecInfo[]{info("c2.android.avc", "video/avc", false),
        info("vendor.hevc", "video/hevc", true)};
    equal(DecoderSupport.supportedWireCodecs(), new byte[]{0, 2}, "do not advertise software-only AVC");
    equal(DecoderSupport.hardwareWireCodecs(), new byte[]{2}, "hardware preference matches playback");
    Build.VERSION.SDK_INT = 28;
    MediaCodecList.infos = new MediaCodecInfo[]{info("OMX.google.avc.decoder", "video/avc", false),
        info("c2.android.hevc.decoder", "video/hevc", false)};
    equal(DecoderSupport.supportedWireCodecs(), new byte[]{0}, "legacy software names excluded");
    Build.VERSION.SDK_INT = 35;
    MediaCodecInfo broken = info("broken", "video/avc", true); broken.descriptorThrows = true;
    MediaCodecList.infos = new MediaCodecInfo[]{broken, info("good", "video/avc", true)};
    check("good".equals(DecoderSupport.findHardwareDecoder("video/avc")), "bad descriptor cannot hide others");
  }

  private static void formatFilteringAndCandidateFallback() {
    resetFakes();
    MediaCodecInfo small = info("small", "video/avc", true); small.maxWidth = 640;
    MediaCodecInfo baseline = info("baseline", "video/avc", true); baseline.onlyProfile = 1;
    MediaCodecList.infos = new MediaCodecInfo[]{small, baseline, info("broken", "video/avc", true),
        info("good", "video/avc", true)};
    plan("broken").configureThrows = true;
    try (MediaCodecVideoDecoder decoder = new MediaCodecVideoDecoder()) {
      check(decoder.queue(frame(true), new Surface()) == 1, "try next valid hardware candidate");
      check("good".equals(decoder.backend()), "working candidate selected");
      check(MediaCodec.instances.size() == 2, "unsupported size/profile never instantiated");
      check(MediaCodec.instances.get(0).released, "failed configure released");
      MediaFormat format = MediaCodec.instances.get(1).configured;
      equal(bytes(format.getByteBuffer("csd-0")), annex(SPS), "actual configure gets Annex B");
      check(format.getInteger(MediaFormat.KEY_PROFILE) == 8, "actual configure gets High profile");
      check(format.getInteger(MediaFormat.KEY_MAX_INPUT_SIZE) >= frame(true).accessUnit.length,
          "max input size covers initial access unit");
      check(!format.containsKey("csd-2"), "never invent csd-2");
    }
    check(MediaCodec.instances.get(1).released, "close releases active decoder");
  }

  private static void bufferOwnershipAndBoundedFailure() {
    for (String failure : new String[]{"small", "null", "queue", "drain", "starved"}) {
      resetFakes(); MediaCodecList.infos = new MediaCodecInfo[]{info("bad", "video/avc", true)};
      MediaCodec.Plan plan = plan("bad");
      plan.capacity = failure.equals("small") ? 1 : 1024;
      plan.inputNull = failure.equals("null"); plan.queueThrows = failure.equals("queue");
      plan.drainThrows = failure.equals("drain"); plan.inputUnavailable = failure.equals("starved");
      MediaCodecVideoDecoder decoder = new MediaCodecVideoDecoder(); Surface surface = new Surface();
      int result = decoder.queue(frame(true), surface);
      if (failure.equals("starved")) {
        check(result == 0 && decoder.needsKeyframe(), "starvation requests fresh IDR");
        decoder.queue(frame(true), surface); result = decoder.queue(frame(true), surface);
      }
      check(result == -1 && decoder.failedWireCodec() == 1, failure + " requests negotiation fallback");
      check(MediaCodec.instances.get(0).released && !MediaCodec.instances.get(0).ownsInput,
          failure + " returns all input ownership by releasing codec");
      check(decoder.consecutiveFailures() == 1, failure + " does not reset failure count while releasing");
      decoder.reset();
      for (int i = 0; i < 100; i++) decoder.queue(frame(true), surface);
      check(MediaCodec.instances.size() == 1, failure + " cannot retry same failed decoder indefinitely");
      decoder.resetSession(); plan.capacity = 1024; plan.inputNull = plan.queueThrows = plan.drainThrows = plan.inputUnavailable = false;
      check(decoder.queue(frame(true), surface) == 1, failure + " new session may retry candidate");
      decoder.close();
    }
  }

  private static void resetAndKeyframeRecovery() {
    resetFakes(); MediaCodecList.infos = new MediaCodecInfo[]{info("first", "video/avc", true), info("second", "video/avc", true)};
    plan("first").capacity = 1;
    try (MediaCodecVideoDecoder decoder = new MediaCodecVideoDecoder()) {
      Surface surface = new Surface();
      check(decoder.queue(frame(true), surface) == -1 && decoder.failedWireCodec() == 0,
          "runtime failure leaves untried candidate available");
      check(decoder.queue(frame(false), surface) == 0 && decoder.needsKeyframe(), "drop delta after failure");
      check(decoder.queue(frame(true), surface) == 1 && "second".equals(decoder.backend()), "IDR chooses next candidate");
      decoder.reset();
      check(decoder.queue(frame(false), surface) == 0 && decoder.needsKeyframe(), "Surface reset requires fresh IDR");
      check(MediaCodec.instances.size() == 2, "delta cannot start a decoder");
      check(decoder.queue(frame(true), surface) == 1, "IDR restores Surface");
      byte[] newSps = SPS.clone(); newSps[3] = 41;
      EncodedVideoFrame changed = EncodedVideoFrame.parse(1, 1920, 1080, wire(2, newSps, PPS, DELTA), false);
      check(decoder.queue(changed, surface) == 0 && decoder.needsKeyframe(), "CSD change on delta cannot reuse decoder");
    }
  }

  private static void parserFuzz() {
    Random random = new Random(20260920);
    for (int i = 0; i < 10000; i++) {
      byte[] payload = new byte[random.nextInt(2048)]; random.nextBytes(payload);
      EncodedVideoFrame f = EncodedVideoFrame.parse(1 + (i & 1), 1920, 1080, payload, true);
      check(f == null || f.accessUnit.length <= payload.length, "malformed packet remains bounded");
    }
  }
  private static void largeAccessUnitStress() {
    resetFakes(); MediaCodecList.infos = new MediaCodecInfo[]{info("large", "video/avc", true)};
    byte[] picture = new byte[1024 * 1024]; picture[0] = 0x65;
    byte[] payload = wire(2, SPS, PPS, picture);
    plan("large").capacity = payload.length;
    try (MediaCodecVideoDecoder decoder = new MediaCodecVideoDecoder()) {
      Surface surface = new Surface();
      for (int i = 0; i < 512; i++) {
        EncodedVideoFrame frame = EncodedVideoFrame.parse(1, 1920, 1080, payload, true);
        check(frame != null && decoder.queue(frame, surface) == 1, "512 MiB repeated access-unit stress");
      }
      check(MediaCodec.instances.size() == 1 && !MediaCodec.instances.get(0).ownsInput,
          "stress reuses one codec without leaked slots");
    }
    byte[] oversized = new byte[EncodedVideoFrame.MAX_INPUT_BYTES + 1];
    check(EncodedVideoFrame.parse(1, 1920, 1080, oversized, true) == null,
        "oversized access unit rejected before parser allocation");
  }
  private static EncodedVideoFrame frame(boolean idr) {
    return EncodedVideoFrame.parse(1, 1920, 1080, wire(2, SPS, PPS, idr ? IDR : DELTA), idr);
  }
  private static MediaCodecInfo info(String name, String mime, boolean hardware) { return new MediaCodecInfo(name, mime, hardware); }
  private static MediaCodec.Plan plan(String name) { return MediaCodec.plans.computeIfAbsent(name, k -> new MediaCodec.Plan()); }
  private static void resetFakes() { MediaCodec.instances.clear(); MediaCodec.plans.clear(); Build.VERSION.SDK_INT = 35; }
  private static byte[] bytes(ByteBuffer source) { ByteBuffer b = source.duplicate(); byte[] result = new byte[b.remaining()]; b.get(result); return result; }
  private static byte[] wire(int count, byte[]... nals) {
    ByteArrayOutputStream out = new ByteArrayOutputStream(); out.write(count);
    for (byte[] nal : nals) { out.write((nal.length >>> 24) & 255); out.write((nal.length >>> 16) & 255);
      out.write((nal.length >>> 8) & 255); out.write(nal.length & 255); out.write(nal, 0, nal.length); }
    return out.toByteArray();
  }
  private static byte[] annex(byte[]... nals) {
    ByteArrayOutputStream out = new ByteArrayOutputStream();
    for (byte[] nal : nals) { out.write(0); out.write(0); out.write(0); out.write(1); out.write(nal, 0, nal.length); }
    return out.toByteArray();
  }
  private static void equal(byte[] actual, byte[] expected, String message) { check(Arrays.equals(actual, expected), message); }
  private static void check(boolean ok, String message) { assertions++; if (!ok) throw new AssertionError(message); }
}
