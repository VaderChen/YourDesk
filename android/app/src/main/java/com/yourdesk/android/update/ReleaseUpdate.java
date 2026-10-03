package com.yourdesk.android.update;

import java.io.IOException;
import java.net.URI;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Locale;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

/** Android 發行識別；不把只有桌面附件的 latest 當成手機新版。 */
public final class ReleaseUpdate {
  public static final String REPOSITORY = "https://github.com/VaderChen/YourDesk/";
  public static final String FEED = "https://api.github.com/repos/VaderChen/YourDesk/releases";
  public static final long MAX_ZIP_BYTES = 128L * 1024 * 1024;
  public static final long MAX_APK_BYTES = 256L * 1024 * 1024;
  public static final int MAX_JSON_BYTES = 2 * 1024 * 1024;
  private static final Pattern VERSION = Pattern.compile("([0-9]{1,3})\\.([0-9]{2})\\.([0-9]{4}) build ([0-9]{4})");
  private static final Pattern HASH = Pattern.compile("[a-f0-9]{64}");

  public final String version, archiveName, archiveUrl, archiveHash, apkName, apkHash, signerHash;
  public final long versionCode, archiveSize;

  public ReleaseUpdate(String version, long versionCode, String archiveName, String archiveUrl,
      String archiveHash, long archiveSize, String apkName, String apkHash, String signerHash) throws IOException {
    versionKey(version);
    String stem = "YourDesk-" + version.replace(' ', '-') + "-android-arm64";
    if (versionCode <= 0 || versionCode > Integer.MAX_VALUE || archiveSize <= 0 || archiveSize > MAX_ZIP_BYTES
        || !archiveName.equals(stem + ".zip") || !apkName.equals(stem + ".apk")) throw invalid();
    assetUrl(archiveUrl, archiveName);
    this.version = version; this.versionCode = versionCode;
    this.archiveName = archiveName; this.archiveUrl = archiveUrl; this.archiveSize = archiveSize;
    this.archiveHash = hash(archiveHash); this.apkName = apkName;
    this.apkHash = hash(apkHash); this.signerHash = hash(signerHash);
  }

  public interface Fetcher { byte[] fetch(String url, int maximum) throws IOException; }

  public static ReleaseUpdate find(Fetcher fetcher, long installedCode, String installedVersion) throws IOException {
    long current = versionKey(installedVersion);
    List<Candidate> candidates = new ArrayList<>();
    try {
      // 有限分頁涵蓋獨立 Android Release 與桌面／Android 合併發行。
      for (int page = 1; page <= 10; page++) {
        JSONArray releases = new JSONArray(new String(fetcher.fetch(FEED + "?per_page=20&page=" + page,
            MAX_JSON_BYTES), java.nio.charset.StandardCharsets.UTF_8));
        for (int i = 0; i < releases.length(); i++) {
          JSONObject release = releases.getJSONObject(i);
          if (release.optBoolean("draft", true) || release.optBoolean("prerelease", true)) continue;
          String tag = release.optString("tag_name", "");
          String version = (tag.startsWith("android-") ? tag.substring(8) : tag).replace("-build-", " build ");
          long key;
          try { key = versionKey(version); } catch (IOException ignored) { continue; }
          if (key <= current) continue;
          String name = "YourDesk-" + version.replace(' ', '-') + "-android-arm64.zip";
          JSONObject zip = null, manifest = null;
          boolean duplicate = false;
          JSONArray assets = release.optJSONArray("assets");
          if (assets == null) continue;
          for (int j = 0; j < assets.length(); j++) {
            JSONObject asset = assets.getJSONObject(j);
            if (!"uploaded".equals(asset.optString("state"))) continue;
            if (name.equals(asset.optString("name"))) { duplicate |= zip != null; zip = asset; }
            if ((name + ".json").equals(asset.optString("name"))) { duplicate |= manifest != null; manifest = asset; }
          }
          if (zip == null || manifest == null || duplicate) continue;
          try { candidates.add(new Candidate(version, tag, zip, manifest)); }
          catch (IOException | JSONException ignored) { /* 無完整雜湊／附件的 Release 不可套用。 */ }
        }
        if (releases.length() < 20) break;
      }
      candidates.sort(Comparator.comparingLong((Candidate c) -> c.key).reversed());
      for (Candidate candidate : candidates) {
        byte[] bytes = fetcher.fetch(candidate.manifestUrl, 64 * 1024);
        if (bytes.length != candidate.manifestSize || !UpdateFiles.sha256(bytes).equals(candidate.manifestHash)) throw invalid();
        ReleaseUpdate value = fromJson(new JSONObject(new String(bytes, java.nio.charset.StandardCharsets.UTF_8)), candidate.zipUrl);
        if (!value.version.equals(candidate.version) || value.archiveSize != candidate.zipSize
            || !value.archiveHash.equals(candidate.zipHash)) throw invalid();
        // 顯示版號與 versionCode 都必須向前；找到最新可用版本後不再下載歷史 manifest。
        if (value.versionCode > installedCode) return value;
      }
      return null;
    } catch (JSONException | IllegalArgumentException error) {
      throw new IOException("Invalid Android release metadata", error);
    }
  }

