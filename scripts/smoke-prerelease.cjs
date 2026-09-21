// 執行實際設定頁事件，驗證開關、查詢來源與確認流程。
const fs=require('fs'),vm=require('vm'),assert=require('assert'),path=require('path');
const source=fs.readFileSync(path.join(__dirname,'../internal/clientui/web/app.js'),'utf8');
const html=fs.readFileSync(path.join(__dirname,'../internal/clientui/web/index.html'),'utf8');
assert(/id="install-prerelease"[^>]*hidden/.test(html));
const elements=new Map(),calls=[],renders=[];
const $=id=>{if(!elements.has(id))elements.set(id,{checked:false,hidden:true,handlers:{},addEventListener(k,f){this.handlers[k]=f}});return elements.get(id)};
const context=vm.createContext({$,state:{},i18n:{t:s=>s},api:async route=>{calls.push(route);return {available:true,version:'test',message:'test'}},renderRelease:(r,manual)=>renders.push({r,manual}),action:f=>f(),downloadUpdate:()=>{throw Error('查詢不應直接安裝')}});
vm.runInContext(source.slice(source.indexOf('async function checkUpdate('),source.indexOf('// 使用原生拖曳影像預覽')),context);
(async()=>{
 await $('#install-prerelease').handlers.click();assert.equal(calls.length,0);
 $('#force-update').checked=true;$('#force-update').handlers.change();assert(!$('#install-prerelease').hidden);
 await $('#install-prerelease').handlers.click();assert.equal(calls.pop(),'updates?force=true&prerelease=true');assert(renders.pop().manual);
 await $('#check-update').handlers.click();assert.equal(calls.pop(),'updates?force=true');
 $('#force-update').checked=false;$('#force-update').handlers.change();assert($('#install-prerelease').hidden);
 await $('#check-update').handlers.click();assert.equal(calls.pop(),'updates');
 console.log('PASS: 強制更新開關、測試版查詢與安裝確認、正式版查詢');
})().catch(e=>{console.error(e);process.exitCode=1});
