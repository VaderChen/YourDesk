package com.yourdesk.android.audio;

import android.os.SystemClock;
import com.yourdesk.androidcore.core.Viewer;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;
import java.util.function.Consumer;
import org.json.JSONArray;
import org.json.JSONObject;

/** 協商與播放分別在背景執行；慢速網路指令不阻塞解碼、操作或即時靜音。 */
public final class RemoteAudioSession implements AutoCloseable {
  private static final class Desired {
    final Viewer viewer; final boolean enabled; final String profile;
    Desired(Viewer viewer, boolean enabled, String profile) { this.viewer = viewer; this.enabled = enabled; this.profile = profile; }
  }
  private static final class Active {
    final Desired desired; final long generation; final String codec; final int wire;
    Active(Desired desired, long generation, String codec) {
      this.desired = desired; this.generation = generation; this.codec = codec;
      wire = codec.equals("opus") ? 3 : codec.equals("pcm") ? 2 : 1;
    }
  }
  private static final class Failure {
    final Active active; final String message;
    Failure(Active active, String message) { this.active = active; this.message = message; }
  }
  public static final class Stats {
    public final long generation, decodedFrames, writtenFrames, playedFrames, nonzeroSamples;
    public final String codec, decoder;
    Stats(Active active, RemoteAudioPlayer player) {
      generation = active == null ? 0 : active.generation;
      codec = active == null ? "" : active.codec;
      decoder = player == null ? "" : player.decoderName();
      decodedFrames = player == null ? 0 : player.framesDecoded();
      writtenFrames = player == null ? 0 : player.framesWritten();
      playedFrames = player == null ? 0 : player.framesPlayed();
      nonzeroSamples = player == null ? 0 : player.nonzeroSamples();
    }
  }
  private final ScheduledExecutorService control = worker("YourDeskAudioControl");
  private final ScheduledExecutorService playback = worker("YourDeskAudioPlayback");
  private final Consumer<String> statusChanged;
  private volatile Desired desired = new Desired(null, false, "standard");
  private volatile Active playable;
  private volatile boolean closed;
  private volatile boolean unavailable;
  private volatile Stats stats = new Stats(null, null);
  private final AtomicReference<Failure> failure = new AtomicReference<>();
  private final AtomicReference<String> lastStatus = new AtomicReference<>("");
  // 以下協商狀態只由 control 執行緒操作。
  private Desired applied;
  private Active active;
  private List<String> candidates = new ArrayList<>();
  private int candidate;
  private boolean exhausted, negotiated;
  private long generation, started, nextLease;
  // 以下播放狀態只由 playback 執行緒操作。
  private Active playing;
  private RemoteAudioPlayer player;
  private boolean playbackFailed;
  private AudioPacketGate gate;
  private long lastPacket, lastStats;

  public RemoteAudioSession(Consumer<String> statusChanged) {
    this.statusChanged = statusChanged;
    control.scheduleWithFixedDelay(this::controlTick, 0, 100, TimeUnit.MILLISECONDS);
    playback.execute(this::playbackTick);
  }
  private static ScheduledExecutorService worker(String name) {
    return Executors.newSingleThreadScheduledExecutor(task -> { Thread thread = new Thread(task, name); thread.setDaemon(true); return thread; });
  }
  public synchronized void setPlayback(Viewer viewer, boolean enabled, String profile) {
    if (closed) return;
    Desired old = desired;
    if (old.viewer == viewer && old.enabled == enabled && old.profile.equals(profile)) return;
    desired = new Desired(viewer, enabled, profile);
    unavailable = false;
    playable = null; // UI 的關閉／失去焦點立即生效，不等遠端回覆。
    publish(enabled ? "正在協商聲音" : "聲音已關閉");
  }
  public Stats stats() { return stats; }
  public String status() { return lastStatus.get(); }
  public boolean unavailable() { return unavailable; }
  private void publish(String message) {
    if (!closed && !message.equals(lastStatus.getAndSet(message))) statusChanged.accept(message);
  }

