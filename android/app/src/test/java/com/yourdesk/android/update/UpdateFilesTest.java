package com.yourdesk.android.update;

import static org.junit.Assert.*;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.InterruptedIOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.zip.ZipEntry;
import java.util.zip.ZipOutputStream;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;

public class UpdateFilesTest {
  @Rule public TemporaryFolder temporary = new TemporaryFolder();
  private static final String VERSION = "1.26.1003 build 1500";
  private static final String STEM = "YourDesk-1.26.1003-build-1500-android-arm64";
  private static final byte[] APK = "synthetic APK fixture".getBytes(StandardCharsets.UTF_8);
  private File archive(String... names) throws Exception {
    File zip = temporary.newFile();
    try (ZipOutputStream output = new ZipOutputStream(Files.newOutputStream(zip.toPath()))) {
      for (String name : names) { output.putNextEntry(new ZipEntry(name)); output.write(APK); output.closeEntry(); }
    }
    return zip;
  }
  private ReleaseUpdate update(File zip) throws Exception {
    return new ReleaseUpdate(VERSION, 101, STEM + ".zip", ReleaseUpdate.REPOSITORY + "releases/download/1.26.1003-build-1500/" + STEM + ".zip",
        UpdateFiles.sha256(zip, () -> false), zip.length(), STEM + ".apk", UpdateFiles.sha256(APK), "a".repeat(64));
  }
  @Test public void streamsExactBytesAndHash() throws Exception {
    ByteArrayOutputStream output = new ByteArrayOutputStream();
    UpdateFiles.copy(new ByteArrayInputStream(APK), output, 100, APK.length, UpdateFiles.sha256(APK), () -> false);
    assertArrayEquals(APK, output.toByteArray());
  }
  @Test public void rejectsTruncationExtraBytesAndCorruptedHash() {
    for (long expected : new long[]{APK.length - 1, APK.length + 1})
      assertThrows(IOException.class, () -> UpdateFiles.copy(new ByteArrayInputStream(APK), new ByteArrayOutputStream(), 100, expected, null, () -> false));
    assertThrows(IOException.class, () -> UpdateFiles.copy(new ByteArrayInputStream(APK), new ByteArrayOutputStream(), 100, APK.length, "b".repeat(64), () -> false));
  }
  @Test public void metadataReadHasHardLimit() {
    assertThrows(IOException.class, () -> UpdateFiles.read(new ByteArrayInputStream(APK), 2, () -> false));
  }
  @Test public void extractsOnlyExpectedApk() throws Exception {
    File zip = archive("README.md", STEM + ".apk", "BUILD.json"), output = new File(temporary.getRoot(), "update.part");
    UpdateFiles.extract(zip, output, update(zip), () -> false);
    assertArrayEquals(APK, Files.readAllBytes(output.toPath()));
    assertFalse(new File(temporary.getRoot(), "README.md").exists());
  }
  @Test public void cancelledExtractionLeavesNoPartialApk() throws Exception {
    File zip = archive(STEM + ".apk"), output = new File(temporary.getRoot(), "update.part");
    assertThrows(InterruptedIOException.class, () -> UpdateFiles.extract(zip, output, update(zip), () -> true));
    assertFalse(output.exists());
  }
  @Test public void cancellationAfterWritingRemovesPartialOutput() throws Exception {
    File zip = archive(STEM + ".apk"), output = new File(temporary.getRoot(), "update.part");
    assertThrows(InterruptedIOException.class,
        () -> UpdateFiles.extract(zip, output, update(zip), () -> output.length() > 0));
    assertFalse(output.exists());
  }
  @Test public void oversizedNonApkEntryIsBoundedEvenWithoutDeclaredSize() throws Exception {
    File zip = temporary.newFile(), output = new File(temporary.getRoot(), "update.part");
    try (ZipOutputStream target = new ZipOutputStream(Files.newOutputStream(zip.toPath()))) {
      target.putNextEntry(new ZipEntry(STEM + ".apk")); target.write(APK); target.closeEntry();
      target.putNextEntry(new ZipEntry("README.md")); target.write(new byte[2 * 1024 * 1024 + 1]); target.closeEntry();
    }
    assertThrows(IOException.class, () -> UpdateFiles.extract(zip, output, update(zip), () -> false));
    assertFalse(output.exists());
  }
  @Test public void refusesTraversalSecondApkAndMissingApk() throws Exception {
    for (String bad : new String[]{"../escape.apk", "dir\\escape.apk", "another.apk"}) {
      File zip = archive(STEM + ".apk", bad), output = new File(temporary.getRoot(), "update.part");
      assertThrows(bad, IOException.class, () -> UpdateFiles.extract(zip, output, update(zip), () -> false));
      assertFalse(output.exists());
    }
    File zip = archive("README.md"), output = new File(temporary.getRoot(), "update.part");
    assertThrows(IOException.class, () -> UpdateFiles.extract(zip, output, update(zip), () -> false));
    assertFalse(output.exists());
  }
  @Test public void rejectsTooManyEntries() throws Exception {
    String[] names = new String[33]; names[0] = STEM + ".apk"; for (int i = 1; i < names.length; i++) names[i] = "file-" + i;
    File zip = archive(names), output = new File(temporary.getRoot(), "update.part");
    assertThrows(IOException.class, () -> UpdateFiles.extract(zip, output, update(zip), () -> false));
    assertFalse(output.exists());
  }
  @Test public void apkHashMismatchRemovesPartialOutput() throws Exception {
    File zip = archive(STEM + ".apk"), output = new File(temporary.getRoot(), "update.part");
    ReleaseUpdate value = update(zip);
    ReleaseUpdate bad = new ReleaseUpdate(VERSION, 101, value.archiveName, value.archiveUrl, value.archiveHash,
        value.archiveSize, value.apkName, "c".repeat(64), value.signerHash);
    assertThrows(IOException.class, () -> UpdateFiles.extract(zip, output, bad, () -> false));
    assertFalse(output.exists());
  }
  @Test public void archiveTamperingIsRejectedBeforeExtraction() throws Exception {
    File zip = archive(STEM + ".apk"), output = new File(temporary.getRoot(), "update.part");
    ReleaseUpdate value = update(zip); Files.write(zip.toPath(), new byte[]{1,2,3});
    assertThrows(IOException.class, () -> UpdateFiles.extract(zip, output, value, () -> false));
    assertFalse(output.exists());
  }
}
