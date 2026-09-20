package com.yourdesk.android;

public final class SignalingAddressTest {
  public static void main(String[] args) {
    if (!SignalingAddress.DEFAULT.equals(SignalingAddress.validate(""))) throw new AssertionError();
    for (String valid : new String[]{"wss://example.test/ws", "wss://localhost:8443/ws", "wss://[::1]:8443/ws", "https://example.test:8081"})
      if (!valid.equals(SignalingAddress.validate(valid))) throw new AssertionError(valid);
    if (!"wss://example.test/ws".equals(SignalingAddress.validate("WSS://example.test/ws"))) throw new AssertionError();
    for (String invalid : new String[]{"ws://example.test/ws", "http://example.test/ws", "wss:///ws",
        "wss://user:pass@example.test/ws", "wss://example.test/ws#fragment", "wss://example.test:0/ws",
        "wss://example.test:65536/ws", "wss://example.test/\n"}) {
      try { SignalingAddress.validate(invalid); throw new AssertionError(invalid); }
      catch (IllegalArgumentException expected) { }
    }
    System.out.println("SignalingAddress: default/custom/TLS validation PASS");
  }
}
