'use strict';
const term=new Terminal({disableStdin:true,cursorBlink:true,scrollback:5000,fontSize:14,theme:{background:'#142019',foreground:'#dcebe1'}});
const fit=new FitAddon.FitAddon();term.loadAddon(fit);term.open(document.getElementById('terminal')); term.textarea?.setAttribute('inputmode','text'); term.textarea?.setAttribute('autocomplete','off'); document.getElementById('terminal').addEventListener('pointerdown',()=>{term.focus();if(inputProxy){inputProxy.focus();inputProxy.click();}window.YourDesk?.showKeyboard?.();});
const status=document.getElementById('status');
const inputProxy=document.getElementById('input-proxy'); inputProxy?.addEventListener('input',()=>{const v=inputProxy.value;if(v&&active)send(new TextEncoder().encode(v));inputProxy.value='';});
const adjustFont=delta=>{term.options.fontSize=Math.max(8,Math.min(32,(term.options.fontSize||14)+delta));fit.fit();if(active)call('resize',size()).catch(fail);};
document.getElementById('font-down').onclick=()=>adjustFont(-1);
document.getElementById('font-up').onclick=()=>adjustFont(1);document.getElementById('keyboard').onclick=()=>{if(inputProxy){inputProxy.focus();inputProxy.click();}window.YourDesk?.showKeyboard?.();};
let attempt=0,timer;let next=0,ack=0,active=false,closed=false;const pending=new Map();
function call(method,args={}){return new Promise((resolve,reject)=>{const id=++next;pending.set(id,{resolve,reject});window.YourDesk.request(JSON.stringify({id,method,...args}));});}
window.shellReply=out=>{const p=pending.get(out.id);if(!p)return;pending.delete(out.id);out.error?p.reject(new Error(out.error)):p.resolve(out.result);};
const size=()=>({columns:Math.max(20,Math.min(500,term.cols)),rows:Math.max(5,Math.min(300,term.rows))});
function fail(e){active=false;closed=true;status.textContent=e.message;window.YourDesk.closeTerminal();}
document.getElementById('back').onclick=()=>{closed=true;active=false;window.YourDesk.closeTerminal();};
async function startShell(){
 try{
  active=true;term.options.disableStdin=false;status.textContent='Shell 已連線';fit.fit();await call('resize',size());
  inputProxy.focus();window.YourDesk.showKeyboard();
  while(active&&!closed){
   const out=await call('read',{ack});if(closed)break;
   if(out.data&&out.sequence>ack){await new Promise(r=>term.write(Uint8Array.from(atob(out.data),c=>c.charCodeAt(0)),r));ack=out.sequence;}
   if(out.ended){fail(new Error('Shell 已斷線'));break;}
   await new Promise(r=>setTimeout(r,out.data?0:80));
  }
 }catch(e){if(!closed)fail(e);}
}
let writes=Promise.resolve();
function send(bytes){writes=writes.then(async()=>{for(let i=0;i<bytes.length;i+=2048){if(!active)return;await call('write',{data:btoa(String.fromCharCode(...bytes.slice(i,i+2048)))});}}).catch(fail);}
term.onData(data=>{if(active)send(new TextEncoder().encode(data));});term.onBinary(data=>{if(active)send(Uint8Array.from(data,c=>c.charCodeAt(0)&255));});
const resizeViewport=()=>{const h=window.visualViewport?.height||window.innerHeight;document.body.style.height=h+'px';fit.fit();if(active)call('resize',size()).catch(fail);};
window.visualViewport?.addEventListener('resize',resizeViewport);
let resizeTimer;window.addEventListener('resize',()=>{clearTimeout(resizeTimer);resizeTimer=setTimeout(()=>{fit.fit();if(active)call('resize',size()).catch(fail);},150);});requestAnimationFrame(()=>fit.fit());

requestAnimationFrame(startShell);
