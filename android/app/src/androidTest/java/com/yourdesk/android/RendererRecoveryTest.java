package com.yourdesk.android;

import static org.junit.Assert.*;
import android.app.Instrumentation;
import android.content.Intent;
import android.graphics.Bitmap;
import android.graphics.Color;
import android.util.Base64;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.json.JSONObject;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;
import org.junit.runner.RunWith;
import java.io.ByteArrayOutputStream;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.util.concurrent.atomic.AtomicReference;

/** Real Activity/Bitmap regression, intentionally independent of a remote Host. */
@RunWith(AndroidJUnit4.class)
public class RendererRecoveryTest {
  private final Instrumentation instrumentation = InstrumentationRegistry.getInstrumentation();
  private MainActivity activity;

  @Before public void open() {
    activity = (MainActivity) instrumentation.startActivitySync(new Intent(instrumentation.getTargetContext(), MainActivity.class)
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
  }
  @After public void close() { instrumentation.runOnMainSync(() -> activity.finish()); }

  private String frame(int sequence, boolean keyframe, int fullWidth, int fullHeight,
      int patchWidth, int patchHeight, int color) throws Exception {
    Bitmap bitmap = Bitmap.createBitmap(patchWidth, patchHeight, Bitmap.Config.ARGB_8888);
    bitmap.eraseColor(color);
    ByteArrayOutputStream output = new ByteArrayOutputStream();
    bitmap.compress(Bitmap.CompressFormat.JPEG, 100, output);
    bitmap.recycle();
    return new JSONObject().put("codec", 0).put("sequence", sequence).put("display", 0)
        .put("keyframe", keyframe).put("width", fullWidth).put("height", fullHeight)
        .put("data", Base64.encodeToString(output.toByteArray(), Base64.NO_WRAP)).toString();
  }

  private boolean apply(String json) throws Exception {
    AtomicReference<Boolean> result = new AtomicReference<>();
    AtomicReference<Throwable> failure = new AtomicReference<>();
    instrumentation.runOnMainSync(() -> {
      try {
        Method method = MainActivity.class.getDeclaredMethod("applyFrameJSON", String.class);
        method.setAccessible(true);
        result.set((Boolean) method.invoke(activity, json));
      } catch (Throwable error) { failure.set(error); }
    });
    if (failure.get() != null) throw new AssertionError(failure.get());
    return result.get();
  }

  @Test public void missingDeltaRequiresCompleteKeyframe() throws Exception {
    assertTrue(apply(frame(10, true, 32, 32, 32, 32, Color.BLUE)));
    assertFalse(apply(frame(12, false, 32, 32, 8, 8, Color.RED)));
    assertFalse(apply(frame(13, false, 32, 32, 8, 8, Color.GREEN)));
    assertTrue(apply(frame(14, true, 32, 32, 32, 32, Color.RED)));
    assertTrue(apply(frame(15, false, 32, 32, 8, 8, Color.GREEN)));
  }

  @Test public void oversizedOrPartialKeyframeIsRejectedBeforeComposition() throws Exception {
    assertFalse(apply(frame(1, true, 1, 1, 32, 32, Color.RED)));
    assertFalse(apply(frame(2, true, 32, 32, 8, 8, Color.RED)));
    assertTrue(apply(frame(3, true, 32, 32, 32, 32, Color.BLUE)));
    instrumentation.runOnMainSync(() -> {
      try {
        Field field = MainActivity.class.getDeclaredField("composedFrame");
        field.setAccessible(true);
        Bitmap bitmap = (Bitmap) field.get(activity);
        assertTrue("Full JPEG is directly reused as mutable baseline", bitmap.isMutable());
        assertEquals(32, bitmap.getWidth());
      } catch (ReflectiveOperationException error) { throw new AssertionError(error); }
    });
  }
}
