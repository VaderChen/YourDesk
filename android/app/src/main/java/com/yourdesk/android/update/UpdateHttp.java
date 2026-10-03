package com.yourdesk.android.update;

import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URI;
import java.net.URL;
import java.util.Set;

/** 僅連線到 GitHub 官方 HTTPS 端點，不帶站台密碼或其他認證資料。 */
public final class UpdateHttp implements ReleaseUpdate.Fetcher, AutoCloseable {
  private static final Set<String> HOSTS = new java.util.HashSet<>(java.util.Arrays.asList("api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com"));
  private volatile boolean cancelled;
  private volatile HttpURLConnection active;

  @Override public byte[] fetch(String url, int maximum) throws IOException {
    HttpURLConnection connection = open(url);
    try (InputStream input = connection.getInputStream()) {
      if (connection.getContentLengthLong() > maximum) throw new IOException("Oversized update metadata");
      return UpdateFiles.read(input, maximum, () -> cancelled);
    } finally { connection.disconnect(); active = null; }
  }

  public void download(ReleaseUpdate update, File target) throws IOException {
    HttpURLConnection connection = open(update.archiveUrl);
    boolean complete = false;
    try (InputStream input = connection.getInputStream(); FileOutputStream output = new FileOutputStream(target)) {
      long size = connection.getContentLengthLong();
      if (size >= 0 && size != update.archiveSize) throw new IOException("Update Content-Length mismatch");
      UpdateFiles.copy(input, output, ReleaseUpdate.MAX_ZIP_BYTES, update.archiveSize, update.archiveHash, () -> cancelled);
      output.getFD().sync(); complete = true;
    } finally {
      connection.disconnect(); active = null;
      if (!complete) target.delete();
    }
  }

  private HttpURLConnection open(String value) throws IOException {
    URL url = new URL(value);
    for (int redirects = 0; redirects <= 5; redirects++) {
      UpdateFiles.check(() -> cancelled);
      if (!allowed(url.toString())) throw new IOException("Untrusted update URL");
      HttpURLConnection connection = (HttpURLConnection) url.openConnection();
      active = connection;
      try {
        UpdateFiles.check(() -> cancelled);
        connection.setInstanceFollowRedirects(false);
        connection.setConnectTimeout(15000); connection.setReadTimeout(30000);
        connection.setRequestProperty("User-Agent", "YourDesk-Android-Updater");
        connection.setRequestProperty("Accept-Encoding", "identity");
        connection.setRequestProperty("Accept", "api.github.com".equals(url.getHost()) ? "application/vnd.github+json" : "application/octet-stream");
        if ("api.github.com".equals(url.getHost())) connection.setRequestProperty("X-GitHub-Api-Version", "2022-11-28");
        int status = connection.getResponseCode();
        if (status == 200) return connection;
        if (status == 301 || status == 302 || status == 303 || status == 307 || status == 308) {
          String location = connection.getHeaderField("Location");
          if (location == null) throw new IOException("Missing update redirect");
          url = new URL(url, location);
        } else throw new IOException("Update HTTP status " + status);
      } catch (IOException | RuntimeException error) {
        connection.disconnect(); active = null; throw error;
      }
      connection.disconnect(); active = null;
    }
    throw new IOException("Too many update redirects");
  }

  public static boolean allowed(String value) {
    try {
      URI uri = URI.create(value);
      return "https".equals(uri.getScheme()) && HOSTS.contains(uri.getHost()) && uri.getUserInfo() == null
          && (uri.getPort() == -1 || uri.getPort() == 443) && uri.getFragment() == null;
    } catch (IllegalArgumentException error) { return false; }
  }

  /** 主執行緒僅發出取消；disconnect 可能排空 socket，必須留在 I/O 執行緒。 */
  public void cancel() { cancelled = true; }

  @Override public void close() {
    cancel();
    HttpURLConnection connection = active;
    if (connection != null) connection.disconnect();
  }
}
