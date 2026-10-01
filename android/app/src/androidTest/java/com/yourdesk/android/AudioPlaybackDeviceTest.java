package com.yourdesk.android;

import static org.junit.Assert.*;
import android.app.Instrumentation;
import android.content.Intent;
import android.media.AudioFocusRequest;
import android.media.AudioManager;
import android.os.SystemClock;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import com.yourdesk.android.audio.RemoteAudioPlayer;
import java.io.DataInputStream;
import java.io.EOFException;
import java.util.ArrayList;
import java.util.List;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;
import org.junit.runner.RunWith;

/** 驗證系統解碼器產生非靜音 PCM，並由真實 AudioTrack 消耗取樣框。 */
@RunWith(AndroidJUnit4.class)
public class AudioPlaybackDeviceTest {
  private final Instrumentation instrumentation = InstrumentationRegistry.getInstrumentation();
  private MainActivity activity;
  private AudioManager manager;
  private AudioFocusRequest focus;
  @Before public void open() {
    activity = (MainActivity)instrumentation.startActivitySync(new Intent(instrumentation.getTargetContext(), MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
    manager = (AudioManager)activity.getSystemService(android.content.Context.AUDIO_SERVICE);
    focus = new AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN).setAudioAttributes(RemoteAudioPlayer.attributes()).build();
    assertEquals(AudioManager.AUDIOFOCUS_REQUEST_GRANTED, manager.requestAudioFocus(focus));
  }
  @After public void close() {
    if (manager != null && focus != null) manager.abandonAudioFocusRequest(focus);
    if (activity != null) instrumentation.runOnMainSync(activity::finish);
  }
  @Test public void opusDecodesAndPlays() throws Exception { play(3, fixture("audio-opus.bin")); }
  @Test public void aacDecodesAndPlays() throws Exception { play(1, fixture("audio-aac.bin")); }
  @Test public void pcmPlaysWithoutDecoder() throws Exception {
    List<byte[]> packets = new ArrayList<>();
    for (int frame=0; frame<40; frame++) {
      byte[] data = new byte[4096];
      for (int i=0; i<1024; i++) {
        short value=(short)(1000*Math.sin(2*Math.PI*440*(frame*1024+i)/48000));
        for (int channel=0; channel<2; channel++) {data[i*4+channel*2]=(byte)value; data[i*4+channel*2+1]=(byte)(value>>8);}
      }
      packets.add(data);
    }
    play(2, packets);
  }
  private List<byte[]> fixture(String name) throws Exception {
    List<byte[]> packets = new ArrayList<>();
    try (DataInputStream input = new DataInputStream(instrumentation.getContext().getAssets().open(name))) {
      while (true) {int size; try {size=input.readInt();} catch(EOFException done){break;}
        assertTrue(size > 0 && size <= 8192); byte[] data=new byte[size]; input.readFully(data); packets.add(data);
      }
    }
    return packets;
  }
  private void play(int codec, List<byte[]> packets) throws Exception {
    assertTrue("裝置應有對應聲音解碼器", RemoteAudioPlayer.supports(codec));
    RemoteAudioPlayer player = new RemoteAudioPlayer(codec);
    try {
      long due=SystemClock.uptimeMillis();
      for (byte[] packet : packets) {
        player.queue(packet); due += codec==3 ? 20 : 21;
        while(SystemClock.uptimeMillis()<due) {player.drain();SystemClock.sleep(4);}
      }
      long deadline=SystemClock.uptimeMillis()+2000;
      while(player.framesPlayed()<4096 && SystemClock.uptimeMillis()<deadline) {player.drain();SystemClock.sleep(10);}
      assertTrue("實際解碼至少 0.5 秒取樣框", player.framesDecoded() >= 24000);
      assertTrue("解碼結果不是靜音", player.nonzeroSamples() > 1000);
      assertTrue("已寫入系統播放裝置", player.framesWritten() >= 24000);
      assertTrue("AudioTrack 的硬體播放游標必須前進", player.framesPlayed() >= 4096);
    } finally {player.close();}
    assertEquals("釋放後不再持有播放裝置",0,player.framesPlayed());
  }
}
