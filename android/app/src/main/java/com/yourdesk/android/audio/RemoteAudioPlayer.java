package com.yourdesk.android.audio;

import android.media.AudioAttributes;
import android.media.AudioFormat;
import android.media.AudioTrack;
import android.media.MediaCodec;
import android.media.MediaCodecInfo;
import android.media.MediaCodecList;
import android.media.MediaFormat;
import android.os.Build;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.List;

/** 專用背景執行緒持有解碼器與 AudioTrack，音訊佇列有上限且寫入不阻塞。 */
public final class RemoteAudioPlayer implements AutoCloseable {
  public static final int RATE = 48000, CHANNELS = 2;
  private static final int MAX_PCM_BYTES = RATE * CHANNELS * 2 / 5;
  public static AudioAttributes attributes() {
    return new AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_MEDIA)
        .setContentType(AudioAttributes.CONTENT_TYPE_MOVIE).build();
  }
  private final int wireCodec;
  private MediaCodec decoder;
  private AudioTrack track;
  private final ArrayDeque<ByteBuffer> pending = new ArrayDeque<>();
  private int pendingBytes;
  private long inputSamples;
  private long framesWritten, framesDecoded, nonzeroSamples;
  private int inputsWithoutOutput;
  private String decoderName = "PCM";

  public RemoteAudioPlayer(int codec) throws Exception {
    if (codec < 1 || codec > 3) throw new IllegalArgumentException("不支援的聲音編碼");
    wireCodec = codec;
    if (codec == 2) return;
    Exception failure = null;
    for (String name : decoderNames(codec)) {
      MediaCodec candidate = null;
      try {
        candidate = MediaCodec.createByCodecName(name);
        candidate.configure(format(codec), null, null, 0); candidate.start();
        decoder = candidate; decoderName = name; return;
      } catch (Exception error) { failure = error; release(candidate); }
    }
    throw new IllegalStateException("手機無法建立聲音解碼器", failure);
  }

  public static boolean supports(int codec) { return codec == 2 || !decoderNames(codec).isEmpty(); }

  private static List<String> decoderNames(int codec) {
    List<String> names = new ArrayList<>(), software = new ArrayList<>();
    MediaFormat format = format(codec);
    try {
      for (MediaCodecInfo info : new MediaCodecList(MediaCodecList.REGULAR_CODECS).getCodecInfos()) {
        if (info.isEncoder()) continue;
        try {
          if (!info.getCapabilitiesForType(format.getString(MediaFormat.KEY_MIME)).isFormatSupported(format)) continue;
          boolean hardware = Build.VERSION.SDK_INT >= 29 ? info.isHardwareAccelerated()
              : !(info.getName().startsWith("OMX.google.") || info.getName().startsWith("c2.android."));
          (hardware ? names : software).add(info.getName());
        } catch (RuntimeException ignored) { }
      }
    } catch (RuntimeException ignored) { }
    names.addAll(software); return names;
  }

  private static MediaFormat format(int codec) {
    MediaFormat format = MediaFormat.createAudioFormat(codec == 3 ? "audio/opus" : "audio/mp4a-latm", RATE, CHANNELS);
    format.setInteger(MediaFormat.KEY_MAX_INPUT_SIZE, 8192);
    format.setInteger(MediaFormat.KEY_PCM_ENCODING, AudioFormat.ENCODING_PCM_16BIT);
    if (codec == 3) {
      ByteBuffer header = ByteBuffer.allocate(19).order(ByteOrder.LITTLE_ENDIAN);
      header.put(new byte[]{'O','p','u','s','H','e','a','d'}).put((byte)1).put((byte)2);
      header.putShort((short)0).putInt(RATE).putShort((short)0).put((byte)0).flip();
      format.setByteBuffer("csd-0", header);
      format.setByteBuffer("csd-1", nativeLong(0));
      format.setByteBuffer("csd-2", nativeLong(80000000L));
    } else {
      // Host 傳送 AAC-LC raw access unit，固定 48 kHz／雙聲道，沒有 ADTS 標頭。
      format.setByteBuffer("csd-0", ByteBuffer.wrap(new byte[]{0x11, (byte)0x90}));
      format.setInteger(MediaFormat.KEY_IS_ADTS, 0);
    }
    return format;
  }
  private static ByteBuffer nativeLong(long value) {
    ByteBuffer data = ByteBuffer.allocate(8).order(ByteOrder.nativeOrder()); data.putLong(value).flip(); return data;
  }

  public void queue(byte[] data) throws Exception {
    if (wireCodec == 2) { enqueuePcm(ByteBuffer.wrap(data)); pump(); return; }
    drain();
    int index = decoder.dequeueInputBuffer(1000);
    if (index < 0) throw new IllegalStateException("聲音解碼器未釋放輸入緩衝區");
    ByteBuffer input = decoder.getInputBuffer(index);
    if (input == null || data.length > input.capacity()) throw new IllegalStateException("聲音解碼輸入超過上限");
    input.clear(); input.put(data);
    decoder.queueInputBuffer(index, 0, data.length, inputSamples * 1000000L / RATE, 0);
    inputSamples += wireCodec == 3 ? 960 : 1024;
    if (++inputsWithoutOutput > 30) throw new IllegalStateException("聲音解碼器沒有產生輸出");
    drain();
  }

  public void drain() throws Exception {
    if (decoder != null) {
      MediaCodec.BufferInfo info = new MediaCodec.BufferInfo();
      for (int i = 0; i < 16; i++) {
        int index = decoder.dequeueOutputBuffer(info, 0);
        if (index == MediaCodec.INFO_TRY_AGAIN_LATER) break;
        if (index == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED) {
          MediaFormat output = decoder.getOutputFormat();
          if (output.getInteger(MediaFormat.KEY_SAMPLE_RATE) != RATE || output.getInteger(MediaFormat.KEY_CHANNEL_COUNT) != CHANNELS
              || (output.containsKey(MediaFormat.KEY_PCM_ENCODING) && output.getInteger(MediaFormat.KEY_PCM_ENCODING) != AudioFormat.ENCODING_PCM_16BIT))
            throw new IllegalStateException("聲音解碼輸出格式不相容");
        } else if (index >= 0) {
          try {
            if (info.size > 0 && (info.flags & MediaCodec.BUFFER_FLAG_CODEC_CONFIG) == 0) {
              ByteBuffer output = decoder.getOutputBuffer(index);
              if (output == null || info.offset < 0 || info.size > MAX_PCM_BYTES || info.offset > output.capacity() - info.size)
                throw new IllegalStateException("聲音解碼輸出超過上限");
              output.position(info.offset); output.limit(info.offset + info.size); enqueuePcm(output);
              inputsWithoutOutput = 0;
            }
          } finally { decoder.releaseOutputBuffer(index, false); }
        }
      }
    }
    pump();
  }

  private void enqueuePcm(ByteBuffer pcm) {
    if (pcm.remaining() == 0 || pcm.remaining() % 4 != 0 || pcm.remaining() > MAX_PCM_BYTES)
      throw new IllegalArgumentException("PCM 音訊大小無效");
    byte[] data = new byte[pcm.remaining()]; pcm.get(data);
    framesDecoded += data.length / 4;
    for (int i = 0; i < data.length; i += 2) if (data[i] != 0 || data[i + 1] != 0) nonzeroSamples++;
    if (pendingBytes + data.length > MAX_PCM_BYTES) {
      pending.clear(); pendingBytes = 0;
      if (track != null) { track.pause(); track.flush(); track.play(); }
    }
    pending.add(ByteBuffer.wrap(data)); pendingBytes += data.length;
  }

  private void pump() {
    if (pending.isEmpty()) return;
    if (track == null) {
      int minimum = AudioTrack.getMinBufferSize(RATE, AudioFormat.CHANNEL_OUT_STEREO, AudioFormat.ENCODING_PCM_16BIT);
      if (minimum <= 0) throw new IllegalStateException("手機無法播放雙聲道聲音");
      track = new AudioTrack.Builder().setAudioAttributes(attributes())
          .setAudioFormat(new AudioFormat.Builder().setSampleRate(RATE).setChannelMask(AudioFormat.CHANNEL_OUT_STEREO)
              .setEncoding(AudioFormat.ENCODING_PCM_16BIT).build())
          .setTransferMode(AudioTrack.MODE_STREAM).setBufferSizeInBytes(Math.max(minimum, RATE * 4 / 12))
          .setPerformanceMode(AudioTrack.PERFORMANCE_MODE_LOW_LATENCY).build();
      if (track.getState() != AudioTrack.STATE_INITIALIZED) throw new IllegalStateException("聲音播放裝置建立失敗");
      track.play();
    }
    for (int i = 0; i < 8 && !pending.isEmpty(); i++) {
      ByteBuffer data = pending.peek();
      int written = track.write(data, data.remaining(), AudioTrack.WRITE_NON_BLOCKING);
      if (written < 0) throw new IllegalStateException("聲音播放裝置已失效（" + written + "）");
      pendingBytes -= written; framesWritten += written / 4;
      if (!data.hasRemaining()) pending.remove();
      if (written == 0) break;
    }
  }
  public long framesDecoded() { return framesDecoded; }
  public long framesWritten() { return framesWritten; }
  public long framesPlayed() { return track == null ? 0 : Integer.toUnsignedLong(track.getPlaybackHeadPosition()); }
  public long nonzeroSamples() { return nonzeroSamples; }
  public String decoderName() { return decoderName; }
  @Override public void close() {
    release(decoder); decoder = null;
    if (track != null) {
      try { track.pause(); track.flush(); } catch (RuntimeException ignored) { }
      try { track.release(); } catch (RuntimeException ignored) { }
      track = null;
    }
    pending.clear(); pendingBytes = 0;
  }
  private static void release(MediaCodec codec) {
    if (codec != null) {
      try { codec.stop(); } catch (RuntimeException ignored) { }
      try { codec.release(); } catch (RuntimeException ignored) { }
    }
  }
}
