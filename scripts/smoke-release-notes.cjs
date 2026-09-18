// 執行正式 renderRelease 與事件處理，確認完成通知不觸發下載或取消下一版更新。
const fs=require('fs'),vm=require('vm'),assert=require('assert'),path=require('path');
const source=fs.readFileSync(path.join(__dirname,'../internal/clientui/web/app.js'),'utf8');
const start=source.indexOf("let releaseShown = '';");
const end=source.indexOf('// 下載期間只輪詢',start);
const dict=JSON.parse(fs.readFileSync(path.join(__dirname,'../internal/clientui/web/translations.json'),'utf8'));
function fixture(updates,lang){
 const elements=new Map(),calls=[];
 const $=id=>{if(!elements.has(id))elements.set(id,{hidden:false,open:false,handlers:{},classList:{toggle(){}},setAttribute(){},replaceChildren(){},addEventListener(k,f){this.handlers[k]=f},close(){this.open=false}});return elements.get(id)};
 const context=vm.createContext({$,state:{updates},i18n:{preference:lang,t:s=>dict[s]?.[{en:0,ja:1,ko:2}[lang]]||s},document:{documentElement:{lang},querySelector:()=>null},window:{addEventListener(){}},openDialog:id=>$(id).open=true,text:(tag,s)=>s,action:f=>f(),api:async route=>{calls.push(route);return {}}});
 vm.runInContext(source.slice(start,end),context);
 return {$,calls,context,render:()=>vm.runInContext('renderRelease(state.updates)',context)};
}
(async()=>{
 for(const lang of ['zh-Hant','en','ja','ko']){
  for(const button of ['#release-download','#release-close']){
   const f=fixture({showNotes:true,version:'next',notesVersion:'installed',notes:{en:['Fixed']},message:'更新流程測試',downloadError:'舊錯誤',available:true,asset:{browser_download_url:'example'}},lang);
   f.render();assert(f.$('#release-dialog').open);
   assert.equal(f.$('#release-version').textContent,'installed');
   assert(f.$('#release-later').hidden);assert(f.$('#release-status').hidden);
   assert.equal(f.$('#release-status').textContent,'');
   assert(!f.$('#release-download').disabled);
   assert.equal(f.$('#release-download').textContent,lang==='zh-Hant'?'確定':lang==='ko'?'확인':'OK');
   await f.$(button).handlers.click();
   assert.deepEqual(f.calls,['updates/notes/ack']);assert(!f.$('#release-dialog').open);
   assert(!f.context.state.updates.showNotes);
   f.context.state.updates={available:true,version:'next',asset:{browser_download_url:'example'}};
   f.render();assert(!f.$('#release-later').hidden);assert(!f.$('#release-status').hidden);
   assert.notEqual(f.$('#release-download').textContent,lang==='zh-Hant'?'確定':lang==='ko'?'확인':'OK');
   await f.$('#release-download').handlers.click();assert(f.calls.includes('updates/download'));
  }
 }
 console.log('PASS: 四語更新完成通知只確認、不下載、不取消下一版更新，正常更新按鈕可恢復');
})().catch(e=>{console.error(e);process.exitCode=1});
