package com.yourdesk.android;

import android.content.SharedPreferences;
import android.util.Base64;
import org.json.JSONArray;
import org.json.JSONObject;

/** 站台與加密密碼共用一次 commit；端點身分包含信令伺服器，避免不同站台共用密碼。 */
final class SiteStore {
  private final SharedPreferences preferences;

  SiteStore(SharedPreferences preferences) { this.preferences = preferences; }

  static String identity(String room, String signal) {
    if (room == null || room.trim().isEmpty() || room.length() > 255
        || room.trim().codePoints().anyMatch(c -> Character.isWhitespace(c) || Character.isISOControl(c)))
      throw new IllegalArgumentException("請輸入有效的遠端 ID 或 IP:Port");
    return SignalingAddress.validate(signal) + "\n" + room.trim();
  }

  private static String storedIdentity(JSONObject site) {
    String signal = site.optString("signal", "").trim();
    int colon = signal.indexOf(':');
    if (colon > 0) signal = signal.substring(0, colon).toLowerCase(java.util.Locale.ROOT) + signal.substring(colon);
    return (signal.isEmpty() ? SignalingAddress.DEFAULT : signal) + "\n" + site.optString("id", "").trim();
  }

  static JSONObject cleanSite(JSONObject site) throws Exception {
    String room = site.getString("id").trim();
    String signal = site.optString("signal", "").trim();
    identity(room, signal);
    String name = site.getString("name").trim(), note = site.optString("note", "").trim();
    if (name.isEmpty() || name.length() > 120 || note.length() > 1024)
      throw new IllegalArgumentException("站台名稱或備註長度無效");
    return new JSONObject().put("id", room).put("name", name).put("note", note)
        .put("signal", signal.isEmpty() ? "" : SignalingAddress.validate(signal))
        .put("online", false).put("terminal", site.optBoolean("terminal", true))
        .put("desktop", site.optBoolean("desktop", true));
  }

  synchronized String loadSites() { return preferences.getString("siteLibrary", "[]"); }

  synchronized boolean saveSites(String json) {
    try {
      JSONArray clean = validatedSites(json);
      return preferences.edit().putString("siteLibrary", clean.toString()).commit();
    } catch (Exception error) { return false; }
  }

  private JSONArray validatedSites(String json) throws Exception {
    if (json == null || json.length() > 4 * 1024 * 1024) throw new IllegalArgumentException("站台資料過大");
    JSONArray input = new JSONArray(json), clean = new JSONArray();
    if (input.length() > 2000) throw new IllegalArgumentException("最多可儲存 2000 個站台");
    java.util.HashSet<String> identities = new java.util.HashSet<>();
    for (int i = 0; i < input.length(); i++) {
      JSONObject site = cleanSite(input.getJSONObject(i));
      if (!identities.add(identity(site.getString("id"), site.getString("signal"))))
        throw new IllegalArgumentException("此站台已存在");
      clean.put(site);
    }
    return clean;
  }

  synchronized String saveSite(String json, String original, String secret, boolean forget) {
    try {
      if (json == null || json.length() > 8192) throw new IllegalArgumentException("站台資料過大");
      JSONObject site = cleanSite(new JSONObject(json));
      String target = identity(site.getString("id"), site.getString("signal"));
      JSONArray current = new JSONArray(loadSites()), next = new JSONArray();
      boolean found = original == null || original.isEmpty();
      for (int i = 0; i < current.length(); i++) {
        JSONObject item = current.getJSONObject(i);
        String key = storedIdentity(item);
        if (key.equals(original)) { found = true; continue; }
        if (key.equals(target)) throw new IllegalArgumentException("此站台已存在");
        next.put(item);
      }
      if (!found) throw new IllegalArgumentException("站台已變更，請重新開啟");
      JSONArray ordered = new JSONArray().put(site);
      for (int i = 0; i < next.length(); i++) ordered.put(next.getJSONObject(i));
      if (ordered.length() > 2000) throw new IllegalArgumentException("最多可儲存 2000 個站台");
      JSONObject credentials = readCredentials();
      if (original != null && !original.isEmpty() && !original.equals(target)) removeSecret(credentials, original);
      if (forget) removeSecret(credentials, target);
      else if (secret != null && !secret.isEmpty()) putSecret(credentials, target, secret);
      SharedPreferences.Editor editor = preferences.edit().putString("siteLibrary", ordered.toString());
      writeCredentials(editor, credentials);
      if (!editor.commit()) throw new IllegalStateException();
      return "";
    } catch (IllegalArgumentException error) { return error.getMessage(); }
    catch (Exception error) { return "站台或密碼儲存失敗，請重試"; }
  }

