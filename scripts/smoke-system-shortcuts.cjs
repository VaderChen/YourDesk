// 真實 DOM 驗證快捷鍵詢問與取消；橋接只記錄 action，不操作本機或遠端。
const {chromium}=require(process.env.YOURDESK_PLAYWRIGHT_MODULE||'playwright');
const fs=require('fs'),path=require('path'),assert=require('assert');
const root=path.resolve(__dirname,'../cmd/remote/web');
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.YOURDESK_SMOKE_BROWSER,headless:true});
 try {
  const page=await browser.newPage({viewport:{width:800,height:500}}),errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  await page.setContent('<script>window.messages=[];window.webkit={messageHandlers:{titlebar:{postMessage:m=>messages.push(m)}}};window.ydTitlebar=async m=>messages.push(m);<\/script>'+require('./smoke-titlebar-fixture.cjs')());
  await page.addScriptTag({path:path.join(root,'titlebar_windows.js')});
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  const actions=()=>page.evaluate(()=>messages.filter(m=>m&&m.action).map(m=>m.action));
  const open=async(secure=false)=>page.evaluate(secure=>{messages=[];window.showSystemShortcut({label:secure?'Ctrl+Alt+Del':'Alt+F4',secure})},secure);
  for(const [button,action] of [['本機',40],['遠端',41],['取消',42]]){
   await open();assert.deepEqual(await actions(),[],'確認前不得執行');
   assert.equal(await page.locator(':focus').textContent(),'遠端','預設焦點應為遠端');
   await page.getByRole('dialog').getByRole('button',{name:button,exact:true}).click();
   assert.deepEqual(await actions(),[action],'只能回報一次選擇');
  }
  await open();await page.keyboard.press('Escape');assert.deepEqual(await actions(),[42]);
  await open();await page.keyboard.press('Enter');assert.deepEqual(await actions(),[41]);
  await open();await page.keyboard.press('Shift+Tab');assert.equal(await page.locator(':focus').textContent(),'本機');
  await page.keyboard.press('Tab');assert.equal(await page.locator(':focus').textContent(),'遠端');
  await page.keyboard.press('Tab');assert.equal(await page.locator(':focus').textContent(),'取消');
  await page.evaluate(()=>window.dispatchEvent(new Event('blur')));assert.deepEqual(await actions(),[42]);
  await open(true);
  assert(await page.getByRole('button',{name:'本機',exact:true}).isDisabled());
  assert(!(await page.getByRole('button',{name:'遠端',exact:true}).isDisabled()));
  assert((await page.getByRole('dialog').textContent()).includes('登入前服務'));
  assert.equal(await page.locator(':focus').textContent(),'遠端','安全快捷鍵可選遠端服務');
  await page.keyboard.press('Enter');assert.deepEqual(await actions(),[41]);
  await page.evaluate(()=>{messages=[];window.showSystemShortcut({label:'Alt+F4',secure:false,remoteAvailable:false})});
  assert(await page.getByRole('button',{name:'遠端',exact:true}).isDisabled());
  assert(!(await page.getByRole('button',{name:'本機',exact:true}).isDisabled()));
  assert.equal(await page.locator(':focus').textContent(),'取消','遠端不可用時預設取消');
  await page.keyboard.press('Enter');assert.deepEqual(await actions(),[42]);
  await open();
  await page.evaluate(()=>{Object.defineProperty(navigator,'platform',{value:'MacIntel',configurable:true});document.dispatchEvent(new KeyboardEvent('keydown',{key:'q',metaKey:true,bubbles:true,cancelable:true}))});
  assert.deepEqual(await actions(),[],'對話框期間不能直接執行另一個關閉快捷鍵');
  await page.keyboard.press('Escape');assert.deepEqual(await actions(),[42]);
  // 在工具列取得焦點時，生命週期按鍵必須請求選擇，不能回報直接退出 action。
  for(const [platform,key,mods,expected] of [['MacIntel','q',{metaKey:true},51],['MacIntel','w',{metaKey:true},50],['Win32','w',{ctrlKey:true},50],['Win32','F4',{altKey:true},52]]){
   await page.evaluate(({platform,key,mods})=>{messages=[];Object.defineProperty(navigator,'platform',{value:platform,configurable:true});document.dispatchEvent(new KeyboardEvent('keydown',{key,...mods,bubbles:true,cancelable:true}))},{platform,key,mods});
   assert.deepEqual(await actions(),[expected]);
  }
  for(const key of ['c','v']){
   const prevented=await page.evaluate(key=>{messages=[];const e=new KeyboardEvent('keydown',{key,ctrlKey:true,bubbles:true,cancelable:true});document.dispatchEvent(e);return e.defaultPrevented},key);
   assert.equal(prevented,false);assert.deepEqual(await actions(),[]);
  }
  const dictionary=JSON.parse(fs.readFileSync(path.join(root,'translations.json'),'utf8'));
  await page.setViewportSize({width:640,height:360});
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  for(const [i,language] of [[-1,'zh-Hant'],[0,'en'],[1,'ja'],[2,'ko']]){
   const strings=i<0?{}:Object.fromEntries(Object.entries(dictionary).map(([key,values])=>[key,values[i]]));
   await page.evaluate(state=>window.updateWindowsTitlebar(state),{strings,language});
   await open();
   const bounds=await page.getByRole('dialog').boundingBox();assert(bounds.x>=0&&bounds.x+bounds.width<=640);
   await page.keyboard.press('Escape');
  }
  await page.setViewportSize({width:800,height:500});
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  await page.evaluate(()=>window.updateWindowsTitlebar({strings:{},language:'zh-Hant'}));await open();
  if(process.env.YOURDESK_SHORTCUT_SCREENSHOT)await page.screenshot({path:process.env.YOURDESK_SHORTCUT_SCREENSHOT});

  // macOS 使用獨立 WebView sheet；驗證實際產生的 HTML 與一次性橋接。
  const sheet=await browser.newPage({viewport:{width:480,height:300}});
  sheet.on('pageerror',e=>errors.push(e.message));
  for(const [button,action] of [['本機',40],['遠端',41],['取消',42]]){
   const html=await page.evaluate(()=>window.systemShortcutDocument({label:'Cmd+W',secure:false,remoteAvailable:true}));
   await sheet.goto('about:blank');
   await sheet.setContent(html.replace('<head>','<head><script>window.actions=[];window.webkit={messageHandlers:{titlebar:{postMessage:a=>actions.push(a)}}};<\/script>'));
   assert.equal(await sheet.locator(':focus').textContent(),'遠端');
   await sheet.waitForFunction(()=>actions.some(a=>a&&a.shortcutHeight));
   const height=await sheet.evaluate(()=>actions.filter(a=>a&&a.shortcutHeight).at(-1).shortcutHeight);
   assert(height>=140&&height<220,'一般快捷鍵視窗應依內容縮短: '+height);
   await sheet.setViewportSize({width:480,height});
   const bottom=await sheet.locator('.buttons').evaluate(el=>el.getBoundingClientRect().bottom);
   assert(height-bottom>=20&&height-bottom<=24,'按鈕下方僅保留標準內距');
   await sheet.getByRole('button',{name:button,exact:true}).click();
   assert.deepEqual(await sheet.evaluate(()=>actions.filter(a=>typeof a==='number')),[action]);
  }
  const secureHTML=await page.evaluate(()=>window.systemShortcutDocument({label:'Ctrl+Alt+Del',secure:true,remoteAvailable:true}));
  await sheet.goto('about:blank');
  await sheet.setContent(secureHTML.replace('<head>','<head><script>window.actions=[];window.webkit={messageHandlers:{titlebar:{postMessage:a=>actions.push(a)}}};<\/script>'));
  assert(!(await sheet.getByRole('button',{name:'遠端',exact:true}).isDisabled()));
  assert(await sheet.getByRole('button',{name:'本機',exact:true}).isDisabled());
  await sheet.keyboard.press('Escape');assert.deepEqual(await sheet.evaluate(()=>actions.filter(a=>typeof a==='number')),[42]);
  await sheet.close();
  assert.deepEqual(errors,[]);
  console.log('PASS: 本機／遠端／取消、預設遠端、不可用時預設取消、Escape、焦點圈選、失焦取消、安全快捷鍵限制、C/V 不攔截與四語系窄視窗');
 } finally { await browser.close() }
})().catch(e=>{console.error(e);process.exitCode=1});
