// 執行實際自動配置控制流程；模擬 DOM/API，不開啟遠端連線或寫入偏好。
const fs = require('fs'), vm = require('vm'), assert = require('assert');
const source = fs.readFileSync(require('path').join(__dirname, '../internal/clientui/web/app.js'), 'utf8');
const start = source.indexOf('let streamAutoRun = null;'), end = source.indexOf('let lastHostConflict', start);
assert(start >= 0 && end > start);
function fixture() {
 const elements = new Map(), calls = [];
 function element() { return { dataset:{}, hidden:false, disabled:false, children:[], handlers:{}, addEventListener(k,f){this.handlers[k]=f}, setAttribute(){},removeAttribute(){},replaceChildren(){},append(){},focus(){},close(){this.open=false;this.handlers.close?.()},showModal(){this.open=true}}; }
 const $ = id => {if(!elements.has(id))elements.set(id,element());return elements.get(id)};
 $('#stream-auto-steps').children = Array.from({length:4},element);
 $('#stream-auto-steps').lastElementChild = $('#stream-auto-steps').children[3];
 let polls=0;
 const result={id:'smoke',startedAt:123,pending:false,measuredFPS:29.8,activeSeconds:10,preferences:{sourceFPSLimit:30,keyframeInterval:10,bitrateLimitMbps:12}};
 const context=vm.createContext({$,busy:false,state:{running:{},preferences:{sourceFPSLimit:10}},i18n:{t:s=>s},performance:{now:()=>0},setTimeout:f=>queueMicrotask(f),requestAnimationFrame:f=>queueMicrotask(f),openDialog:id=>$(id).showModal(),action:f=>f(),text:()=>element(),applyPreferences:()=>{},api:async(route,method,body)=>{calls.push(body);if(body.cancel)return {};if(body.apply)return result;return ++polls===1?{id:'smoke',startedAt:123,pending:true}:result;}});
 vm.runInContext(source.slice(start,end),context);
 return {$,calls,context,run:()=>vm.runInContext("autoConfigureStream('latency')",context)};
}
const flush=async()=>{for(let n=0;n<20;n++)await Promise.resolve()};
(async()=>{
 for(const apply of [false,true]){
  const f=fixture(),done=f.run();await flush();assert.equal(f.calls.length,0,'opening must not start detection');
  assert.equal(f.$('#stream-auto-steps').children[0].dataset.state,'waiting');
  f.$('#stream-auto-start').handlers.click();await flush();
  assert.equal(f.$('#stream-auto-apply').hidden,false,'preview must offer Apply');
  assert.equal(f.calls.some(c=>c.apply),false,'preview must not apply');
  if(apply)f.$('#stream-auto-apply').handlers.click();else f.$('#stream-auto-cancel').handlers.click();
  await done;
  assert.equal(f.calls.filter(c=>c.apply).length,apply?1:0);
  assert(f.calls.some(c=>c.cancel),'must release probe');
  assert.equal(f.context.state.preferences.sourceFPSLimit,apply?30:10);
 }
 const f=fixture(),done=f.run();f.$('#stream-auto-close').handlers.click();await done;assert.equal(f.calls.length,0,'cancel before Start must not call API');
 console.log('PASS 開啟不偵測、開始後偵測、預覽不套用、手動套用、取消與復原');
})().catch(e=>{console.error(e);process.exitCode=1});
