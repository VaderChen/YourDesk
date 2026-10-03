package com.yourdesk.android.update;

import static org.junit.Assert.*;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;
import org.json.JSONArray;
import org.json.JSONObject;
import org.junit.Test;

public class ReleaseUpdateTest {
  private static final String CURRENT = "1.26.1002 build 2301", NEXT = "1.26.1003 build 1500";
  private static final String HASH = "a".repeat(64);

  private static final class Feed implements ReleaseUpdate.Fetcher {
    final Map<String, byte[]> data = new HashMap<>();
    int calls;
    @Override public byte[] fetch(String url, int limit) throws IOException {
      calls++;
      byte[] value = data.get(url);
      if (value == null) throw new IOException("Unexpected URL: " + url);
      if (value.length > limit) throw new IOException("Limit exceeded");
      return value;
    }
    void page(int page, JSONArray value) { data.put(ReleaseUpdate.FEED + "?per_page=20&page=" + page, bytes(value.toString())); }
  }
  private static byte[] bytes(String text) { return text.getBytes(StandardCharsets.UTF_8); }
  private static JSONObject desktop() throws org.json.JSONException { return new JSONObject().put("tag_name", "1.26.1004-build-1200")
      .put("draft", false).put("prerelease", false).put("assets", new JSONArray()); }
  private static JSONObject manifest(String version, long code) throws org.json.JSONException {
    String stem = "YourDesk-" + version.replace(' ', '-') + "-android-arm64";
    return new JSONObject().put("versionName", version).put("versionCode", code)
        .put("applicationId", "com.yourdesk.android").put("abi", "arm64-v8a").put("apkFile", stem + ".apk")
        .put("sha256", HASH).put("signerSha256", HASH).put("archive", new JSONObject()
            .put("fileName", stem + ".zip").put("sha256", HASH).put("size", 1234)
            .put("entries", new JSONObject().put(stem + ".apk", HASH)));
  }
  private static JSONObject release(Feed feed, String version, long code, boolean standalone) throws org.json.JSONException {
    return release(feed, manifest(version, code), standalone);
  }
  private static JSONObject release(Feed feed, JSONObject metadata, boolean standalone) throws org.json.JSONException {
    String version = metadata.getString("versionName"), tag = (standalone ? "android-" : "") + version.replace(' ', '-');
    String name = "YourDesk-" + version.replace(' ', '-') + "-android-arm64.zip";
    String url = ReleaseUpdate.REPOSITORY + "releases/download/" + tag + "/" + name;
    byte[] data = bytes(metadata.toString()); feed.data.put(url + ".json", data);
    JSONObject zip = new JSONObject().put("name", name).put("state", "uploaded").put("size", 1234)
        .put("digest", "sha256:" + HASH).put("browser_download_url", url);
    JSONObject record = new JSONObject().put("name", name + ".json").put("state", "uploaded")
        .put("size", data.length).put("digest", "sha256:" + UpdateFiles.sha256(data)).put("browser_download_url", url + ".json");
    return new JSONObject().put("tag_name", tag).put("draft", false).put("prerelease", false)
        .put("assets", new JSONArray().put(zip).put(record));
  }
  private static ReleaseUpdate find(Feed feed) throws IOException { return ReleaseUpdate.find(feed, 100, CURRENT); }

