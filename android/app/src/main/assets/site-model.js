'use strict';
// UI、QR 與儲存統一使用相同的站台身分與輸入規則；Native 仍會再次驗證。
window.SiteModel=(()=>{
 const defaultSignal='wss://desktop.mars-cloud.com:8080/ws';
 function signal(value=''){
  const raw=String(value);
  if(/[\u0000-\u001f\u007f]/u.test(raw))throw Error('信令伺服器位址包含無效字元');
  const text=raw.trim();if(!text)return '';
  let url;try{url=new URL(text);}catch{throw Error('請輸入有效的 WSS 或 HTTPS 信令伺服器位址');}
  if(text.length>2048||!['wss:','https:'].includes(url.protocol)||!url.hostname||url.username||url.password||url.hash||url.port==='0')throw Error('信令伺服器必須使用 WSS 或 HTTPS，且不可包含帳號或片段');
  return url.protocol+text.slice(text.indexOf(':')+1);
 }
 function site(value){
  const id=String(value.id||'').trim(),name=String(value.name||id).trim(),note=String(value.note||'').trim();
  if(!id||id.length>255||/[\s\u0000-\u001f\u007f]/u.test(id))throw Error('請輸入有效的遠端 ID 或 IP:Port');
  if(!name||name.length>120||note.length>1024)throw Error('站台名稱或備註過長');
  return {id,name,note,signal:signal(value.signal||''),online:false,terminal:value.terminal!==false,desktop:value.desktop!==false};
 }
 const identity=value=>(String(value.signal||'').trim().replace(/^[^:]+:/,v=>v.toLowerCase())||defaultSignal)+'\n'+String(value.id||'').trim();
 function qr(value){
  let text=String(value||'').trim();
  if(/^yourdesk%3a/i.test(text)){try{text=decodeURIComponent(text);}catch{throw Error('QR Code 編碼無效');}}
  if(text.length>4096||/[\u0000-\u0020\u007f]/u.test(text))throw Error('QR Code 格式無效');
  let url;try{url=new URL(text);}catch{throw Error('QR Code 格式無效');}
  if(url.protocol!=='yourdesk:'||url.hostname!=='site'||url.port||url.username||url.password||url.hash||!['','/'].includes(url.pathname)||url.searchParams.get('v')!=='1')throw Error('僅支援 YourDesk v1 站台 QR Code');
  const allowed=new Set(['v','name','room','signal']);
  for(const key of url.searchParams.keys())if(!allowed.has(key)||url.searchParams.getAll(key).length!==1)throw Error('QR Code 欄位無效');
  if(!url.searchParams.get('room')||!url.searchParams.get('signal'))throw Error('QR Code 缺少遠端 ID 或信令伺服器');
  const result=site({id:url.searchParams.get('room'),name:url.searchParams.get('name'),signal:url.searchParams.get('signal')});
  if(new URL(result.signal).search)throw Error('QR Code 的信令位址不可包含查詢參數');
  return result;
 }
 return {signal,site,identity,qr,defaultSignal};
})();
