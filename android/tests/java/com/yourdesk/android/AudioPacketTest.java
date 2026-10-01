package com.yourdesk.android;

import com.yourdesk.android.audio.AudioPacket;
import com.yourdesk.android.audio.AudioPacketGate;
import java.nio.ByteBuffer;

public final class AudioPacketTest {
  private static void check(boolean value, String message) { if (!value) throw new AssertionError(message); }
  private static byte[] wire(int codec, long generation, long sequence, int size) {
    ByteBuffer data = ByteBuffer.allocate(24 + size);
    data.put(new byte[]{'Y','D','A','1',(byte)codec,0,0,0}).putLong(generation).putLong(sequence);
    return data.array();
  }
  public static void main(String[] args) {
    byte[] valid = wire(3, 0x0102030405060708L, 123, 1275);
    AudioPacket packet = AudioPacket.parse(valid);
    check(packet != null && packet.generation == 0x0102030405060708L && packet.sequence == 123 && packet.codec == 3, "Host big-endian 世代／序號格式");
    valid[24] = 100; check(packet.data[0] == 0, "音訊不引用可變封包記憶體");
    check(AudioPacket.parse(wire(2, 1, 1, 4096)) != null, "PCM 1024 個雙聲道取樣框");
    check(AudioPacket.parse(wire(1, 1, 1, 8192)) != null, "AAC 封包上限");
    for (byte[] bad : new byte[][]{new byte[24], wire(0,1,1,30),wire(3,0,1,30),wire(3,1,0,30),wire(3,-1,1,30),wire(3,1,1,1276),wire(2,1,1,4095),wire(1,1,1,8193)})
      check(AudioPacket.parse(bad) == null, "拒絕錯誤格式與過大封包");
    byte[] reserved=wire(3,1,1,30); reserved[5]=1;check(AudioPacket.parse(reserved)==null,"保留位元組不得隱含新格式");
    check(AudioPacket.parse(null)==null,"空白讀取");
    AudioPacketGate gate = new AudioPacketGate(4,3);
    check(gate.accept(AudioPacket.parse(wire(3,3,999,30)))==0,"舊連線封包不可污染新連線");
    check(gate.accept(AudioPacket.parse(wire(1,4,999,30)))==0,"錯誤編碼不得改變排序狀態");
    check(gate.accept(AudioPacket.parse(wire(3,4,2,30)))==1,"第一包可從任意正序號開始");
    check(gate.accept(AudioPacket.parse(wire(3,4,2,30)))==0,"重複封包不重播");
    check(gate.accept(AudioPacket.parse(wire(3,4,1,30)))==0,"倒序封包不重播");
    check(gate.accept(AudioPacket.parse(wire(3,4,4,30)))==2,"缺包會要求清除解碼／播放歷史");
    check(gate.accept(AudioPacket.parse(wire(3,4,5,30)))==1,"缺包後恢復連續播放");
    System.out.println("AudioPacket：協定格式、世代／序號、邊界與記憶體隔離 PASS");
  }
}