  @Test public void skipsDesktopLatestAndFindsCombinedAndroidRelease() throws Exception {
    Feed feed = new Feed(); feed.page(1, new JSONArray().put(desktop()).put(release(feed, NEXT, 101, false)));
    ReleaseUpdate update = find(feed);
    assertEquals(NEXT, update.version); assertEquals(101, update.versionCode);
    assertTrue(update.archiveUrl.endsWith("-android-arm64.zip"));
  }
  @Test public void searchesPastFullDesktopPage() throws Exception {
    Feed feed = new Feed(); JSONArray first = new JSONArray(); for (int i = 0; i < 20; i++) first.put(desktop());
    feed.page(1, first); feed.page(2, new JSONArray().put(release(feed, NEXT, 101, true)));
    assertNotNull(find(feed)); assertEquals(3, feed.calls);
  }
  @Test public void prefersNewestAndroidReleaseAndIgnoresDraftAndPrerelease() throws Exception {
    Feed feed = new Feed();
    feed.page(1, new JSONArray().put(release(feed, "1.26.1004 build 1600", 102, false))
        .put(release(feed, NEXT, 101, true))
        .put(release(feed, "1.26.1005 build 1600", 103, true).put("draft", true))
        .put(release(feed, "1.26.1006 build 1600", 104, true).put("prerelease", true)));
    assertEquals(102, find(feed).versionCode);
    assertEquals("Do not fetch historical Android manifests after finding the newest update", 2, feed.calls);
  }
  @Test public void currentVersionIsNotDownloaded() throws Exception {
    Feed feed = new Feed(); feed.page(1, new JSONArray().put(release(feed, CURRENT, 100, true)));
    assertNull(find(feed)); assertEquals(1, feed.calls);
  }
  @Test public void increasingDisplayVersionCannotDowngradeAndroidCode() throws Exception {
    Feed feed = new Feed(); feed.page(1, new JSONArray().put(release(feed, NEXT, 99, true)));
    assertNull(find(feed));
  }
  @Test public void refusesTamperedManifest() throws Exception {
    Feed feed = new Feed(); JSONObject release = release(feed, NEXT, 101, true);
    String url = release.getJSONArray("assets").getJSONObject(1).getString("browser_download_url");
    feed.data.put(url, bytes("{}")); feed.page(1, new JSONArray().put(release));
    assertThrows(IOException.class, () -> find(feed));
  }
  @Test public void verifiesArchiveAgainstGithubDigest() throws Exception {
    Feed feed = new Feed(); JSONObject metadata = manifest(NEXT, 101);
    metadata.getJSONObject("archive").put("sha256", "b".repeat(64));
    feed.page(1, new JSONArray().put(release(feed, metadata, true)));
    assertThrows(IOException.class, () -> find(feed));
  }
  @Test public void wrongApplicationArchitectureOrApkDigestIsRejected() throws Exception {
    for (String change : new String[]{"applicationId", "abi", "sha256"}) {
      Feed feed = new Feed(); JSONObject metadata = manifest(NEXT, 101).put(change, "bad");
      feed.page(1, new JSONArray().put(release(feed, metadata, true)));
      assertThrows(change, IOException.class, () -> find(feed));
    }
  }
  @Test public void fractionalVersionCodeIsRejected() throws Exception {
    Feed feed = new Feed(); feed.page(1, new JSONArray().put(release(feed, manifest(NEXT, 101).put("versionCode", 101.5), true)));
    assertThrows(IOException.class, () -> find(feed));
  }
  @Test public void untrustedAssetOrMissingGithubHashIsSkipped() throws Exception {
    for (String change : new String[]{"url", "hash"}) {
      Feed feed = new Feed(); JSONObject release = release(feed, NEXT, 101, false);
      JSONObject zip = release.getJSONArray("assets").getJSONObject(0);
      if (change.equals("url")) zip.put("browser_download_url", "https://evil.invalid/update.zip");
      else zip.remove("digest");
      feed.page(1, new JSONArray().put(release)); assertNull(find(feed));
    }
  }
  @Test public void duplicateOrIncompleteAssetsAreSkipped() throws Exception {
    Feed feed = new Feed(); JSONObject release = release(feed, NEXT, 101, false);
    JSONArray assets = release.getJSONArray("assets"); assets.put(assets.getJSONObject(0));
    feed.page(1, new JSONArray().put(release)); assertNull(find(feed));
    assets.remove(2); assets.getJSONObject(1).put("state", "new");
    feed.page(1, new JSONArray().put(release)); assertNull(find(feed));
  }
  @Test public void persistedMetadataRetainsAllValidationFields() throws Exception {
    Feed feed = new Feed(); feed.page(1, new JSONArray().put(release(feed, NEXT, 101, true)));
    ReleaseUpdate value = find(feed); JSONObject saved = value.toJson();
    ReleaseUpdate loaded = ReleaseUpdate.fromJson(saved, saved.getString("archiveUrl"));
    assertEquals(value.versionCode, loaded.versionCode); assertEquals(value.archiveHash, loaded.archiveHash);
    assertEquals(value.apkHash, loaded.apkHash); assertEquals(value.signerHash, loaded.signerHash);
  }
  @Test public void limitsReleasePagination() throws Exception {
    Feed feed = new Feed(); JSONArray data = new JSONArray(); for (int i = 0; i < 20; i++) data.put(desktop());
    for (int page = 1; page <= 10; page++) feed.page(page, data);
    assertNull(find(feed)); assertEquals(10, feed.calls);
  }
  @Test public void onlyOfficialHttpsRedirectHostsAreAllowed() {
    assertTrue(UpdateHttp.allowed("https://release-assets.githubusercontent.com/github-production-release-asset/file?token=public"));
    for (String value : new String[]{"http://github.com/a", "https://github.com.evil.invalid/a", "https://me@github.com/a",
        "https://127.0.0.1/a", "file:///tmp/a", "https://github.com:444/a", "https://github.com/a#fragment"})
      assertFalse(value, UpdateHttp.allowed(value));
  }
}
