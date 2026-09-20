package com.yourdesk.android;

public final class FramePolicyTest {
  private static void check(boolean result) { if (!result) throw new AssertionError(); }
  public static void main(String[] args) {
    FramePolicy policy = new FramePolicy();
    check(!policy.accept(1, 1920, 1080, 0, false));
    check(policy.accept(10, 1920, 1080, 0, true));
    policy.commit(10, 1920, 1080, 0);
    check(!policy.accept(12, 1920, 1080, 0, false));
    check(!policy.accept(13, 1920, 1080, 0, false));
    check(policy.needsKeyframe());
    for (int sequence = 14; sequence < 100_000; sequence++)
      check(!policy.accept(sequence, 1920, 1080, 0, false));
    check(policy.accept(100_000, 1920, 1080, 0, true));
    policy.commit(100_000, 1920, 1080, 0);
    check(!policy.accept(100_000, 1920, 1080, 0, true));
    check(policy.accept(100_001, 1920, 1080, 0, false));
    policy.commit(100_001, 1920, 1080, 0);
    check(!policy.accept(100_002, 1920, 1080, 1, false));
    policy.reset();
    check(policy.accept(1, 3840, 2160, 0, true));
    check(!FramePolicy.dimensions(Integer.MAX_VALUE, Integer.MAX_VALUE));
    check(!FramePolicy.dimensions(8192, 8192));
    check(FramePolicy.dimensions(5120, 2880));
    check(FramePolicy.dimensions(6016, 3384));
    check(!FramePolicy.patch(1, 1, 0, 0, 8192, 8192, true));
    check(!FramePolicy.patch(1920, 1080, Integer.MAX_VALUE, 0, 100, 100, false));
    check(!FramePolicy.patch(1920, 1080, 0, 0, 100, 100, true));
    check(FramePolicy.patch(1920, 1080, 0, 0, 1920, 1080, true));
    check(FramePolicy.patch(1920, 1080, 1820, 980, 100, 100, false));
    System.out.println("FramePolicy: recovery, 100000-frame pressure, dimensions PASS");
  }
}
