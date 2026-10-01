package com.yourdesk.android;

import static org.junit.Assert.*;
import android.content.Context;
import android.content.SharedPreferences;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.json.JSONArray;
import org.json.JSONObject;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;
import org.junit.runner.RunWith;

/** 真實 AndroidKeyStore／SharedPreferences；使用獨立測試資料。 */
@RunWith(AndroidJUnit4.class)
public class SiteStoreDeviceTest {
  private SharedPreferences preferences;
  private SiteStore store;
  @Before public void setup() {
    Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
    preferences = context.getSharedPreferences("release-store-smoke", Context.MODE_PRIVATE);
    assertTrue(preferences.edit().clear().commit());
    store = new SiteStore(preferences);
  }
  @After public void cleanup() { preferences.edit().clear().commit(); }
  private String site(String signal, String name) throws Exception {
    return new JSONObject().put("id", "fixture-peer").put("name", name).put("signal", signal).toString();
  }
  @Test public void credentialsAreScopedEncryptedAndDeletedWithSite() throws Exception {
    String first = "wss://first.example/ws", second = "wss://second.example/ws";
    assertEquals("", store.saveSite(site(first, "第一個站台"), "", "fixture-first-secret", false));
    assertEquals("", store.saveSite(site(second, "第二個站台"), "", "fixture-second-secret", false));
    assertEquals(2, new JSONArray(store.loadSites()).length());
    assertEquals("fixture-first-secret", store.rememberedSecret("fixture-peer", first));
    assertEquals("fixture-second-secret", store.rememberedSecret("fixture-peer", second));
    assertEquals("", store.rememberedSecret("fixture-peer", ""));
    assertFalse(preferences.getString("credentials", "").contains("fixture-first-secret"));
    assertTrue(store.deleteSite(SiteStore.identity("fixture-peer", first)));
    assertEquals("", store.rememberedSecret("fixture-peer", first));
    assertEquals("fixture-second-secret", store.rememberedSecret("fixture-peer", second));
    assertEquals(1, new JSONArray(store.loadSites()).length());
  }
  @Test public void editingPreservesOrForgetsSecretAndRejectsDuplicateAtomically() throws Exception {
    String endpoint = "wss://first.example/ws", key = SiteStore.identity("fixture-peer", endpoint);
    assertEquals("", store.saveSite(site(endpoint, "原名稱"), "", "fixture-secret", false));
    assertEquals("", store.saveSite(site(endpoint, "更新名称"), key, "", false));
    assertEquals("fixture-secret", store.rememberedSecret("fixture-peer", endpoint));
    String before = preferences.getString("credentials", ""), sites = store.loadSites();
    assertFalse(store.saveSite(site(endpoint, "重複"), "", "different", false).isEmpty());
    assertEquals(before, preferences.getString("credentials", ""));
    assertEquals(sites, store.loadSites());
    assertEquals("", store.saveSite(site(endpoint, "清除密碼"), key, "", true));
    assertEquals("", store.rememberedSecret("fixture-peer", endpoint));
    assertFalse(preferences.contains("credentials"));
  }
  @Test public void changingEndpointDoesNotTransferPassword() throws Exception {
    String first = "wss://first.example/ws", second = "wss://second.example/ws";
    assertEquals("", store.saveSite(site(first, "站台"), "", "fixture-secret", false));
    assertEquals("", store.saveSite(site(second, "站台"), SiteStore.identity("fixture-peer", first), "", false));
    assertEquals("", store.rememberedSecret("fixture-peer", first));
    assertEquals("", store.rememberedSecret("fixture-peer", second));
  }
  @Test public void legacyInvalidServerCanBeEditedWithoutLosingOtherSites() throws Exception {
    JSONArray legacy = new JSONArray().put(new JSONObject(site("ws://old.example/ws", "舊站台")))
        .put(new JSONObject(site("wss://working.example/ws", "可用站台")));
    assertTrue(preferences.edit().putString("siteLibrary", legacy.toString()).commit());
    assertEquals("", store.saveSite(site("wss://fixed.example/ws", "已修復站台"), "ws://old.example/ws\nfixture-peer", "", false));
    assertEquals(2, new JSONArray(store.loadSites()).length());
    assertTrue(store.loadSites().contains("working.example"));
  }
  @Test public void invalidOrUnreadableDataIsNotOverwritten() throws Exception {
    assertFalse(store.saveSite(site("http://unsafe.example", "站台"), "", "", false).isEmpty());
    assertEquals("[]", store.loadSites());
    assertTrue(preferences.edit().putString("credentials", "invalid-encrypted-data").commit());
    assertFalse(store.saveSite(site("", "站台"), "", "", false).isEmpty());
    assertEquals("[]", store.loadSites());
    assertEquals("invalid-encrypted-data", preferences.getString("credentials", ""));
  }
}
