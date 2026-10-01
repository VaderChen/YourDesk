package com.yourdesk.android;

import static org.junit.Assert.*;
import android.app.Activity;
import android.app.Instrumentation;
import android.graphics.Rect;
import android.os.SystemClock;
import android.view.InputDevice;
import android.view.MotionEvent;
import android.view.View;
import android.view.accessibility.AccessibilityNodeInfo;
import java.util.concurrent.atomic.AtomicBoolean;

/** 以實際觸控操作原生選單，處理旋轉、捲動位置、動畫及 Popup 焦點。 */
final class NativeMenuTestHelper {
  static void choose(Instrumentation instrumentation, Activity activity, View anchor, String title) throws Exception {
    waitFocus(instrumentation,activity);
    instrumentation.getUiAutomation().waitForIdle(200,4000);
    Rect anchorBounds=new Rect();
    instrumentation.runOnMainSync(()->assertTrue(anchor.getGlobalVisibleRect(anchorBounds)));
    tap(instrumentation,anchorBounds);
    instrumentation.getUiAutomation().waitForIdle(150,4000);
    long deadline=SystemClock.uptimeMillis()+6000;
    boolean searchedFromTop=false;
    while(SystemClock.uptimeMillis()<deadline) {
      AccessibilityNodeInfo root=instrumentation.getUiAutomation().getRootInActiveWindow();
      if(root!=null)try {
        Rect window=new Rect();root.getBoundsInScreen(window);
        for(AccessibilityNodeInfo item:root.findAccessibilityNodeInfosByText(title))try {
          if(!item.isVisibleToUser() || !title.contentEquals(item.getText()==null?"":item.getText()))continue;
          Rect bounds=new Rect();item.getBoundsInScreen(bounds);
          if(!bounds.intersect(window) || bounds.isEmpty())continue;
          tap(instrumentation,bounds);
          waitFocus(instrumentation,activity);
          return;
        }finally{item.recycle();}
        int direction=searchedFromTop?AccessibilityNodeInfo.ACTION_SCROLL_FORWARD:AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD;
        searchedFromTop=true;
        if(scroll(root,direction)) {
          SystemClock.sleep(350);
          instrumentation.getUiAutomation().waitForIdle(150,4000);
        }
      }finally{root.recycle();}
      SystemClock.sleep(40);
    }
    fail("找不到原生選單項目："+title);
  }

  private static void tap(Instrumentation instrumentation,Rect bounds) {
    long now=SystemClock.uptimeMillis();
    for(int action:new int[]{MotionEvent.ACTION_DOWN,MotionEvent.ACTION_UP}) {
      MotionEvent event=MotionEvent.obtain(now,SystemClock.uptimeMillis(),action,bounds.centerX(),bounds.centerY(),0);
      event.setSource(InputDevice.SOURCE_TOUCHSCREEN);
      try{instrumentation.sendPointerSync(event);}finally{event.recycle();}
    }
    instrumentation.waitForIdleSync();
  }

  private static void waitFocus(Instrumentation instrumentation,Activity activity) {
    long deadline=SystemClock.uptimeMillis()+5000;
    while(SystemClock.uptimeMillis()<deadline) {
      AtomicBoolean ready=new AtomicBoolean();
      instrumentation.runOnMainSync(()->ready.set(activity.hasWindowFocus()
          && activity.getResources().getConfiguration().orientation==android.content.res.Configuration.ORIENTATION_LANDSCAPE));
      if(ready.get())return;
      SystemClock.sleep(20);
    }
    fail("桌面尚未完成橫向配置或未取得焦點");
  }

  private static boolean scroll(AccessibilityNodeInfo node,int direction) {
    if(node.isScrollable() && node.performAction(direction))return true;
    for(int i=0;i<node.getChildCount();i++) {
      AccessibilityNodeInfo child=node.getChild(i);
      if(child==null)continue;
      try{if(scroll(child,direction))return true;}finally{child.recycle();}
    }
    return false;
  }
}