  synchronized boolean deleteSite(String key) {
    try {
      JSONArray current = new JSONArray(loadSites()), next = new JSONArray();
      boolean found = false;
      for (int i = 0; i < current.length(); i++) {
        JSONObject item = current.getJSONObject(i);
        if (storedIdentity(item).equals(key)) found = true;
        else next.put(item);
      }
      if (!found) return false;
      JSONObject credentials = readCredentials();
      removeSecret(credentials, key);
      SharedPreferences.Editor editor = preferences.edit().putString("siteLibrary", next.toString());
      writeCredentials(editor, credentials);
      return editor.commit();
    } catch (Exception error) { return false; }
  }

  synchronized String rememberedSecret(String room, String signal) {
    try {
      String key = identity(room, signal);
      JSONObject credentials = readCredentials();
      if (credentials.has("v2:" + key)) return credentials.getString("v2:" + key);
      // 舊版僅用 room 作為索引；只讓預設伺服器沿用，避免把密碼帶到自訂伺服器。
      return SignalingAddress.DEFAULT.equals(SignalingAddress.validate(signal))
          ? credentials.optString(room.trim(), "") : "";
    } catch (Exception error) { return ""; }
  }

  synchronized boolean remember(String room, String signal, String secret) {
    try {
      String key = identity(room, signal);
      JSONObject credentials = readCredentials();
      if (secret == null || secret.isEmpty()) removeSecret(credentials, key);
      else putSecret(credentials, key, secret);
      SharedPreferences.Editor editor = preferences.edit();
      writeCredentials(editor, credentials);
      return editor.commit();
    } catch (Exception error) { return false; }
  }

  private void putSecret(JSONObject credentials, String key, String secret) throws Exception {
    if (secret.length() > 4096) throw new IllegalArgumentException("密碼長度無效");
    removeSecret(credentials, key);
    credentials.put("v2:" + key, secret);
  }

  private void removeSecret(JSONObject credentials, String key) {
    credentials.remove("v2:" + key);
    if (key.startsWith(SignalingAddress.DEFAULT + "\n"))
      credentials.remove(key.substring(key.indexOf('\n') + 1));
  }

  private javax.crypto.SecretKey key() throws Exception {
    java.security.KeyStore store = java.security.KeyStore.getInstance("AndroidKeyStore");
    store.load(null);
    if (!store.containsAlias("shell")) {
      javax.crypto.KeyGenerator generator = javax.crypto.KeyGenerator.getInstance("AES", "AndroidKeyStore");
      generator.init(new android.security.keystore.KeyGenParameterSpec.Builder("shell",
          android.security.keystore.KeyProperties.PURPOSE_ENCRYPT | android.security.keystore.KeyProperties.PURPOSE_DECRYPT)
          .setBlockModes(android.security.keystore.KeyProperties.BLOCK_MODE_GCM)
          .setEncryptionPaddings(android.security.keystore.KeyProperties.ENCRYPTION_PADDING_NONE).build());
      generator.generateKey();
    }
    return (javax.crypto.SecretKey) store.getKey("shell", null);
  }

  private JSONObject readCredentials() throws Exception {
    String value = preferences.getString("credentials", "");
    if (value.isEmpty()) return new JSONObject();
    String[] parts = value.split(":", -1);
    if (parts.length != 2) throw new IllegalStateException("密碼資料無效");
    javax.crypto.Cipher cipher = javax.crypto.Cipher.getInstance("AES/GCM/NoPadding");
    cipher.init(javax.crypto.Cipher.DECRYPT_MODE, key(),
        new javax.crypto.spec.GCMParameterSpec(128, Base64.decode(parts[0], Base64.NO_WRAP)));
    JSONObject stored = new JSONObject(new String(cipher.doFinal(Base64.decode(parts[1], Base64.NO_WRAP)),
        java.nio.charset.StandardCharsets.UTF_8));
    JSONObject sites = stored.optJSONObject("sites");
    if (sites != null) return sites;
    JSONObject migrated = new JSONObject();
    if (!stored.optString("room", "").isEmpty()) migrated.put(stored.getString("room"), stored.optString("secret", ""));
    return migrated;
  }

  private void writeCredentials(SharedPreferences.Editor editor, JSONObject credentials) throws Exception {
    editor.remove("room");
    if (credentials.length() == 0) { editor.remove("credentials"); return; }
    javax.crypto.Cipher cipher = javax.crypto.Cipher.getInstance("AES/GCM/NoPadding");
    cipher.init(javax.crypto.Cipher.ENCRYPT_MODE, key());
    byte[] data = new JSONObject().put("sites", credentials).toString().getBytes(java.nio.charset.StandardCharsets.UTF_8);
    editor.putString("credentials", Base64.encodeToString(cipher.getIV(), Base64.NO_WRAP) + ":"
        + Base64.encodeToString(cipher.doFinal(data), Base64.NO_WRAP));
  }
}
