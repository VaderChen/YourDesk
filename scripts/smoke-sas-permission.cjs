// 驗證啟動檢查：確認前不提權、每次開啟只詢問一次、設定按鈕可重試。
const fs=require('fs'),vm=require('vm'),assert=require('assert'),path=require('path');
const source=fs.readFileSync(path.join(__dirname,'../internal/clientui/web/app.js'),'utf8');
function setup(service){
 const elements=new Map(),requests=[],prompts=[];let dialog=false;
 const $=id=>{if(!elements.has(id))elements.set(id,{handlers:{},classList:{toggle(){}},dataset:{},addEventListener(k,f){this.handlers[k]=f},closest(){return this}});return elements.get(id)};
 const context=vm.createContext({$,state:{prelogin:service},i18n:{t:s=>s},document:{querySelector:()=>dialog?{}:null},confirmDelete:(title,message,callback)=>{dialog=true;prompts.push({message,callback})},toast(){},action:f=>f(),api:async(...args)=>requests.push(args),updateRunning:async()=>{}});
 vm.runInContext(source.slice(source.indexOf('let preloginWasBusy='),source.indexOf("$('#stop-incoming')",source.indexOf('let preloginWasBusy='))),context);
 return {context,$,requests,prompts,close:()=>{dialog=false},render:()=>vm.runInContext('renderPrelogin()',context)};
}
(async()=>{
 const x=setup({supported:true,sasSupported:true,enabled:false,sasAllowed:false});
 x.render();assert.equal(x.prompts.length,1);assert.equal(x.requests.length,0);assert(x.prompts[0].message.includes('開機自動啟動'));
 x.close();x.render();assert.equal(x.prompts.length,1);
 x.$('#authorize-sas').handlers.click();assert.equal(x.prompts.length,2);
 await x.prompts[1].callback();assert.equal(x.requests.length,1);assert.equal(x.requests[0][2].secureAttention,true);
 const ready=setup({supported:true,sasSupported:true,enabled:true,sasAllowed:true});ready.render();assert.equal(ready.prompts.length,0);assert(ready.$('#authorize-sas').disabled);
 const mac=setup({supported:true,enabled:true});mac.render();assert.equal(mac.prompts.length,0);assert(mac.$('#authorize-sas').hidden);
 const busy=setup({supported:true,sasSupported:true,busy:true});busy.render();assert.equal(busy.prompts.length,0);
 console.log('PASS: 啟動詢問、取消不重複、確認才提權、設定重試、已授權及非 Windows 不詢問');
})().catch(e=>{console.error(e);process.exitCode=1});
