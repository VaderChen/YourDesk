package com.yourdesk.android;

import android.content.Context;
import android.view.InputDevice;
import android.view.MotionEvent;
import android.view.ViewConfiguration;

import java.util.ArrayList;
import java.util.function.Consumer;

/** 全螢幕左緣右滑：先判斷意圖，再決定退出或交還原本的觸控目標。 */
final class FullscreenExitGesture {
  private final float edgeWidth;
  private final float swipeDistance;
  private final int touchSlop;
  private final Consumer<MotionEvent> dispatchToContent;
  private final Runnable exitFullscreen;
  private final ArrayList<MotionEvent> pending = new ArrayList<>();
  private float startX, startY;
  private boolean swallowing;

  FullscreenExitGesture(Context context, Consumer<MotionEvent> dispatchToContent,
      Runnable exitFullscreen) {
    float density = context.getResources().getDisplayMetrics().density;
    edgeWidth = 24 * density;
    touchSlop = ViewConfiguration.get(context).getScaledTouchSlop();
    swipeDistance = Math.max(64 * density, touchSlop * 3);
    this.dispatchToContent = dispatchToContent;
    this.exitFullscreen = exitFullscreen;
  }

  boolean dispatch(MotionEvent event, boolean enabled, float windowLeft) {
    int action = event.getActionMasked();
    if (action == MotionEvent.ACTION_DOWN) {
      reset();
      float edgeX = event.getRawX() - windowLeft;
      if (!enabled || !event.isFromSource(InputDevice.SOURCE_TOUCHSCREEN)
          || edgeX < 0 || edgeX > edgeWidth) return false;
      startX = event.getRawX();
      startY = event.getRawY();
      pending.add(MotionEvent.obtain(event));
      return true;
    }
    // 退出後仍吞掉同一次手勢的尾端，避免遠端收到沒有 DOWN 的 MOVE／UP。
    if (swallowing) {
      if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) reset();
      return true;
    }
    if (pending.isEmpty()) return false;
    if (action == MotionEvent.ACTION_CANCEL || !enabled) {
      reset();
      swallowing = action != MotionEvent.ACTION_CANCEL && action != MotionEvent.ACTION_UP;
      return true;
    }
    // 等待判斷期間只保留最新 MOVE，長按不會無限制累積事件。
    if (action == MotionEvent.ACTION_MOVE && pending.get(pending.size() - 1).getActionMasked()
        == MotionEvent.ACTION_MOVE) pending.remove(pending.size() - 1).recycle();
    pending.add(MotionEvent.obtain(event));
    float dx = event.getRawX() - startX;
    float dy = Math.abs(event.getRawY() - startY);
    if (event.getPointerCount() == 1 && dx >= swipeDistance && dx > dy * 1.5f) {
      reset();
      swallowing = action != MotionEvent.ACTION_UP;
      exitFullscreen.run();
      return true;
    }
    if (action == MotionEvent.ACTION_UP || event.getPointerCount() > 1 || dx < -touchSlop
        || (dy > touchSlop && dy >= Math.max(0, dx))) {
      // 點擊、垂直拖曳與多指操作保留完整 DOWN/UP 配對；只攔截明確的右滑。
      for (MotionEvent buffered : pending) dispatchToContent.accept(buffered);
      reset();
    }
    return true;
  }

  void reset() {
    for (MotionEvent event : pending) event.recycle();
    pending.clear();
    swallowing = false;
  }
}
