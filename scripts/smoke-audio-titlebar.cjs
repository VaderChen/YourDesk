const fs=require('fs'),path=require('path'),assert=require('node:assert/strict'),{chromium}=require('playwright');
const html=require('./smoke-titlebar-fixture.cjs')();
(async()=>{const browser=await chromium.launch({headless:true});try{
 const page=await browser.newPage({viewport:{width:1000,height:120}}),errors=[];
 page.on('pageerror',e=>errors.push(e.message));
 await page.addInitScript(()=>{window.messages=[];window.webkit={messageHandlers:{titlebar:{postMessage:m=>window.messages.push(m)}}};});
 await page.route('https://titlebar.test/',r=>r.fulfill({body:html,contentType:'text/html'}));await page.goto('https://titlebar.test/');
 const state={title:'YourDesk',count:1,display:0,mode:2,audio:{enabled:false}};
 for(const windows of [false,true]){
  if(windows){await page.evaluate(()=>window.ydTitlebar=async message=>window.messages.push(message.action??message));await page.addScriptTag({content:fs.readFileSync(path.resolve('cmd/remote/web/titlebar_windows.js'),'utf8')});}
  for(const enabled of [false,true,false]){
   await page.evaluate(s=>window.setTitlebarState(s),{...state,audio:{enabled}});
   const button=page.locator('#audio-toggle');assert.equal(await button.getAttribute('aria-pressed'),String(enabled));
   assert.equal(await button.getAttribute('aria-label'),enabled?'關閉遠端聲音':'開啟遠端聲音');
   assert.equal(await button.locator('.fa').textContent(),enabled?'\uf028':'\uf6a9');
   await button.click();assert.equal(await page.evaluate(()=>window.messages.at(-1)),18);
   assert.equal(await page.locator('.audio-control').evaluate(el=>getComputedStyle(el).borderRightWidth),'1px');
   const audio=await button.boundingBox(),crop=await page.locator('#crop').boundingBox();assert(audio.x+audio.width<crop.x);
  }
  for(const width of [640,1000]){await page.setViewportSize({width,height:120});assert(await page.evaluate(()=>document.querySelector('header').scrollWidth<=innerWidth),'工具列不可溢出');}
  await page.screenshot({path:path.resolve('.local-run/audio-toggle-'+(windows?'windows':'macos')+'.png')});
 }
 await page.evaluate(s=>window.setTitlebarState(s),{...state,language:'en',strings:{'開啟遠端聲音':'Enable remote audio'}});
 assert.equal(await page.locator('#audio-toggle').getAttribute('aria-label'),'Enable remote audio');
 for(const width of [640,1000]){
  await page.setViewportSize({width,height:120});
  assert(await page.evaluate(()=>document.querySelector('header').scrollWidth<=innerWidth),'窄視窗工具列不可溢出');
 }
 const output=path.resolve('.local-run/audio-toggle.png');await page.screenshot({path:output});
 assert.deepEqual(errors,[]);console.log('PASS: 喇叭開關狀態、Mac/Windows 橋接、位置、分隔線、語系；'+output);
}finally{await browser.close()}})().catch(e=>{console.error(e);process.exit(1)});
