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
const displaySelect=document.getElementById('display-select'),displayState=document.getElementById('display-state');
let displayOptions='';
window.desktopDisplays=state=>{
  if(!state||!displaySelect)return;
  const known=state.known===true,count=Number.isInteger(state.count)&&state.count>=0&&state.count<=64?state.count:0;
  const signature=known+':'+count;
  if(displayOptions!==signature){
    displayOptions=signature;displaySelect.replaceChildren();
    if(!known||!count){const option=document.createElement('option');option.textContent=known?'沒有可用螢幕':'等待螢幕資訊';displaySelect.append(option);}
    else for(let i=0;i<count;i++){const option=document.createElement('option');option.value=String(i);option.textContent=`螢幕 ${i+1} / ${count}`;displaySelect.append(option);}
  }
  displaySelect.disabled=!known||count<2||state.pending===true;
  if(known&&count)displaySelect.value=String(state.pending?state.requested:state.current);
  displayState.textContent=state.pending?'切換中…':state.waitingFrame?'等待影像…':state.error?'切換未完成':'';
  displayState.title=state.error||'';
};
displaySelect.addEventListener('change',()=>{
  releaseKeys();window.YourDesk.selectDisplay(Number(displaySelect.value));
});
try{window.desktopDisplays(JSON.parse(window.YourDesk.displayStateJSON()));}catch{}
// 滑鼠與觸控僅由實際繪製影像的原生 View 處理，背景 WebView 不轉送。
const keyboardProxy=document.getElementById('keyboard-proxy');
const pressedKeys=new Map();
let rawKeysUsed=false;
// KeyboardEvent.code 表示實體位置；以既有 Windows 掃描碼協定傳送，保留 E0 與左右修飾鍵。
const scanCodes={Escape:1,Minus:12,Equal:13,Backspace:14,Tab:15,BracketLeft:26,BracketRight:27,
  Enter:28,ControlLeft:29,Semicolon:39,Quote:40,Backquote:41,ShiftLeft:42,Backslash:43,
  Comma:51,Period:52,Slash:53,ShiftRight:54,NumpadMultiply:55,AltLeft:56,Space:57,CapsLock:58,
  Pause:69,ScrollLock:70,NumpadSubtract:74,NumpadAdd:78,NumpadDecimal:83,IntlBackslash:86,
  F11:87,F12:88,NumpadEqual:89,NumpadEnter:284,ControlRight:285,NumpadDivide:309,
  PrintScreen:311,AltRight:312,NumLock:325,Home:327,ArrowUp:328,PageUp:329,ArrowLeft:331,
  ArrowRight:333,End:335,ArrowDown:336,PageDown:337,Insert:338,Delete:339,MetaLeft:347,MetaRight:348,ContextMenu:349};
for(const [letters,start] of [['QWERTYUIOP',16],['ASDFGHJKL',30],['ZXCVBNM',44]])
  [...letters].forEach((letter,i)=>{scanCodes['Key'+letter]=start+i;});
