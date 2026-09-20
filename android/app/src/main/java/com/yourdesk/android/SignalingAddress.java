package com.yourdesk.android;

import java.net.URI;

final class SignalingAddress {
  static final String DEFAULT = "wss://desktop.mars-cloud.com:8080/ws";
  private SignalingAddress() { }

  static String validate(String value) {
    if (value != null) for (int i = 0; i < value.length(); i++) {
      char character = value.charAt(i);
      if (character < 0x20 || character == 0x7f) throw new IllegalArgumentException("Signaling 位址包含控制字元");
    }
    String address = value == null ? "" : value.trim();
    if (address.isEmpty()) return DEFAULT;
    try {
      URI uri = new URI(address);
      String scheme = uri.getScheme();
      if (address.length() > 2048 || !("wss".equalsIgnoreCase(scheme) || "https".equalsIgnoreCase(scheme))
          || uri.getHost() == null || uri.getHost().isEmpty() || uri.getUserInfo() != null
          || uri.getFragment() != null || uri.getPort() == 0 || uri.getPort() > 65535)
        throw new IllegalArgumentException("Signaling 位址必須是有效的 WSS 或 HTTPS URL");
      return scheme.toLowerCase(java.util.Locale.ROOT) + uri.toASCIIString().substring(scheme.length());
    } catch (java.net.URISyntaxException error) {
      throw new IllegalArgumentException("Signaling 位址無效", error);
    }
  }
}
