package com.yourdesk.android.update;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.InterruptedIOException;
import java.io.OutputStream;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.HashSet;
import java.util.Set;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;

/** 串流下載／解壓，只輸出指定 APK，不將 ZIP 路徑用作本機輸出位置。 */
public final class UpdateFiles {
  private UpdateFiles() { }
  public interface Cancelled { boolean get(); }

  public static byte[] read(InputStream source, long limit, Cancelled cancelled) throws IOException {
    ByteArrayOutputStream output = new ByteArrayOutputStream();
    copy(source, output, limit, -1, null, cancelled);
    return output.toByteArray();
  }

  public static void copy(InputStream source, OutputStream output, long limit, long expected,
      String expectedHash, Cancelled cancelled) throws IOException {
    MessageDigest digest = digest();
    byte[] buffer = new byte[64 * 1024];
    long count = 0;
    while (true) {
      check(cancelled);
      int length = source.read(buffer);
      if (length == -1) break;
      count += length;
      if (count > limit || (expected >= 0 && count > expected)) throw new IOException("Update exceeds declared size");
      output.write(buffer, 0, length); digest.update(buffer, 0, length);
    }
    check(cancelled);
    if (expected >= 0 && count != expected) throw new IOException("Incomplete update download");
    if (expectedHash != null && !hex(digest.digest()).equals(expectedHash)) throw new IOException("Update SHA-256 mismatch");
  }

  public static void extract(File zip, File output, ReleaseUpdate update, Cancelled cancelled) throws IOException {
    boolean complete = false;
    try {
      if (zip.length() != update.archiveSize || !sha256(zip, cancelled).equals(update.archiveHash))
        throw new IOException("Update archive does not match release");
      // 逐一讀取 local entry，不載入可膨脹至整個 ZIP 大小的 central directory。
      try (ZipInputStream archive = new ZipInputStream(new FileInputStream(zip))) {
        Set<String> names = new HashSet<>();
        boolean found = false;
        OutputStream discard = new OutputStream() {
          @Override public void write(int value) { }
          @Override public void write(byte[] value, int offset, int length) { }
        };
        for (ZipEntry entry; (entry = archive.getNextEntry()) != null;) {
          check(cancelled);
          String name = entry.getName();
          // 官方 Android ZIP 只有根目錄檔案；拒絕重複 APK、路徑跳脫及 ZIP bomb。
          if (!names.add(name) || names.size() > 32 || entry.isDirectory() || name.contains("/")
              || name.contains("\\") || name.contains("..") || name.indexOf('\0') >= 0) throw new IOException("Unsafe update ZIP");
          if (name.endsWith(".apk")) {
            if (!name.equals(update.apkName) || found || entry.getSize() > ReleaseUpdate.MAX_APK_BYTES)
              throw new IOException("Unexpected APK in update");
            try (FileOutputStream target = new FileOutputStream(output)) {
              copy(archive, target, ReleaseUpdate.MAX_APK_BYTES, entry.getSize(), update.apkHash, cancelled);
              target.getFD().sync();
            }
            if (output.length() == 0) throw new IOException("Empty APK in update");
            found = true;
          } else {
            // 說明／授權檔也有限額，避免 skip/closeEntry 無界解壓。
            copy(archive, discard, 2 * 1024 * 1024, entry.getSize(), null, cancelled);
          }
          archive.closeEntry();
        }
        if (!found) throw new IOException("Missing APK");
      }
      complete = true;
    } finally {
      if (!complete) output.delete();
    }
  }

  public static String sha256(File file, Cancelled cancelled) throws IOException {
    MessageDigest digest = digest();
    byte[] buffer = new byte[64 * 1024];
    try (InputStream input = new FileInputStream(file)) {
      for (int count; (count = input.read(buffer)) != -1;) {
        check(cancelled); digest.update(buffer, 0, count);
      }
    }
    check(cancelled);
    return hex(digest.digest());
  }

  public static String sha256(byte[] bytes) { return hex(digest().digest(bytes)); }

  public static void check(Cancelled cancelled) throws InterruptedIOException {
    if (Thread.currentThread().isInterrupted() || cancelled.get()) throw new InterruptedIOException("Update cancelled");
  }

  private static MessageDigest digest() {
    try { return MessageDigest.getInstance("SHA-256"); }
    catch (NoSuchAlgorithmException impossible) { throw new AssertionError(impossible); }
  }

  private static String hex(byte[] value) {
    char[] text = new char[value.length * 2];
    char[] digits = "0123456789abcdef".toCharArray();
    for (int i = 0; i < value.length; i++) { text[i * 2] = digits[(value[i] & 255) >>> 4]; text[i * 2 + 1] = digits[value[i] & 15]; }
    return new String(text);
  }
}