  public static ReleaseUpdate fromJson(JSONObject json, String url) throws IOException, JSONException {
    if (!"com.yourdesk.android".equals(json.getString("applicationId"))
        || !"arm64-v8a".equals(json.getString("abi"))) throw invalid();
    JSONObject archive = json.getJSONObject("archive");
    String apk = json.getString("apkFile"), apkHash = json.getString("sha256");
    if (!apkHash.equals(archive.getJSONObject("entries").getString(apk))) throw invalid();
    return new ReleaseUpdate(json.getString("versionName"), integer(json, "versionCode"), archive.getString("fileName"),
        url, archive.getString("sha256"), integer(archive, "size"), apk, apkHash, json.getString("signerSha256"));
  }

  public JSONObject toJson() throws JSONException {
    return new JSONObject().put("applicationId", "com.yourdesk.android").put("abi", "arm64-v8a")
        .put("versionName", version).put("versionCode", versionCode).put("apkFile", apkName)
        .put("sha256", apkHash).put("signerSha256", signerHash).put("archiveUrl", archiveUrl)
        .put("archive", new JSONObject().put("fileName", archiveName).put("sha256", archiveHash)
            .put("size", archiveSize).put("entries", new JSONObject().put(apkName, apkHash)));
  }

  public static long versionKey(String text) throws IOException {
    Matcher match = VERSION.matcher(text);
    if (!match.matches()) throw invalid();
    return Long.parseLong(match.group(1) + match.group(2) + match.group(3) + match.group(4));
  }

  public static String hash(String value) throws IOException {
    if (!HASH.matcher(value).matches()) throw invalid();
    return value;
  }

  private static long integer(JSONObject object, String key) throws JSONException, IOException {
    Object value = object.get(key);
    if (!(value instanceof Number)) throw invalid();
    long number = ((Number) value).longValue();
    if (((Number) value).doubleValue() != (double) number) throw invalid();
    return number;
  }

  private static void assetUrl(String value, String name) throws IOException {
    try {
      URI uri = URI.create(value);
      String path = uri.getRawPath();
      if (!"https".equals(uri.getScheme()) || !"github.com".equals(uri.getHost()) || uri.getPort() != -1
          || uri.getUserInfo() != null || uri.getRawQuery() != null || uri.getRawFragment() != null
          || path == null || !path.startsWith("/VaderChen/YourDesk/releases/download/")
          || !path.endsWith("/" + name) || path.contains("%") || path.contains("..")) throw invalid();
      String tag = path.substring("/VaderChen/YourDesk/releases/download/".length(), path.length() - name.length() - 1);
      versionKey((tag.startsWith("android-") ? tag.substring(8) : tag).replace("-build-", " build "));
    } catch (IllegalArgumentException error) { throw invalid(); }
  }

  private static IOException invalid() { return new IOException("Invalid Android update package"); }

  private static final class Candidate {
    final String version, zipUrl, zipHash, manifestUrl, manifestHash;
    final long key, zipSize, manifestSize;
    Candidate(String version, String tag, JSONObject zip, JSONObject manifest) throws IOException, JSONException {
      this.version = version; key = versionKey(version);
      String name = zip.getString("name");
      zipUrl = zip.getString("browser_download_url"); manifestUrl = manifest.getString("browser_download_url");
      assetUrl(zipUrl, name); assetUrl(manifestUrl, name + ".json");
      String prefix = REPOSITORY + "releases/download/" + tag + "/";
      if (!zipUrl.equals(prefix + name) || !manifestUrl.equals(prefix + name + ".json")) throw invalid();
      zipHash = digest(zip.getString("digest")); manifestHash = digest(manifest.getString("digest"));
      zipSize = integer(zip, "size"); manifestSize = integer(manifest, "size");
      if (zipSize <= 0 || zipSize > MAX_ZIP_BYTES || manifestSize <= 0 || manifestSize > 64 * 1024) throw invalid();
    }
    private static String digest(String value) throws IOException {
      if (!value.startsWith("sha256:")) throw invalid();
      return hash(value.substring(7).toLowerCase(Locale.ROOT));
    }
  }
}
