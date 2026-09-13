'use strict';
let packetSite=null, packetEpoch=0;
const packetDialog=$('#packet-test-dialog');
packetDialog.addEventListener('close',()=>{packetEpoch++;});
function openPacketTest(site){
 packetEpoch++;packetSite=site;
 $('#packet-test-site').textContent=`${site.name} · ${site.room}`;
 $('#packet-test-report').textContent='';
 $('#packet-test-status').textContent='按下開始測試後，將自動建立背景連線。';
 $('#packet-test-secret-row').hidden=true;$('#packet-test-secret').value='';
 $('#packet-test-start').disabled=false;
 openDialog('#packet-test-dialog');
}
$('#packet-test-start').addEventListener('click',async()=>{
 const epoch=++packetEpoch,site=packetSite,start=performance.now();
 const reachedLayers=new Set(),lastTX=new Map(),lastRX=new Map();
 let verified=0,attempts=0,sent=0,received=0,rtt=0,token=null,testStart=0,failureSince=0;
 $('#packet-test-start').disabled=true;
 $('#packet-test-status').textContent='正在建立背景連線…';
 $('#packet-test-report').textContent='';
 try{
  const opened=await api('packet-test','POST',{id:site.id,action:'start',secret:$('#packet-test-secret').value});
  $('#packet-test-secret').value='';
  if(opened.needsSecret){if(epoch===packetEpoch){$('#packet-test-secret-row').hidden=false;$('#packet-test-status').textContent='輸入遠端密碼後即可自動連線測試。';}return;}
  token=opened.token;
  if(epoch===packetEpoch)$('#packet-test-secret-row').hidden=true;
  while(epoch===packetEpoch){
   const status=await api('packet-test','POST',{id:site.id,action:'status',token});
   if(status.stage==='connected')break;
   if(performance.now()-start>30000)throw new Error('背景連線逾時，請確認遠端在線且密碼正確。');
   await new Promise(resolve=>setTimeout(resolve,300));
  }
  testStart=performance.now();
  while(epoch===packetEpoch&&(performance.now()-testStart<60000||failureSince)){
   $('#packet-test-status').textContent='雙向傳輸中…';
   const r=await api('packet-test','POST',{id:site.id,action:'batch',token});
   if(epoch!==packetEpoch)return;
   verified+=r.verified;attempts+=r.attempts;sent+=r.sent;received+=r.received;rtt+=r.rttMillis*r.verified;
   for(const l of r.layers||[]){
    if(l.sent>0||l.received>0||l.state==='connected')reachedLayers.add(l.id);
    if(l.sent>0)lastTX.set(l.id,performance.now()-l.txIdleMs);
    if(l.received>0)lastRX.set(l.id,performance.now()-l.rxIdleMs);
   }
   const ago=(map,id)=>map.has(id)?`${((performance.now()-map.get(id))/1000).toFixed(1)} 秒前有進展`:'尚未觀察到進展';
   $('#packet-test-report').textContent=`已測試 ${((performance.now()-testStart)/1000).toFixed(1)} 秒\n往返驗證：${verified} / ${attempts}\n遠端確認收到：${(sent/1048576).toFixed(2)} MiB\n本機驗證收到：${(received/1048576).toFixed(2)} MiB\n平均往返：${verified?(rtt/verified).toFixed(2):'—'} ms\n\n${(r.layers||[]).filter(l=>reachedLayers.has(l.id)).map(l=>l.observed?`${l.id} ${l.name}\n  送 +${l.sent} ${l.unit}｜${ago(lastTX,l.id)}\n  收 +${l.received} ${l.unit}｜${ago(lastRX,l.id)}`: `${l.id} ${l.name}：${l.state}（僅確認狀態）`).join('\n')}`;
   if(r.error){
    if(!r.error.includes('deadline')&&!r.error.includes('context'))throw new Error(r.error);
    if(!failureSince)failureSince=performance.now();
    $('#packet-test-status').textContent='回覆逾時，暫停送出 1 秒後重試，保留 30 秒恢復時間…';
    if(performance.now()-failureSince>=30000)throw new Error(r.error);
    await new Promise(resolve=>setTimeout(resolve,1000));
   }else failureSince=0;
  }
  $('#packet-test-status').textContent='測試完成';
 }catch(e){if(epoch===packetEpoch)$('#packet-test-status').textContent=`測試中止：${e.message}`;}
 finally{if(token)await api('packet-test','POST',{id:site.id,action:'stop',token}).catch(()=>{});if(epoch===packetEpoch)$('#packet-test-start').disabled=false;}
});

let networkDebugEnabled=false;
api('network-debug').then(r=>{networkDebugEnabled=r.enabled;$('#network-debug').checked=r.enabled;if(state)renderSites();}).catch(()=>{});
$('#network-debug').addEventListener('change',async event=>{
 const input=event.target;input.disabled=true;
 try{const r=await api('network-debug','POST',{enabled:input.checked});networkDebugEnabled=r.enabled;input.checked=r.enabled;if(!r.enabled){packetEpoch++;if(packetDialog.open)packetDialog.close();}renderSites();}
 catch(e){input.checked=networkDebugEnabled;toast(e.message,true);}
 finally{input.disabled=false;}
});