  private void controlTick() {
    if (closed) return;
    Desired wanted = desired;
    try {
      if (applied != wanted) {
        stopRemote();
        applied = wanted; active = null; candidates.clear(); candidate = 0;
        negotiated = exhausted = false; started = SystemClock.uptimeMillis(); nextLease = 0;
        failure.set(null);
      }
      if (!wanted.enabled || wanted.viewer == null || exhausted) return;
      if (!wanted.viewer.isConnected()) { playable = null; return; }
      if (!wanted.viewer.supportsCommand("audio.configure")) {
        if (SystemClock.uptimeMillis() - started > 5000) {
          exhausted = unavailable = true; publish("對方尚未支援遠端聲音，請更新 Host");
        }
        return;
      }
      if (!negotiated) {
        JSONArray source = wanted.viewer.supportsCommand("audio.capabilities")
            ? new JSONArray(wanted.viewer.audioCapabilitiesJSON()) : null;
        candidates = negotiate(source); negotiated = true;
        if (desired != wanted || closed) return;
      }
      Failure error = failure.getAndSet(null);
      if (error != null && error.active == active) { tryNext(error.message); return; }
      if (active == null) {
        if (candidate >= candidates.size()) {
          exhausted = unavailable = true; publish("兩端沒有共同可用的聲音編碼"); return;
        }
        active = new Active(wanted, ++generation, candidates.get(candidate));
        nextLease = 0;
        publish("等待遠端聲音（" + label(active.codec) + "）");
      }
      if (desired != wanted || closed || SystemClock.uptimeMillis() < nextLease) return;
      nextLease = SystemClock.uptimeMillis() + 1000;
      Active request = active;
      // 音訊可能早於 configure 回覆到達，先公告唯一可接受的世代。
      playable = request;
      JSONObject reply = new JSONObject(wanted.viewer.configureAudio(true, request.codec, wanted.profile, request.generation));
      if (desired != wanted || closed) { playable = null; return; }
      if (reply.optLong("generation") == request.generation && !reply.optString("error").isEmpty()) {
        tryNext(reply.optString("error"));
      }
    } catch (Exception error) {
      if (desired == wanted && !closed) {
        playable = null;
        nextLease = SystemClock.uptimeMillis() + 1000;
        publish("聲音連線暫停：" + message(error));
      }
    }
  }

  private List<String> negotiate(JSONArray source) {
    List<String> options = new ArrayList<>();
    for (String codec : new String[]{"aac", "opus", "aac-software", "pcm"}) {
      int wire = codec.equals("opus") ? 3 : codec.equals("pcm") ? 2 : 1;
      if (!RemoteAudioPlayer.supports(wire)) continue;
      boolean supported = source == null && !codec.equals("aac");
      if (source != null) for (int i = 0; i < source.length(); i++) {
        JSONObject capability = source.optJSONObject(i);
        if (capability != null && capability.optString("codec").equals(wire == 1 ? "aac" : codec))
          supported |= capability.optBoolean("encode") && (!codec.equals("aac") || capability.optBoolean("hardwareEncode"));
      }
      if (supported) options.add(codec);
    }
    return options;
  }
  private void tryNext(String reason) {
    playable = null; stopRemote(); active = null; candidate++; nextLease = 0;
    if (candidate >= candidates.size()) { exhausted = unavailable = true; publish("聲音無法使用：" + reason); }
    else publish("重新協商聲音：" + reason);
  }
  private void stopRemote() {
    Active previous = active;
    if (previous != null) try {
      previous.desired.viewer.configureAudio(false, previous.codec, previous.desired.profile, ++generation);
    } catch (Exception ignored) { /* 對端另有六秒租約；斷線或通知遺失不會持續擷取。 */ }
  }
  private void playbackTick() {
    try {
      Active current = playable;
      if (current != playing) {
        closePlayer(); playing = current; lastPacket = 0; playbackFailed = false;
        gate = current == null ? null : new AudioPacketGate(current.generation, current.wire);
      }
      if (closed || current == null || current.desired != desired || playbackFailed) return;
      for (int i = 0; i < 8 && playable == current && current.desired == desired; i++) {
        byte[] wire = current.desired.viewer.readAudioPacket();
        if (wire == null || wire.length == 0) break;
        AudioPacket packet = AudioPacket.parse(wire);
        int accepted = gate.accept(packet);
        if (accepted == 0) continue;
        if (accepted == 2) closePlayer();
        lastPacket = SystemClock.uptimeMillis();
        if (player == null) player = new RemoteAudioPlayer(current.wire);
        player.queue(packet.data);
      }
      if (player != null) {
        player.drain();
        long now = SystemClock.uptimeMillis();
        if (now - lastPacket > 500) {
          closePlayer(); publish("等待遠端聲音（" + label(current.codec) + "）");
        } else if (now - lastStats > 100) {
          lastStats = now; stats = new Stats(current, player);
          if (stats.playedFrames > 0) publish("聲音播放中（" + label(current.codec) + "）");
        }
      }
    } catch (Exception error) {
      closePlayer(); playbackFailed = true;
      if (playing != null) failure.set(new Failure(playing, message(error)));
    } finally {
      if (closed) closePlayer();
      else try { playback.schedule(this::playbackTick, playable == null ? 50 : 8, TimeUnit.MILLISECONDS); }
      catch (java.util.concurrent.RejectedExecutionException stopping) { closePlayer(); }
    }
  }
  private void closePlayer() {
    if (player != null) { player.close(); player = null; }
    stats = new Stats(null, null);
  }
  private static String label(String codec) { return codec.startsWith("aac") ? "AAC" : codec.equals("opus") ? "Opus" : "PCM"; }
  private static String message(Exception error) { return error.getMessage() == null ? error.getClass().getSimpleName() : error.getMessage(); }
  @Override public synchronized void close() {
    if (closed) return;
    closed = true; playable = null;
    control.execute(this::stopRemote);
    control.shutdown();
    playback.execute(this::closePlayer);
    playback.shutdown();
  }
}
