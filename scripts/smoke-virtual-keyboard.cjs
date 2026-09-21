// 真實 DOM：驗證全尺寸配置、修飾鍵、平台切換及兩種 WebView 容器。
const {chromium}=require('playwright');
const fs=require('fs'),assert=require('assert'),path=require('path');
const root=path.resolve(__dirname,'../cmd/remote/web');
// 從產品共用表解析測試請求，避免另外維護一份掃描碼。
const source=fs.readFileSync(path.join(root,'../../../internal/rawkey/event.go'),'utf8');
const codes={};for(const platform of ['darwin','windows']){
 const block=source.split('"'+platform+'": {')[1].split('\n\t},')[0];codes[platform]={};
 for(const m of block.matchAll(/(\d+):\s*"([^"]+)"/g))codes[platform][m[2]]=Number(m[1]);
}
(async()=>{const browser=await chromium.launch({headless:true});try{
 const page=await browser.newPage({viewport:{width:1200,height:600}}),errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.setContent('<script>window.messages=[];window.webkit={messageHandlers:{titlebar:{postMessage:m=>messages.push(m)}}};window.ydTitlebar=async m=>messages.push(m);<\/script>'+require('./smoke-titlebar-fixture.cjs')());

 assert.deepEqual(errors,[],'產品 HTML 不得有 JavaScript 載入錯誤');
 async function verifyToolbar(windows){
  await page.evaluate(()=>window.setTitlebarState({title:'YourDesk smoke',count:2,display:0,crop:0,mode:0,quality:1,fps:30}));
  for(const button of await page.locator('header button[data-action]:visible').all()){
   const expected=Number(await button.getAttribute('data-action'));
   await page.evaluate(()=>{messages=[]});await button.click();
   assert((await page.evaluate(()=>messages)).some(m=>windows?m.action===expected:m===expected),'按鈕未送出 action '+expected);
  }
  for(const menu of ['shortcuts','quality','scale']){
   await page.evaluate(()=>{messages=[]});await page.locator('[data-menu="'+menu+'"]').click();
   assert((await page.evaluate(()=>messages)).some(m=>windows?m.nativeMenu===menu:m.menu===menu),'選單未送出 '+menu);
  }
  await page.locator('#codec-status').hover();
  assert((await page.evaluate(()=>messages)).some(m=>windows?m.popup:m.tooltip),'狀態泡泡未回應');
 }
 await verifyToolbar(false);
 const request={platform:'darwin',codes};
 const html=await page.evaluate(r=>window.virtualKeyboardDocument(r),request);
 const mac=await browser.newPage({viewport:{width:1080,height:350}});
 await mac.setContent('<script>window.messages=[];window.webkit={messageHandlers:{titlebar:{postMessage:m=>messages.push(m)}}};<\/script>'+html);
 assert.equal(await mac.locator('[data-key="f19"]').count(),1);
 assert.equal(await mac.locator('[data-key^="kp"]').count(),17);
 await mac.locator('[data-key="leftsuper"]').click();await mac.locator('[data-key="c"]').click();
 assert.deepEqual(await mac.evaluate(()=>messages.filter(m=>typeof m==='number')),[1000+8*512+codes.darwin.c]);
 assert.equal(await mac.locator('[aria-pressed=true]').count(),0);
 await mac.locator('[data-key="capslock"]').click();await mac.locator('[data-key="a"]').click();
 assert.equal((await mac.evaluate(()=>messages.filter(m=>typeof m==='number'))).at(-1),1000+16384+codes.darwin.a);
 await mac.selectOption('select','windows');assert.equal(await mac.locator('[data-key="f19"]').count(),0);
 assert.equal(await mac.locator('[data-key="printscreen"]').count(),1);
 assert.equal(await mac.locator('[data-key="leftsuper"]').textContent(),'Win');
 await mac.selectOption('select','numeric');
 assert.equal(await mac.locator('.vk-main:visible').count(),0);
 assert.equal(await mac.locator('[data-key="kp7"]:visible').count(),1);
 await mac.selectOption('select','zhuyin');
 assert((await mac.locator('[data-key="q"]').textContent()).includes('ㄆ'));
 assert((await mac.locator('[data-key="1"]').textContent()).includes('ㄅ'));
 assert.equal(await mac.locator('.vk-hint').count(),0);
 const head=await mac.locator('.vk-head strong').boundingBox();
 await mac.mouse.move(head.x+10,head.y+5);await mac.mouse.down();await mac.mouse.move(head.x+65,head.y+30);await mac.mouse.up();
 assert((await mac.evaluate(()=>messages)).some(m=>m.keyboardDrag==='move'&&m.dx===55&&m.dy===25));
 await mac.screenshot({path:'/tmp/yourdesk-virtual-keyboard.png'});
 await page.addScriptTag({path:path.join(root,'titlebar_windows.js')});
 await verifyToolbar(true);
 await page.evaluate(r=>window.showVirtualKeyboard(r),{...request,platform:'windows'});
 await page.evaluate(()=>window.dispatchEvent(new Event('blur')));
 assert(await page.locator('.virtual-keyboard').isVisible(),'切回遠端畫面不應關閉鍵盤');
 assert((await page.evaluate(()=>messages)).some(m=>m.popup&&m.keyboard&&!m.menu),'鍵盤不能標記為阻塞整個畫面的選單');
 await page.locator('.vk [data-key="leftcontrol"]').click();await page.locator('.vk [data-key="leftalt"]').click();await page.locator('.vk [data-key="delete"]').click();
 assert((await page.evaluate(()=>messages)).some(m=>m.action===1000+8192+6*512+codes.windows.delete));
 await page.selectOption('.vk select','numeric');
 await page.waitForTimeout(50);
 const before=await page.locator('.virtual-keyboard').boundingBox();assert(before.width<350);
 const handle=await page.locator('.vk-head strong').boundingBox();
 await page.mouse.move(handle.x+10,handle.y+5);await page.mouse.down();await page.mouse.move(handle.x+65,handle.y+40);await page.mouse.up();
 const after=await page.locator('.virtual-keyboard').boundingBox();
 assert(after.x>before.x&&after.y>before.y,'鍵盤應隨拖曳移動');
 await page.selectOption('.vk select','zhuyin');assert((await page.locator('.vk [data-key="q"]').textContent()).includes('ㄆ'));
 await page.evaluate(()=>window.showInputNotice('SAS 測試錯誤 <保留原文>'));
 assert.equal(await page.locator('.vk [role="status"]').textContent(),'SAS 測試錯誤 <保留原文>');
 assert.equal(await page.locator('.vk [role="status"] *').count(),0);
 await page.locator('.vk-close').click();assert((await page.evaluate(()=>messages)).some(m=>m.action===42));
 assert.deepEqual(errors,[]);console.log('PASS: Mac / Windows 全尺寸鍵盤、組合鍵、Caps Lock、平台切換及關閉');
}finally{await browser.close()}})().catch(e=>{console.error(e);process.exit(1)});
