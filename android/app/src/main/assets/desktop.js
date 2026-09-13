'use strict';
let stopped=false;
const screenImage=document.getElementById('screen'),desktopStatus=document.getElementById('status'),desktopStatsElement=document.getElementById('stats'),displayBar=document.getElementById('displays');
document.getElementById('back').onclick=()=>{stopped=true;window.YourDesk.closeTerminal();};
window.addEventListener('pagehide',()=>{stopped=true;});
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
  if(stopped)return;
  const result=window.YourDesk.sendControlJSON(JSON.stringify(c));
  if(result!=='ok')desktopStatus.textContent='控制傳送失敗';
};
const point=e=>{
  const r=screenImage.getBoundingClientRect();
  return {x:Math.max(0,Math.min(1,(e.clientX-r.left)/Math.max(1,r.width))),y:Math.max(0,Math.min(1,(e.clientY-r.top)/Math.max(1,r.height)))};
};
let activePointer=null;
screenImage.addEventListener('pointerdown',e=>{
  if(activePointer)return;
  activePointer={id:e.pointerId,button:e.button+1};
  screenImage.setPointerCapture(e.pointerId);
  send({type:'button',button:activePointer.button,down:true,...point(e)});
  e.preventDefault();
});
screenImage.addEventListener('pointermove',e=>{
  if(activePointer && activePointer.id!==e.pointerId)return;
  send({type:'move',...point(e)});
});
const release=e=>{
  if(!activePointer || activePointer.id!==e.pointerId)return;
  send({type:'button',button:activePointer.button,down:false,...point(e)});
  activePointer=null;
};
for(const type of ['pointerup','pointercancel','lostpointercapture'])screenImage.addEventListener(type,release);
screenImage.addEventListener('contextmenu',e=>e.preventDefault());
screenImage.addEventListener('wheel',e=>{e.preventDefault();send({type:'wheel',delta:e.deltaY});},{passive:false});
window.addEventListener('keydown',e=>{send({type:'key',key:e.key,down:true});e.preventDefault();});
window.addEventListener('keyup',e=>send({type:'key',key:e.key,down:false}));
const reportLayout=()=>window.YourDesk.desktopLayout?.(screenImage.getBoundingClientRect().top,window.innerWidth);
new ResizeObserver(reportLayout).observe(document.querySelector('header'));
window.addEventListener('resize',reportLayout);
reportLayout();
