'use strict';
let stopped=false;
const screenImage=document.getElementById('screen'),desktopStatus=document.getElementById('status'),desktopStatsElement=document.getElementById('stats'),displayBar=document.getElementById('displays');
document.getElementById('back').onclick=()=>{releaseKeys();stopped=true;window.YourDesk.closeTerminal();};
window.addEventListener('pagehide',()=>{releaseKeys();stopped=true;clearTimeout(compositionTimer);});
// 影像由 Android 原生圖層呈現；WebView 同步配置、狀態與控制事件。
window.desktopFrameStatus=(width,height,partial,codec,display=0)=>{
  if(desktopStatus) desktopStatus.textContent=`${codec||'JPEG'}${partial?' · 差分':''} · ${width} × ${height}`;
  if(displayBar) displayBar.style.display='flex';
};
const formatRate=bytes=>{const value=Number.isFinite(bytes)?Math.max(0,bytes):0;return value>=1024*1024?(value/(1024*1024)).toFixed(1)+' MB/s':(value/1024).toFixed(1)+' KB/s';};
window.desktopStats=(fps,tx,rx)=>{
  if(desktopStatsElement) desktopStatsElement.textContent=`FPS ${Math.round(Math.max(0,Number(fps)||0))} · TX ${formatRate(Number(tx)||0)} · RX ${formatRate(Number(rx)||0)}`;
};
const send=c=>{
  if(stopped)return false;
  const result=window.YourDesk.sendControlJSON(JSON.stringify(c));
  if(result!=='ok')desktopStatus.textContent='控制傳送失敗';
  return result==='ok';
};
// 滑鼠與觸控僅由實際繪製影像的原生 View 處理，背景 WebView 不轉送。
const keyboardProxy=document.getElementById('keyboard-proxy');
const pressedKeys=new Map();
let composing=false,compositionTimer;
let textInputSupported=false;
window.desktopInputCapabilities=supported=>{textInputSupported=supported===true;};
function canSendText(){
  try{if(window.YourDesk.supportsTextInput)textInputSupported=window.YourDesk.supportsTextInput()===true;}catch{}
  return textInputSupported;
}
canSendText();
const isComposition=e=>composing||e.isComposing||e.keyCode===229||['Process','Dead','Unidentified'].includes(e.key);
const keyName=key=>key===' '?'space':key.toLowerCase();
function sendLegacyText(value){
  // 舊 Host 只支援固定按鍵表；不把中文或標點冒充成可用的 key。
  if(!/^[a-zA-Z0-9 \r\n\t]*$/.test(value)){
    desktopStatus.textContent='遠端 Host 不支援此文字輸入，請更新 Host';return;
  }
  for(const character of value.replace(/\r\n?/g,'\n')){
    const key=character==='\n'?'enter':character==='\t'?'tab':keyName(character);
    const shift=/[A-Z]/.test(character)&&![...pressedKeys.values()].includes('shift');
    if(shift&&!send({type:'key',key:'shift',down:true}))return;
    const ok=tapKey(key);
    if(shift)send({type:'key',key:'shift',down:false});
    if(!ok)return;
  }
}
// 每個訊息最多 16 KiB UTF-8，且不拆開 Unicode code point。
function sendText(value){
  if(!canSendText()){sendLegacyText(value);return;}
  let part='',bytes=0;
  for(const character of value){
    const code=character.codePointAt(0),length=code<0x80?1:code<0x800?2:code<0x10000?3:4;
    if(bytes+length>16384){if(!send({type:'text',text:part}))return;part='';bytes=0;}
    part+=character;bytes+=length;
  }
  if(part)send({type:'text',text:part});
}
function commitText(){
  clearTimeout(compositionTimer);
  if(composing)return;
  const value=keyboardProxy.value;keyboardProxy.value='';
  if(value)sendText(value);
}
function tapKey(key){return send({type:'key',key,down:true})&&send({type:'key',key,down:false});}
function releaseKeys(){for(const key of pressedKeys.values())send({type:'key',key,down:false});pressedKeys.clear();}
keyboardProxy.addEventListener('compositionstart',()=>{clearTimeout(compositionTimer);composing=true;});
keyboardProxy.addEventListener('compositionend',()=>{
  composing=false;
  // 某些 IME 的最後 input 在 compositionend 之前，其他則在之後。
  // 最後 input 可先清掉 value；延後的 fallback 因而不會重送。
  compositionTimer=setTimeout(commitText,0);
});
keyboardProxy.addEventListener('input',e=>{if(!composing&&!e.isComposing)commitText();});
keyboardProxy.addEventListener('beforeinput',e=>{
  if(composing||e.isComposing)return;
  const key={deleteContentBackward:'backspace',deleteContentForward:'delete',insertParagraph:'enter',insertLineBreak:'enter'}[e.inputType];
  if(key){e.preventDefault();tapKey(key);}
});
window.addEventListener('keydown',e=>{
  if(stopped||isComposition(e))return;
  const printable=[...e.key].length===1;
  if(printable&&!e.ctrlKey&&!e.metaKey&&!e.altKey){
    if(canSendText()){
      if(e.target!==keyboardProxy){sendText(e.key);e.preventDefault();}
      return; // 已聚焦 proxy 的一般字元只由 input 送出，避免 keydown 重送。
    }
    if(!/^[a-zA-Z0-9 ]$/.test(e.key)){sendLegacyText(e.key);e.preventDefault();return;}
    // 相容舊 Host：保留實體 ASCII 的按下／放開與快捷鍵路徑。
  }
  if(!printable&&!/^(Enter|Escape|Backspace|Tab|Delete|ArrowUp|ArrowDown|ArrowLeft|ArrowRight|Home|End|PageUp|PageDown|Shift|Control|Alt|Meta|F([1-9]|1[0-2]))$/.test(e.key))return;
  const key=keyName(e.key);pressedKeys.set(e.code||e.key,key);
  send({type:'key',key,down:true});e.preventDefault();
});
window.addEventListener('keyup',e=>{
  const id=e.code||e.key,key=pressedKeys.get(id);
  if(key){pressedKeys.delete(id);send({type:'key',key,down:false});e.preventDefault();}
});
window.addEventListener('blur',releaseKeys);
const reportLayout=()=>window.YourDesk.desktopLayout?.(screenImage.getBoundingClientRect().top,window.innerWidth);
new ResizeObserver(reportLayout).observe(document.querySelector('header'));
window.addEventListener('resize',reportLayout);
reportLayout();
