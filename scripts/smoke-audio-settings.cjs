// 實際設定頁 DOM 與事件；隔離 API，不更動使用者偏好。
const fs=require('fs'),path=require('path'),assert=require('assert'),{chromium}=require('playwright');
const root=path.resolve(__dirname,'../internal/clientui/web'),origin='https://yourdesk-audio.test';
(async()=>{const browser=await chromium.launch({headless:true});try{
 const page=await browser.newPage({viewport:{width:1000,height:800}}),errors=[],writes=[];
 page.on('pageerror',e=>errors.push(e.message));
 const state={prelogin:{supported:true,enabled:true,room:'YD-SERVICE'},info:{room:'YD-DEMO',hostname:'Smoke',platform:'darwin',architecture:'arm64'},library:{groups:[],sites:[]},preferences:{language:'zh-Hant',theme:'light',selectedGroup:'*'},running:{host:true},sessions:{},updates:{},hardwareDetection:{status:'complete'},audioCapabilities:[{codec:'opus',encode:true,decode:true,hardwareEncode:false,hardwareDecode:false},{codec:'pcm',encode:true,decode:true},{codec:'aac',encode:true,decode:true,hardwareEncode:false,hardwareDecode:false}]};
 await page.route('**/*',route=>{const req=route.request(),url=new URL(req.url());if(url.origin!==origin)return route.abort();
  if(url.pathname.startsWith('/api/')){let value={};if(url.pathname==='/api/state')value=state;else if(url.pathname==='/api/preferences'&&req.method()==='PUT'){value=req.postDataJSON();writes.push(value);state.preferences=value;}return route.fulfill({json:value})}
  const name=url.pathname==='/'?'index.html':url.pathname.slice(1),p=path.resolve(root,name);if(!p.startsWith(root+path.sep)||!fs.existsSync(p))return route.abort();return route.fulfill({body:fs.readFileSync(p),contentType:({'.html':'text/html','.js':'text/javascript','.css':'text/css','.json':'application/json'})[path.extname(p)]||'application/octet-stream'})
 });
 await page.goto(origin+'/#smoke-token');await page.waitForFunction(()=>document.querySelector('#ui-language').value==='zh-Hant');
 await page.evaluate(()=>{openDialog('#settings-dialog');selectSettingsTab('advanced')});
 assert(!(await page.locator('#ui-remote-audio').isChecked()));assert(!(await page.locator('#audio-codec-row').isVisible()));
 assert.equal(await page.locator('#ui-audio-codec').inputValue(),'opus');
 await page.evaluate(()=>selectSettingsTab('stream'));
 assert(await page.locator('#audio-codec-row').isVisible(),'關閉聲音時仍可預先選擇輸出編碼');
 assert.equal(await page.locator('label[for="stream-codec"]').textContent(),'影像輸出編碼');
 assert.equal(await page.locator('label[for="ui-audio-codec"]').textContent(),'聲音輸出編碼');
 assert.equal(await page.locator('#settings-panel-stream #ui-audio-codec').count(),1);
 assert.deepEqual(await page.locator('#ui-audio-codec optgroup').evaluateAll(groups=>groups.map(g=>g.label)),['可用','不支援']);
 assert.deepEqual(await page.locator('#ui-audio-codec optgroup:not([disabled]) option').evaluateAll(options=>options.map(o=>o.value)),['auto','aac','opus','aac-software','pcm'],'AAC 硬體優先可回退軟體，不能因缺少硬體就歸為不支援');
 await page.selectOption('#stream-codec','software-jpeg');await page.waitForFunction(()=>!document.querySelector('#stream-codec').disabled);assert.equal(writes.at(-1).codec,'software-jpeg');assert(state.prelogin.enabled);
 await page.selectOption('#stream-codec','auto');await page.waitForFunction(()=>!document.querySelector('#stream-codec').disabled);
 await page.selectOption('#stream-codec-goal','bandwidth');await page.waitForFunction(()=>!document.querySelector('#stream-codec-goal').disabled);assert.equal(writes.at(-1).codecGoal,'bandwidth');
 await page.evaluate(()=>selectSettingsTab('advanced'));
 await page.locator('#ui-remote-audio').check();await page.waitForFunction(()=>!document.querySelector('#ui-remote-audio').disabled);
 assert(writes.at(-1).remoteAudio);assert.equal(writes.at(-1).audioCodec,'opus');
 await page.evaluate(()=>selectSettingsTab('stream'));
 assert(await page.locator('#audio-codec-row').isVisible());assert.match(await page.locator('#audio-capabilities').textContent(),/Opus：軟體解碼/);
 await page.selectOption('#ui-audio-codec','aac-software');await page.waitForFunction(()=>!document.querySelector('#ui-audio-codec').disabled);assert.equal(writes.at(-1).audioCodec,'aac-software');
 assert.match(await page.locator('#audio-capabilities').textContent(),/AAC：軟體解碼/);
 await page.reload();await page.waitForFunction(()=>document.querySelector('#ui-language').value==='zh-Hant');
 await page.evaluate(()=>{openDialog('#settings-dialog');selectSettingsTab('stream')});
 assert.equal(await page.locator('#ui-audio-codec').inputValue(),'aac-software','明確保存的 AAC 不可被預設覆寫');
 await page.selectOption('#ui-audio-codec','auto');await page.waitForFunction(()=>!document.querySelector('#ui-audio-codec').disabled);assert.equal(writes.at(-1).audioCodec,'auto');assert.match(await page.locator('#audio-capabilities').textContent(),/AAC 硬體 → Opus 軟體 → AAC 軟體 → PCM 未壓縮/);
 await page.selectOption('#ui-audio-codec','pcm');await page.waitForFunction(()=>!document.querySelector('#ui-audio-codec').disabled);
 assert.match(await page.locator('#audio-capabilities').textContent(),/PCM：軟體解碼/);
 await page.selectOption('#ui-audio-codec','opus');await page.waitForFunction(()=>!document.querySelector('#ui-audio-codec').disabled);
 assert.equal(writes.at(-1).audioCodec,'opus');
 assert.equal(await page.locator('#settings-panel-advanced select').count(),0,'不可另設獨立聲音品質選單');
 await page.screenshot({path:'/tmp/yourdesk-audio-settings.png'});
 await page.evaluate(()=>selectSettingsTab('advanced'));
 await page.locator('#ui-remote-audio').uncheck();await page.waitForFunction(()=>!document.querySelector('#ui-remote-audio').disabled);assert(!writes.at(-1).remoteAudio);
 await page.evaluate(()=>selectSettingsTab('stream'));
 assert(await page.locator('#audio-codec-row').isVisible());assert(!(await page.locator('#audio-capabilities').isVisible()));assert.deepEqual(errors,[]);
 // 能力更新應保留既有選擇；只依本機解碼能力分類，來源能力由連線協商。
 state.audioCapabilities.find(cap=>cap.codec==='aac').decode=false;
 state.audioCapabilities.find(cap=>cap.codec==='opus').encode=false;
 state.preferences.audioCodec='aac-software';
 await page.reload();await page.waitForFunction(()=>document.querySelector('#ui-language').value==='zh-Hant');
 await page.evaluate(()=>{openDialog('#settings-dialog');selectSettingsTab('stream')});
 assert.deepEqual(await page.locator('#ui-audio-codec optgroup:not([disabled]) option').evaluateAll(options=>options.map(o=>o.value)),['auto','opus','pcm']);
 assert.deepEqual(await page.locator('#ui-audio-codec optgroup[disabled] option').evaluateAll(options=>options.map(o=>({value:o.value,disabled:o.disabled}))),[{value:'aac',disabled:true},{value:'aac-software',disabled:true}]);
 assert.equal(await page.locator('#ui-audio-codec').inputValue(),'aac-software','偵測為不支援時不可偷偷改寫保存的編碼');
 const translations=JSON.parse(fs.readFileSync(path.join(root,'translations.json'),'utf8'));
 for(const [index,language] of [[0,'en'],[1,'ja'],[2,'ko'],[-1,'zh-Hant']]){
  await page.evaluate(language=>{i18n.apply(language);renderAudioSettings()},language);
  assert.deepEqual(await page.locator('#ui-audio-codec optgroup').evaluateAll(groups=>groups.map(g=>g.label)),index<0?['可用','不支援']:[translations['可用'][index],translations['不支援'][index]]);
 }
 assert(await page.evaluate(()=>{const option=document.querySelector('#ui-audio-codec option');renderAudioSettings();return option===document.querySelector('#ui-audio-codec option')}),'相同偵測結果不應重建開啟中的選單');
 const capabilities=state.audioCapabilities;
 state.audioCapabilities=null;await page.evaluate(()=>updateRunning());
 assert(await page.locator('#ui-audio-codec').isDisabled());
 assert.equal(await page.locator('#ui-audio-codec').inputValue(),'aac-software');
 assert.equal(await page.locator('#ui-audio-codec option').textContent(),'聲音能力偵測中…');
 state.audioCapabilities=capabilities;state.audioCapabilities.find(cap=>cap.codec==='aac').decode=true;
 await page.evaluate(()=>updateRunning());
 assert(!(await page.locator('#ui-audio-codec').isDisabled()));
 assert.equal(await page.locator('#ui-audio-codec').inputValue(),'aac-software');
 assert.equal(await page.locator('#ui-audio-codec optgroup[disabled] option').count(),0);
 await page.selectOption('#ui-audio-codec','opus');await page.waitForFunction(()=>!document.querySelector('#ui-audio-codec').disabled);
 assert.equal(writes.at(-1).audioCodec,'opus');assert.deepEqual(errors,[]);
 console.log('PASS: 預設關閉、Opus 預設、設定持久化、可用／不支援分組、停用選項、偵測更新保留選擇、四語系、共用品質選單');
}finally{await browser.close()}})().catch(e=>{console.error(e);process.exit(1)});
