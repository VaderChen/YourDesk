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
// 滑鼠與觸控僅由實際繪製影像的原生 View 處理，背景 WebView 不轉送。
window.addEventListener('keydown',e=>{send({type:'key',key:e.key,down:true});e.preventDefault();});
window.addEventListener('keyup',e=>send({type:'key',key:e.key,down:false}));
const reportLayout=()=>window.YourDesk.desktopLayout?.(screenImage.getBoundingClientRect().top,window.innerWidth);
new ResizeObserver(reportLayout).observe(document.querySelector('header'));
window.addEventListener('resize',reportLayout);
reportLayout();
