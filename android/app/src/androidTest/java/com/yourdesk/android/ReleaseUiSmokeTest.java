package com.yourdesk.android;

import static org.junit.Assert.*;
import android.app.Instrumentation;
import android.content.Intent;
import android.os.SystemClock;
import android.webkit.WebView;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.json.JSONObject;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;
import org.junit.runner.RunWith;
import java.lang.reflect.Field;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

/** 真實 WebView、JS bridge、儲存與相機的手機 Smoke；使用獨立 debug applicationId。 */
@RunWith(AndroidJUnit4.class)
public class ReleaseUiSmokeTest {
  private final Instrumentation instrumentation = InstrumentationRegistry.getInstrumentation();
  private MainActivity activity;
  private String fixture;
  @Before public void open() throws Exception {
    activity = (MainActivity) instrumentation.startActivitySync(new Intent(instrumentation.getTargetContext(), MainActivity.class)
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
    fixture = "smoke-" + java.util.UUID.randomUUID();
    waitJS("typeof window.SiteModel==='object' && typeof openSite==='function'");
  }
  @After public void close() throws Exception {
    if (activity != null) instrumentation.runOnMainSync(() -> {
      activity.new Bridge().deleteSite(SiteStore.identity(fixture, ""));
      activity.finish();
    });
  }
  private Object field(String name) throws Exception {
    Field field = MainActivity.class.getDeclaredField(name);field.setAccessible(true);return field.get(activity);
  }
  private String js(String script) throws Exception {
    CountDownLatch complete = new CountDownLatch(1);
    AtomicReference<String> value = new AtomicReference<>();
    instrumentation.runOnMainSync(() -> {
      try { ((WebView) field("web")).evaluateJavascript(script, result -> { value.set(result); complete.countDown(); }); }
      catch (Exception error) { throw new AssertionError(error); }
    });
    assertTrue("WebView 回覆逾時", complete.await(10, TimeUnit.SECONDS));return value.get();
  }
  private void waitJS(String expression) throws Exception {
    long end = SystemClock.uptimeMillis() + 15000;
    while (SystemClock.uptimeMillis() < end) {
      if ("true".equals(js(expression))) return;
      SystemClock.sleep(100);
    }
    fail("WebView 條件逾時：" + expression);
  }
  @Test public void siteCrudCrossesRealJavascriptBridge() throws Exception {
    js("document.getElementById('add').click();document.getElementById('site-name').value='手機 Smoke';document.getElementById('site-room').value="+JSONObject.quote(fixture)+";document.getElementById('site-secret').value='fixture-secret';document.getElementById('site-form').dispatchEvent(new Event('submit',{cancelable:true}));");
    waitJS("document.getElementById('site-dialog').hidden");
    assertEquals("fixture-secret",activity.new Bridge().rememberedSecret(fixture,""));
    assertTrue(activity.new Bridge().loadSites().contains(fixture));
    js("document.querySelector('#sites .edit').click();document.getElementById('site-name').value='手機 Smoke 已更新';document.getElementById('site-form').dispatchEvent(new Event('submit',{cancelable:true}));");
    waitJS("document.getElementById('site-dialog').hidden");
    assertTrue(activity.new Bridge().loadSites().contains("手機 Smoke 已更新"));
    js("document.querySelector('#sites .delete').click();document.getElementById('delete-confirm').click();");
    waitJS("document.getElementById('delete-dialog').hidden");
    assertFalse(activity.new Bridge().loadSites().contains(fixture));
    assertEquals("",activity.new Bridge().rememberedSecret(fixture,""));
  }
  @Test public void cameraStartsAndCancellationReleasesNativeResources() throws Exception {
    if (!activity.getPackageManager().hasSystemFeature("android.hardware.camera.any")) return;
    try (android.os.ParcelFileDescriptor descriptor = instrumentation.getUiAutomation().executeShellCommand(
        "pm grant " + activity.getPackageName() + " android.permission.CAMERA")) {
      try (java.io.InputStream output = new android.os.ParcelFileDescriptor.AutoCloseInputStream(descriptor)) {
        while(output.read()!=-1) { }
      }
    }
    js("document.getElementById('add').click();document.querySelector('[data-tab=qrcode]').click();");
    waitJS("document.getElementById('site-camera').src.startsWith('data:image/jpeg')");
    js("document.getElementById('site-cancel').click();");
    instrumentation.waitForIdleSync();
    instrumentation.runOnMainSync(() -> {
      try { assertNull(field("qrScanner"));assertNull(field("qrAnalysis")); }
      catch(Exception error){throw new AssertionError(error);}
    });
    js("document.getElementById('add').click();document.querySelector('[data-tab=qrcode]').click();document.getElementById('site-cancel').click();");
    SystemClock.sleep(500);
    instrumentation.runOnMainSync(() -> {
      try { assertNull("取消後非同步初始化不得重新綁定",field("qrAnalysis")); }
      catch(Exception error){throw new AssertionError(error);}
    });
  }
  @Test public void backClosesModalWithoutClosingActivity() throws Exception {
    js("document.getElementById('add').click();");
    instrumentation.runOnMainSync(() -> activity.getOnBackPressedDispatcher().onBackPressed());
    waitJS("document.getElementById('site-dialog').hidden");
    assertFalse(activity.isFinishing());
  }
}
