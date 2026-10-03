package com.yourdesk.android.video;

import java.io.ByteArrayOutputStream;
import java.lang.management.ManagementFactory;
import com.sun.management.ThreadMXBean;

/** JVM parser microbenchmark only; no JNI, Android runtime, or hardware decoding. */
public final class VideoFrameBenchmark {
  private static volatile EncodedVideoFrame retained;

  public static void main(String[] args) {
    ThreadMXBean memory = (ThreadMXBean) ManagementFactory.getThreadMXBean();
    if (!memory.isThreadAllocatedMemorySupported()) throw new IllegalStateException("Allocation counter unavailable");
    memory.setThreadAllocatedMemoryEnabled(true);
    long thread = Thread.currentThread().getId();
    System.out.println("codec,pictureBytes,round,nsPerParse,bytesPerParse");
    for (int codec : new int[]{1, 2}) {
      for (int size : new int[]{256 * 1024, 1024 * 1024, 4 * 1024 * 1024}) {
        byte[] picture = new byte[size];
        picture[0] = codec == 1 ? (byte) 0x65 : (byte) 0x26;
        picture[1] = 1;
        byte[][] nals = codec == 1 ? new byte[][]{{0x67, 100, 0, 40}, {0x68, 1}, picture}
            : new byte[][]{{0x40, 1, 1}, {0x42, 1, 1, 2}, {0x44, 1, 1}, picture};
        ByteArrayOutputStream wire = new ByteArrayOutputStream();
        wire.write(nals.length - 1);
        for (byte[] nal : nals) {
          wire.write(nal.length >>> 24); wire.write(nal.length >>> 16);
          wire.write(nal.length >>> 8); wire.write(nal.length); wire.writeBytes(nal);
        }
        byte[] payload = wire.toByteArray();
        for (int i = 0; i < 200; i++) parse(codec, payload);
        for (int round = 1; round <= 3; round++) {
          int iterations = 200;
          long allocated = memory.getThreadAllocatedBytes(thread), started = System.nanoTime();
          for (int i = 0; i < iterations; i++) parse(codec, payload);
          long elapsed = System.nanoTime() - started;
          long bytes = memory.getThreadAllocatedBytes(thread) - allocated;
          System.out.printf("%d,%d,%d,%d,%d%n", codec, size, round, elapsed / iterations, bytes / iterations);
        }
      }
    }
  }

  private static void parse(int codec, byte[] payload) {
    retained = EncodedVideoFrame.parse(codec, 1920, 1080, payload, true);
    if (retained == null) throw new AssertionError("Invalid benchmark frame");
  }
}
