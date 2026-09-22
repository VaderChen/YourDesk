// 真實設定頁及關閉事件；隔離 API，啟動不詢問，只有手動開啟 Switch 才能觸發授權。
const fs=require('fs'),path=require('path'),assert=require('assert'),{chromium}=require('playwright');
const root=path.resolve(__dirname,'../internal/clientui/web'),origin='https://yourdesk-sas.test';
const defaults=()=>({info:{room:'YD-SMOKE',hostname:'Smoke',platform:'windows'},library:{groups:[],sites:[]},preferences:{language:'zh-Hant',theme:'light',selectedGroup:'*'},running:{host:true},sessions:{},updates:{},hardwareDetection:{status:'complete'},audioCapabilities:[],prelogin:{supported:true,sasSupported:true,enabled:false,sasAllowed:false}});
(async()=>{const browser=await chromium.launch({headless:true});try{
 let state=defaults(),requests=[],errors=[],context,page,failToggle=false;
 async function reopen(){
  if(context)await context.close();
  context=await browser.newContext();page=await context.newPage();
  page.on('pageerror',error=>errors.push(error.message));
  await page.route('**/*',route=>{
   const request=route.request(),url=new URL(request.url());if(url.origin!==origin)return route.abort();
   if(url.pathname.startsWith('/api/')){
    let value={};
    if(url.pathname==='/api/state')value=state;
    else if(request.method()==='PUT'&&url.pathname==='/api/preferences'){
     const body=request.postDataJSON();requests.push({path:url.pathname,body});
     Object.assign(state.preferences,body);
    }else if(request.method()==='POST'&&url.pathname==='/api/prelogin/sas'){
     const body=request.postDataJSON();requests.push({path:url.pathname,body});
     if(failToggle)return route.fulfill({status:400,json:{error:'登入前服務尚未支援 Ctrl+Alt+Del 開關，請更新 APP 與服務後再試。'}});
     state.prelogin.sasDisabled=!body.enabled;
    }else if(request.method()==='POST'&&url.pathname==='/api/prelogin'){
     const body=request.postDataJSON();requests.push({path:url.pathname,body});
     state.prelogin.enabled=true;state.prelogin.sasAllowed=true;
    }
    return route.fulfill({json:value});
   }
   const name=url.pathname==='/'?'index.html':url.pathname.slice(1),file=path.resolve(root,name);
   if(!file.startsWith(root+path.sep)||!fs.existsSync(file))return route.abort();
   return route.fulfill({body:fs.readFileSync(file),contentType:({'.html':'text/html','.js':'text/javascript','.css':'text/css','.json':'application/json'})[path.extname(file)]||'application/octet-stream'});
  });
  await page.goto(origin+'/#smoke');await page.waitForFunction(()=>document.querySelector('#ui-language').value==='zh-Hant');
 }
 const serviceRequests=()=>requests.filter(request=>request.path==='/api/prelogin');
 for(const close of ['cancel','close','escape']){
  state=defaults();requests=[];await reopen();
  await page.evaluate(()=>renderPrelogin());
  assert(!(await page.locator('#confirm-dialog').isVisible()),'首次啟動不應詢問授權');
  assert.equal(requests.length,0);
  await page.evaluate(()=>{openDialog('#settings-dialog');selectSettingsTab('advanced')});
  assert(!(await page.locator('#confirm-dialog').isVisible()),'開啟進階設定不應自動詢問授權');
  await page.locator('#authorize-sas').click();
  await page.locator('#confirm-dialog').waitFor({state:'visible'});
  assert.match(await page.locator('#confirm-description').textContent(),/開機自動啟動/);
  assert.equal(serviceRequests().length,0);
  if(close==='escape')await page.keyboard.press('Escape');
  else await page.locator(close==='cancel'?'#confirm-dialog .modal-actions .close':'#confirm-dialog .modal-heading .close').click();
  await page.waitForFunction(()=>!document.querySelector('#confirm-dialog').open);
  assert.equal(requests.length,0,'取消授權不需要改寫偏好或呼叫服務');
  assert(!(await page.locator('#authorize-sas').isChecked()));
  await reopen();await page.evaluate(()=>renderPrelogin());
  assert(!(await page.locator('#confirm-dialog').isVisible()),`${close} 後重開不得再自動詢問`);
  await page.evaluate(()=>{openDialog('#settings-dialog');selectSettingsTab('general')});
  await page.selectOption('#ui-theme','dark');await page.waitForFunction(()=>!document.querySelector('#ui-theme').disabled);
  await reopen();assert(!(await page.locator('#confirm-dialog').isVisible()));
  await page.evaluate(()=>{openDialog('#settings-dialog');selectSettingsTab('advanced')});
  await page.locator('#authorize-sas').click();await page.locator('#confirm-dialog').waitFor({state:'visible'});
  assert.equal(serviceRequests().length,0);
  await page.locator('#confirm-form button[type="submit"]').click();
  await page.waitForFunction(()=>!document.querySelector('#confirm-dialog').open);
  assert.equal(serviceRequests().length,1);assert.deepEqual(serviceRequests()[0].body,{enabled:true,secureAttention:true});
  assert.equal(await page.locator('#authorize-sas').getAttribute('role'),'switch');assert(await page.locator('#authorize-sas').isChecked());
  await page.locator('#authorize-sas').uncheck();await page.waitForFunction(()=>!document.querySelector('#authorize-sas').disabled);
  assert(state.prelogin.sasDisabled);assert(state.prelogin.enabled);assert.equal(serviceRequests().length,1);
  await reopen();assert(!(await page.locator('#confirm-dialog').isVisible()));
  await page.evaluate(()=>{openDialog('#settings-dialog');selectSettingsTab('advanced')});
  assert(!(await page.locator('#authorize-sas').isChecked()));
  await page.locator('#authorize-sas').check();await page.waitForFunction(()=>!document.querySelector('#authorize-sas').disabled);
  assert(!state.prelogin.sasDisabled);assert.equal(serviceRequests().length,1,'已授權的開關不應再次要求管理員授權');
 }
 // 不論舊版有沒有儲存「不再詢問」，啟動與輪詢都不得自動要求授權。
 for(const service of [
  {supported:true,sasSupported:true,enabled:false,sasAllowed:false},
  {supported:true,sasSupported:true,enabled:true,sasAllowed:true},
  {supported:true,enabled:true},
  {supported:true,sasSupported:true,busy:true},
  {supported:true,sasSupported:true,enabled:true,sasAllowed:false}
 ])for(const dismissed of [undefined,false,true]){
  state=defaults();state.prelogin=service;state.preferences.sasPromptDismissed=dismissed;requests=[];await reopen();
  await page.evaluate(()=>updateRunning());
  assert(!(await page.locator('#confirm-dialog').isVisible()));assert.equal(requests.length,0);
 }
 // 舊服務或拒絕設定時，開關還原至服務狀態，不假裝已停用。
 state=defaults();state.prelogin.enabled=true;state.prelogin.sasAllowed=true;failToggle=true;requests=[];await reopen();
 await page.evaluate(()=>{openDialog('#settings-dialog');selectSettingsTab('advanced')});
 const rejected=page.waitForResponse(response=>response.url().endsWith('/api/prelogin/sas')&&response.status()===400);
 await page.locator('#authorize-sas').click();await rejected;
 await page.waitForFunction(()=>!document.querySelector('#authorize-sas').disabled);
 assert(await page.locator('#authorize-sas').isChecked());assert(!state.prelogin.sasDisabled);assert.equal(serviceRequests().length,0);
 failToggle=false;await reopen();
 await page.evaluate(()=>{openDialog('#settings-dialog');selectSettingsTab('advanced')});
 fs.mkdirSync(path.resolve(__dirname,'../.local-run'),{recursive:true});
 await page.locator('#settings-dialog').screenshot({path:path.resolve(__dirname,'../.local-run/sas-switch-smoke.png')});
 assert.deepEqual(errors,[]);
 console.log('PASS: 首次啟動／重開／狀態輪詢均不詢問、不受舊偏好影響、手動開啟並確認才提權、取消／關閉／Esc 不呼叫服務、Switch 狀態保留、已授權切換不重複提權、失敗還原');
}finally{await browser.close()}})().catch(error=>{console.error(error);process.exitCode=1});