[...'1234567890'].forEach((digit,i)=>{scanCodes['Digit'+digit]=2+i;});
[82,79,80,81,75,76,77,71,72,73].forEach((scan,i)=>{scanCodes['Numpad'+i]=scan;});
for(let i=1;i<=10;i++)scanCodes['F'+i]=58+i;
for(let i=13;i<=24;i++)scanCodes['F'+i]=i===24?118:87+i;
function eventScan(e){
  if(scanCodes[e.code]!==undefined)return scanCodes[e.code];
  // 部分 Android 鍵盤沒有 code；僅對不含配置歧義的鍵提供退路。
  let code=e.key;
  if(/^[a-z]$/i.test(code))code='Key'+code.toUpperCase();
  else if(/^[0-9]$/.test(code))code='Digit'+code;
  else if(code===' ')code='Space';
  else if(['Shift','Control','Alt','Meta'].includes(code))code+=e.location===2?'Right':'Left';
  return scanCodes[code];
}
function canSendRaw(){try{return window.YourDesk.supportsRawKeyboard?.()===true;}catch{return false;}}
function rawControl(e,code,down){
  let modifiers=(e.shiftKey?1:0)|(e.ctrlKey?2:0)|(e.altKey?4:0)|(e.metaKey?8:0);
  if(e.getModifierState?.('CapsLock'))modifiers|=16;
  // Android/WebView 未必提供 NumLock 狀態，不宣告「狀態有效」，避免改動遠端鎖定狀態。
  return {type:'raw-key',rawKey:{platform:'windows',code,modifiers,nativeFlags:0,
    disableMapping:true,down,repeat:down&&e.repeat===true}};
}
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
    const shift=/[A-Z]/.test(character)&&![...pressedKeys.values()].some(value=>value.key==='shift'||[42,54].includes(value.rawKey?.code));
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
function releaseKeys(){
  for(const control of pressedKeys.values()){
    if(control.rawKey)send({...control,rawKey:{...control.rawKey,down:false,repeat:false,modifiers:0}});
    else send({...control,down:false});
  }
  pressedKeys.clear();
  if(rawKeysUsed)send({type:'raw-key-reset'});
  rawKeysUsed=false;
}
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
  if(stopped||e.target===displaySelect||isComposition(e))return;
  const printable=[...e.key].length===1;
  const raw=canSendRaw(), scan=eventScan(e);
  // 實體字母、標點和空白要由遠端輸入法組字；螢幕鍵盤仍使用 input/composition。
  if(printable&&!e.ctrlKey&&!e.metaKey&&!e.altKey&&!(raw&&scanCodes[e.code]!==undefined)){
    if(canSendText()){
      if(e.target!==keyboardProxy){sendText(e.key);e.preventDefault();}
      return; // 已聚焦 proxy 的一般字元只由 input 送出，避免 keydown 重送。
    }
    if(!/^[a-zA-Z0-9 ]$/.test(e.key)){sendLegacyText(e.key);e.preventDefault();return;}
    // 相容舊 Host：保留實體 ASCII 的按下／放開與快捷鍵路徑。
  }
  if(raw&&scan!==undefined){
    const control=rawControl(e,scan,true);
    const paste=scan===47&&!e.repeat&&(e.ctrlKey||e.metaKey);
    if(send(paste?{...control,pasteShortcut:true,pasteModifiers:control.rawKey.modifiers}:control)){pressedKeys.set(e.code||e.key,control);rawKeysUsed=true;}
    e.preventDefault();return;
  }
  if(!printable&&!/^(Enter|Escape|Backspace|Tab|Delete|ArrowUp|ArrowDown|ArrowLeft|ArrowRight|Home|End|PageUp|PageDown|Shift|Control|Alt|Meta|F([1-9]|1[0-2]))$/.test(e.key))return;
  const control={type:'key',key:keyName(e.key),down:true};
  if(send(control.key==="v"&&!e.repeat&&(e.ctrlKey||e.metaKey)?{...control,pasteShortcut:true,pasteModifiers:(e.ctrlKey?2:0)|(e.metaKey?8:0)|(e.altKey?4:0)}:control))pressedKeys.set(e.code||e.key,control);e.preventDefault();
});
window.addEventListener('keyup',e=>{
  const id=e.code||e.key,control=pressedKeys.get(id);
  if(control){
    pressedKeys.delete(id);
    send(control.rawKey?rawControl(e,control.rawKey.code,false):{...control,down:false});e.preventDefault();
  }else if(rawKeysUsed&&[29,285,42,54,56,312,347,348].includes(eventScan(e))){
    // 修飾鍵按下可能在 WebView 取得焦點前，仍須傳送它的放開。
    send(rawControl(e,eventScan(e),false));e.preventDefault();
  }
});
window.addEventListener('blur',releaseKeys);
const reportLayout=()=>window.YourDesk.desktopLayout?.(screenImage.getBoundingClientRect().top,window.innerWidth);
new ResizeObserver(reportLayout).observe(document.querySelector('header'));
window.addEventListener('resize',reportLayout);
reportLayout();
