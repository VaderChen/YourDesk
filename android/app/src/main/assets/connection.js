'use strict';
const connectionUI={mode:'shell',waiting:false,timer:null};
const connectionElement=id=>document.getElementById(id);
function stopConnectionWait(){clearInterval(connectionUI.timer);connectionUI.waiting=false;connectionElement('connect-progress').hidden=true;}
function openConnection(room,mode){
 if(connectionUI.waiting)return;
 const normalizedRoom=(room||'').trim();
 connectionUI.mode=mode;
 connectionElement('connect-title').textContent=mode==='desktop'?'遠端桌面連線':'Shell 連線';
 connectionElement('connect-room').value=normalizedRoom;
 connectionElement('connect-secret').value='';
 connectionElement('connect-remember').checked=false;
 connectionElement('connect-status').textContent='';
 try{
  const raw=window.YourDesk?.remembered?.();
  const saved=raw?JSON.parse(raw):null;
  const savedSecret = saved?.sites && typeof saved.sites[normalizedRoom]==='string'
   ? saved.sites[normalizedRoom]
   : (saved?.room?.trim()===normalizedRoom ? saved.secret : null);
  if(typeof savedSecret==='string'){
   connectionElement('connect-secret').value=savedSecret;
   connectionElement('connect-remember').checked=true;
  }
 }catch{}
 connectionElement('connect-overlay').hidden=false;
}
connectionElement('connection-form').onsubmit=e=>{
 e.preventDefault();if(connectionUI.waiting)return;
 const room=connectionElement('connect-room').value.trim();const secret=connectionElement('connect-secret').value;
 if(!room||!secret)return;
 if(!window.YourDesk?.connectSession){connectionElement('connect-status').textContent='無法使用 Android 連線服務';return;}
 let saved=false;
 try{
  const remember=connectionElement('connect-remember').checked;
  saved=window.YourDesk.rememberCredentials(room,remember?secret:'')===true;
 }catch{}
 if(!saved){connectionElement('connect-status').textContent='密碼保存失敗，請重試';return;}
 connectionElement('connect-secret').value='';
 connectionUI.waiting=true;connectionElement('connect-overlay').hidden=true;
 connectionElement('connect-progress').hidden=false;connectionElement('connect-elapsed').textContent='0';
 const start=Date.now();connectionUI.timer=setInterval(()=>{connectionElement('connect-elapsed').textContent=Math.floor((Date.now()-start)/1000);},250);
 try{window.YourDesk.connectSession(JSON.stringify({mode:connectionUI.mode,room,secret}));}catch{window.connectionFailed();}
};
window.connectionFailed=()=>{stopConnectionWait();connectionElement('connect-status').textContent='連線失敗，請確認遠端、密碼及網路後重試。';connectionElement('connect-overlay').hidden=false;};
connectionElement('connect-abort').onclick=()=>{window.YourDesk.abortConnection();stopConnectionWait();connectionElement('connect-status').textContent='已中止連線';connectionElement('connect-overlay').hidden=false;};
connectionElement('connect-cancel').onclick=()=>{connectionElement('connect-secret').value='';connectionElement('connect-overlay').hidden=true;};
window.addEventListener('pagehide',()=>{clearInterval(connectionUI.timer);connectionElement('connect-secret').value='';});
