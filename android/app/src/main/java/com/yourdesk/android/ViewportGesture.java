package com.yourdesk.android;

import android.view.MotionEvent;

/** 多指手勢整段留在本機；換指時重設基準，剩下一指不轉成遠端滑鼠操作。 */
final class ViewportGesture {
  interface Listener {
    void onBegin();
    void onTransform(float factor, float fromX, float fromY, float toX, float toY);
  }
  private final Listener listener;
  private boolean active;
  private int first = -1, second = -1;
  private float focusX, focusY, span;

  ViewportGesture(Listener listener) { this.listener = listener; }
  void reset() { active = false; first = second = -1; span = 0; }

  boolean onTouch(MotionEvent event, boolean panMode) {
    int action = event.getActionMasked();
    if (action == MotionEvent.ACTION_DOWN) {
      reset();
      if (!panMode) return false;
      active = true; listener.onBegin(); rebase(event, -1); return true;
    }
    if (action == MotionEvent.ACTION_POINTER_DOWN) {
      if (!active) { active = true; listener.onBegin(); }
      rebase(event, -1); return true;
    }
    if (!active) return false;
    if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) { reset(); return true; }
    if (action == MotionEvent.ACTION_POINTER_UP) { rebase(event, event.getActionIndex()); return true; }
    if (action == MotionEvent.ACTION_MOVE) {
      int a = event.findPointerIndex(first), b = event.findPointerIndex(second);
      if (a < 0) { rebase(event, -1); return true; }
      float x = event.getX(a), y = event.getY(a), distance = 0;
      if (b >= 0) {
        distance = (float) Math.hypot(x - event.getX(b), y - event.getY(b));
        x = (x + event.getX(b)) * .5f; y = (y + event.getY(b)) * .5f;
      }
      if (b >= 0 || panMode) listener.onTransform(span > 1 && distance > 1 ? distance / span : 1,
          focusX, focusY, x, y);
      focusX = x; focusY = y; span = distance;
    }
    return true;
  }

  private void rebase(MotionEvent event, int excluded) {
    first = second = -1;
    int a = -1, b = -1;
    for (int i = 0; i < event.getPointerCount(); i++) {
      if (i == excluded) continue;
      if (a < 0) a = i;
      else { b = i; break; }
    }
    if (a < 0) return;
    first = event.getPointerId(a); focusX = event.getX(a); focusY = event.getY(a); span = 0;
    if (b >= 0) {
      second = event.getPointerId(b);
      span = (float) Math.hypot(focusX - event.getX(b), focusY - event.getY(b));
      focusX = (focusX + event.getX(b)) * .5f; focusY = (focusY + event.getY(b)) * .5f;
    }
  }
}
