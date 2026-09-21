// Render the real Client UI against isolated, in-memory demonstration data.
// No live server, credentials, remote connections or user preferences are used.
const fs = require('fs');
const path = require('path');
const assert = require('assert/strict');
const {execFileSync} = require('child_process');
const {chromium} = require('playwright');
const root = path.resolve(__dirname, '..');
const web = path.join(root, 'internal/clientui/web');
const origin = 'https://yourdesk-demo.test';
const output = path.join(root, 'images/yourdesk-demo.gif');

async function main() {
  // Never silently replace an existing asset: keep it until a new render passes.
  if (fs.existsSync(output)) fs.copyFileSync(output, output + '.bak', fs.constants.COPYFILE_EXCL);
  const cache = path.join(root, '.local-run');
  fs.mkdirSync(cache, {recursive:true});
  const folder = fs.mkdtempSync(path.join(cache, 'readme-demo-'));
  const state = {
    info:{room:'YD-DEMO-0000-0000-0000-0000',secret:'',hostname:'Demo Mac',platform:'darwin',architecture:'arm64',version:'YourDesk · Demo',configPath:'demo/config'},
    running:{host:true}, sessions:{}, quick:null, updates:{}, hardwareDetection:{status:'complete'},
    preferences:{language:'zh-Hant',theme:'light',selectedGroup:'*',disableHints:true},
    library:{groups:[{id:'work',name:'工作'},{id:'home',name:'家中'}],sites:[
      {id:'office',name:'Office Windows',room:'YD-DEMO-1111-1111-1111-1111',group:'work',note:'辦公室工作站'},
      {id:'studio',name:'Studio Mac',room:'YD-DEMO-2222-2222-2222-2222',group:'work',note:'設計與創作'},
      {id:'home',name:'Home PC',room:'YD-DEMO-3333-3333-3333-3333',group:'home',note:'家中電腦'},
    ]},
  };
  const browser = await chromium.launch({headless:true,...(process.env.DEMO_BROWSER_CHANNEL ? {channel:process.env.DEMO_BROWSER_CHANNEL} : {})});
  const errors = [], unexpected = [], frames = [];
  try {
    const page = await browser.newPage({viewport:{width:1100,height:760},deviceScaleFactor:1,locale:'zh-TW',colorScheme:'light',serviceWorkers:'block'});
    page.on('pageerror', error => errors.push(error.message));
    await page.route('**/*', async route => {
      const request = route.request(), url = new URL(request.url());
      if (url.origin !== origin) { unexpected.push(request.url()); return route.abort(); }
      if (url.pathname.startsWith('/api/')) {
        const api = url.pathname.slice(5), method = request.method();
        let value;
        if (api === 'state' && method === 'GET') value = state;
        else if (api === 'presence' && method === 'GET') value = Object.fromEntries(state.library.sites.map(site => [site.id,{online:true,capabilities:{desktop:true,terminal:true}}]));
        else if (api === 'preferences' && method === 'PUT') value = state.preferences = {...state.preferences,...request.postDataJSON()};
        else if (api === 'library' && method === 'PUT') value = state.library = request.postDataJSON();
        else if (api === 'remembered' && method === 'POST') value = {remembered:false};
        else if (api === 'updates/state' && method === 'GET') value = {};
        else if (api === 'network-debug' && method === 'GET') value = {enabled:false};
        else { unexpected.push(method + ' ' + api); return route.fulfill({status:400,json:{error:'Demo endpoint blocked'}}); }
        return route.fulfill({json:structuredClone(value)});
      }
      const name = url.pathname === '/' ? 'index.html' : url.pathname.slice(1);
      if (!['index.html','style.css','app.js','i18n.js','tooltips.js','diagnostics.js','packet-test.js','site-qr.js','terminal.js','translations.json','app-icon.png','xterm.js','xterm.css'].includes(name)) {
        unexpected.push(name); return route.abort();
      }
      const type = {'.html':'text/html','.css':'text/css','.js':'text/javascript','.json':'application/json','.png':'image/png'}[path.extname(name)];
      return route.fulfill({body:fs.readFileSync(path.join(web,name)),contentType:type});
    });
    await page.goto(origin);
    await page.locator('.site-card').nth(2).waitFor();
    await page.evaluate(async () => { await document.fonts.ready; });
    await page.evaluate(() => {
      const caption = document.createElement('div'); caption.id='demo-caption'; caption.popover='manual';
      caption.style.cssText='position:fixed;inset:auto 24px 18px;margin:0;width:auto;border:1px solid #334155;border-radius:12px;background:#142338;color:white;padding:13px 18px;font:600 16px -apple-system,sans-serif;pointer-events:none;box-shadow:0 5px 20px #0002;display:flex;justify-content:space-between;align-items:center';
      caption.innerHTML='<span></span><small style="font-size:12px;font-weight:400;color:#bdcddd">示範資料 · 未連線遠端</small>';
      document.body.append(caption); caption.showPopover();
      const cursor=document.createElement('div');cursor.id='demo-cursor';cursor.popover='manual';
      cursor.style.cssText='position:fixed;inset:682px auto auto 1032px;margin:0;border:2px solid white;border-radius:50%;width:16px;height:16px;padding:0;background:#2563eb;box-shadow:0 0 0 4px #2563eb40;pointer-events:none';
      document.body.append(cursor);cursor.showPopover();
    });
    let point={x:1040,y:690};
    async function frame(duration=100) {
      const name=String(frames.length).padStart(4,'0')+'.png';
      await page.screenshot({path:path.join(folder,name)});
      frames.push({name,duration});
    }
    async function caption(text) {
      await page.evaluate(text => {
        const node=document.querySelector('#demo-caption');node.querySelector('span').textContent=text;
        node.hidePopover();node.showPopover();
        const cursor=document.querySelector('#demo-cursor');cursor.hidePopover();cursor.showPopover();
      },text);
    }
    async function move(selector) {
      const box=await page.locator(selector).boundingBox();assert(box,selector);
      const dest={x:box.x+box.width/2,y:box.y+box.height/2}, from=point;
      for(let i=1;i<=5;i++) {
        point={x:from.x+(dest.x-from.x)*i/5,y:from.y+(dest.y-from.y)*i/5};
        await page.mouse.move(point.x,point.y);
        await page.evaluate(p=>{const n=document.querySelector('#demo-cursor');n.style.left=(p.x-8)+'px';n.style.top=(p.y-8)+'px';},point);
        await frame(60);
      }
    }
    async function click(selector) {await move(selector);await page.locator(selector).click();await page.waitForTimeout(120);}
    async function type(selector,value) {
      await click(selector);
      for(let i=1;i<=value.length;i++){await page.locator(selector).fill(value.slice(0,i));await frame(90);}
    }
    await caption('YourDesk　讓常用的遠端電腦，一目瞭然');await frame(1600);
    await caption('01　依群組整理工作與家中的電腦');
    await click('[data-group-id="work"] .group-button');assert.equal(await page.locator('.site-card').count(),2);await frame(1300);
    await click('[data-group-id="home"] .group-button');assert.equal(await page.locator('.site-card').count(),1);await frame(1000);
    await click('.brand');
    await caption('02　輸入名稱，快速找到站台');await type('#search','Studio');
    assert.equal(await page.locator('.site-card').count(),1);await frame(1300);await page.locator('#search').fill('');
    await caption('03　儲存常用站台，下次不必重新輸入');await click('#add-site');await caption('03　儲存常用站台，下次不必重新輸入');await frame(600);
    await type('#site-form [name="name"]','Demo Laptop');
    await type('#site-form [name="room"]','YD-DEMO-4444-4444-4444-4444');
    await page.locator('#site-form [name="group"]').selectOption('work');await frame(800);
    await click('#site-form button[type="submit"]');
    await page.locator('.site-card').nth(3).waitFor();await frame(1400);
    await caption('04　明亮或暗調，依習慣切換');await click('#open-settings');await caption('04　明亮或暗調，依習慣切換');
    await move('#ui-theme');await page.locator('#ui-theme').selectOption('dark');await page.waitForTimeout(150);await frame(1200);
    assert.equal(await page.locator('html').getAttribute('data-theme'),'dark');
    await click('#settings-dialog .close');await frame(1000);
    await click('#open-settings');await page.locator('#ui-theme').selectOption('light');await click('#settings-dialog .close');
    await caption('05　可輸入遠端 ID，或從已儲存的站台連線');
    await type('#quick-room','YD-DEMO-1111-1111-1111-1111');await frame(900);
    await page.locator('#quick-room').fill('');
    await click('[data-site-id="office"] [data-connect-mode="desktop"]');
    await page.locator('#connect-dialog[open]').waitFor();await caption('輸入被控端提供的密碼，即可繼續連線');await frame(1800);
    await click('#connect-dialog .modal-actions button.close');
    await caption('YourDesk　站台管理與快速連線');await frame(1400);
    assert.deepEqual(errors,[]);assert.deepEqual(unexpected,[]);
    fs.writeFileSync(path.join(folder,'frames.json'),JSON.stringify(frames));
    execFileSync(process.env.PYTHON || 'python3',[path.join(__dirname,'encode-readme-demo.py'),folder,output],{stdio:'inherit'});
    console.log('Frames:',path.relative(root,folder));
    console.log('GIF:',path.relative(root,output));
  } finally {await browser.close();}
}
main().catch(error=>{console.error(error);process.exitCode=1;});
