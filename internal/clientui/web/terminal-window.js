'use strict';
(async()=>{
 const $=s=>document.querySelector(s), settings=new URLSearchParams(location.hash.slice(1));
 const token=settings.get('token'),key=settings.get('session'),instance=settings.get('instance');
 history.replaceState(null,'',location.pathname);
 await i18n.load();i18n.apply(settings.get('language')||'auto');
 document.title=`${settings.get('name')||'YourDesk'} · ${i18n.t('命令列')}`;
 $('#terminal-title').textContent=document.title;
 const term=new Terminal({cursorBlink:true,scrollback:5000,fontSize:14,fontFamily:'Menlo, Consolas, monospace',theme:{background:'#142019',foreground:'#e0ece3'}});
 const fit=new FitAddon.FitAddon();term.loadAddon(fit);term.open($('#terminal-screen'));
 const state={closed:false,opened:false,failed:false,ack:0,sequence:0,queue:[],queued:0,sending:false,received:false};
 const status=text=>$('#terminal-status').textContent=i18n.t(text);
 const api=async(path,body)=>{
  const response=await fetch(`/api/${path}`,{method:'POST',headers:{'X-YourDesk-Token':token,'Content-Type':'application/json'},body:JSON.stringify(body),signal:AbortSignal.timeout(10000)});
  const out=await response.json();if(!response.ok)throw new Error(out.error||'操作未完成，請稍後重試。');return out;
 };
 const call=(action,params={})=>api('terminal',{session:key,instance,action,params});
 async function closeAfterDisconnect(){
  try{
   const response=await fetch('/api/terminal/preferences',{headers:{'X-YourDesk-Token':token},signal:AbortSignal.timeout(3000)});
   if(response.ok&&(await response.json()).closeWindowOnDisconnect)await close();
  }catch{}
 }
 const fail=error=>{if(state.closed)return;state.failed=true;term.options.disableStdin=true;status(error.message);};
 async function send(){
  if(state.sending||!state.opened||state.closed||state.failed)return;
  state.sending=true;
  try{while(state.queue.length&&!state.closed&&!state.failed){const data=state.queue.shift();state.queued-=data.length;await call('write',{data,sequence:++state.sequence});}}
  catch(e){fail(e)}finally{state.sending=false}
 }
 const enqueue=bytes=>{
  if(state.closed||state.failed)return;
  if(state.queued+bytes.length*2>65536){fail(new Error('輸入過多，請重新連線。'));return;}
  for(let i=0;i<bytes.length;i+=1024){const data=btoa(String.fromCharCode(...bytes.subarray(i,i+1024)));state.queue.push(data);state.queued+=data.length;}
  void send();
 };
 // 在第一批輸出前安裝回應通道，讓終端機查詢（例如游標位置）也能往返。
 const input=term.onData(value=>enqueue(new TextEncoder().encode(value)));
 const binary=term.onBinary(value=>enqueue(Uint8Array.from(value,c=>c.charCodeAt(0)&255)));
 let resizeTimer,waiting;
 const size=()=>({columns:Math.max(20,Math.min(500,term.cols)),rows:Math.max(5,Math.min(300,term.rows))});
 const resize=new ResizeObserver(()=>{clearTimeout(resizeTimer);resizeTimer=setTimeout(()=>{if(state.closed)return;fit.fit();if(state.opened&&!state.failed)call('resize',size()).catch(fail);},100)});
 resize.observe($('#terminal-screen'));
 window.addEventListener('focus',()=>term.focus());
 $('#terminal-screen').addEventListener('pointerdown',()=>term.focus());
 async function close(){
  if(state.closed)return;state.closed=true;clearTimeout(waiting);clearTimeout(resizeTimer);resize.disconnect();input.dispose();binary.dispose();
  try{await call('close')}catch{}finally{await call('disconnect').catch(()=>{});}
  term.dispose();if(typeof window.yourdeskCloseTerminal==='function')await window.yourdeskCloseTerminal();
 }
 $('#terminal-close').addEventListener('click',close);
 window.addEventListener('keydown',e=>{if((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==='w'){e.preventDefault();void close();}});
 try{
  await document.fonts.ready;await new Promise(requestAnimationFrame);fit.fit();term.focus();
  await call('open',size());state.opened=true;void send();term.focus();
  status('等待遠端命令列輸出…');
  waiting=setTimeout(()=>{if(!state.received&&!state.closed)status('尚未收到命令列輸出，請確認遠端 Shell 狀態。');},10000);
  while(!state.closed&&!state.failed){
   const out=await call('read',{ack:state.ack});if(state.closed)break;
   if(out.data&&out.sequence>state.ack){
    const bytes=Uint8Array.from(atob(out.data),c=>c.charCodeAt(0));
    await new Promise(resolve=>term.write(bytes,resolve));state.ack=out.sequence;
    if(!state.received){for(let i=0;i<term.buffer.active.length;i++){if(term.buffer.active.getLine(i)?.translateToString(true).trim()){state.received=true;clearTimeout(waiting);status('已連線；關閉視窗會結束命令列。');break;}}}
   }
   if(out.ended){term.options.disableStdin=true;clearTimeout(waiting);status(out.error||'命令列已結束。');await closeAfterDisconnect();break;}
   await new Promise(resolve=>setTimeout(resolve,out.data?0:40));
  }
 }catch(e){clearTimeout(waiting);fail(e);}
})().catch(error=>{document.querySelector('#terminal-status').textContent=error.message;});
