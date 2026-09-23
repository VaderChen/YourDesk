'use strict';
// Real headless browser, isolated mock server/bindings. This does not test OS drag.
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const {chromium} = require('playwright');
const root = path.resolve(__dirname,'..'), web = path.join(root,'internal/clientui/web');
const origin = 'https://yourdesk-files.test';

async function main() {
  const cache = path.join(root,'.local-run'); fs.mkdirSync(cache,{recursive:true});
  const output = process.env.FILES_SMOKE_OUTPUT ? path.resolve(root,process.env.FILES_SMOKE_OUTPUT) : fs.mkdtempSync(path.join(cache,'files-ui-smoke-'));
  assert.ok(output.startsWith(cache + path.sep),'Screenshots must stay in the local build area'); fs.mkdirSync(output,{recursive:true});
  const browser = await chromium.launch({headless:true,...(process.env.FILES_BROWSER_CHANNEL?{channel:process.env.FILES_BROWSER_CHANNEL}:{})});
  const errors = [], unexpected = [], actions = [], uploads = new Map(); let fileConnected = true, currentInstance='demo-instance', holdWrites=false,holdLists=false,holdCancels=false,loseCancelAck=false;
  const heldWrites=[],heldLists=[],heldCancels=[];let filesystem=null;
  const state = {
    info:{room:'YD-DEMO-0000-0000-0000-0000',secret:'',hostname:'Demo Mac',platform:'darwin',architecture:'arm64',version:'YourDesk · Demo',configPath:'demo/config'},
    running:{host:true},sessions:{},quick:null,updates:{},hardwareDetection:{status:'complete'},
    preferences:{language:'zh-Hant',theme:'light',selectedGroup:'*',disableHints:true},
    library:{groups:[],sites:[
      {id:'files',name:'個人電腦',room:'YD-DEMO-1111-1111-1111-1111',note:'支援檔案傳輸'},
      {id:'older',name:'舊版電腦',room:'YD-DEMO-2222-2222-2222-2222',note:'尚未支援檔案傳輸'}
    ]}
  };
  const entry = (name,directory=false,parent='') => ({name,path:parent ? `${parent}/${name}` : name,directory,size:directory ? 0 : 12,modified:'2026-01-01T12:00:00Z'});
  const remoteEntries=new Map([entry('z-notes.txt'),entry('zeta',true),entry('.hidden.txt'),entry('.private',true),entry('Alpha',true),entry('資料',true),entry('子檔案.txt',false,'資料'),entry('示範檔案.txt'),entry('很長的 Unicode 檔名 — '.repeat(3)+'🗂.txt')].map(item=>[item.path,item]));
  try {
    const context = await browser.newContext({viewport:{width:820,height:640},deviceScaleFactor:1,locale:'zh-TW',colorScheme:'light',serviceWorkers:'block'});
    context.on('page',page => page.on('pageerror',error => errors.push(error.message)));
    await context.route('**/*',async route => {
      const request = route.request(), url = new URL(request.url());
      if (url.origin !== origin) {unexpected.push(url.origin); return route.abort();}
      if (url.pathname.startsWith('/api/')) {
        const api = url.pathname.slice(5), method = request.method(), body = method === 'POST' ? request.postDataJSON() : null;
        actions.push({api,method,body}); let value;
        if (api === 'state') value = state;
        else if (api === 'presence') value = {files:{online:true,capabilities:{desktop:true,terminal:true,files:true}},older:{online:true,capabilities:{desktop:true,terminal:true,files:false}}};
        else if (api === 'remembered') value = {remembered:true};
        else if (api === 'viewer/start') {state.running[`viewer:${body.id}`] = true; state.sessions[`viewer:${body.id}`] = {stage:'connected'}; value = {ok:true};}
        else if (api === 'files/window') value = {ok:true};
        else if (api === 'updates/state') value = {};
        else if (api === 'network-debug') value = {enabled:false};
        else if (api === 'files') {
          assert.equal(request.headers()['x-yourdesk-token'],'demo-file-token');
          assert.equal(body.session,'viewer:files'); assert.equal(body.instance,currentInstance);
          const {action,params} = body;
          if (action === 'location') return filesystem?route.fulfill({json:{path:filesystem.initial}}):route.fulfill({status:400,json:{error:'old host'}});
          if(action==='list'&&filesystem){
            const item=filesystem.directories[params.path];assert(item,'未知磁碟目錄：'+params.path);return route.fulfill({json:{path:params.path,nextOffset:-1,...item}});
          }
          if (action === 'status') value = {connected:fileConnected};
          else if (action === 'list') {
            if(holdLists){await new Promise(resolve=>heldLists.push(resolve));return route.fulfill({status:400,json:{code:'files_session_unavailable'}});}
            value = {path:params.path,entries:[...remoteEntries.values()].filter(item=>(item.path.includes('/')?item.path.slice(0,item.path.lastIndexOf('/')):'')===params.path),nextOffset:-1};
          }
          else if (action === 'begin') {assert.match(params.token,/^[a-f0-9]{64}$/);const id=params.token.slice(0,32);if(!uploads.has(id))uploads.set(id,{path:params.path,token:params.token,size:params.size,data:[]});const upload=uploads.get(id);value={id,resumeToken:params.token,state:upload.committed?'complete':'uploading',nextOffset:upload.data.length};}
          else if (action === 'resume') {const upload=uploads.get(params.id);assert.equal(upload.token,params.token);assert.equal(upload.path,params.path);assert.equal(upload.size,params.size);value={state:upload.committed?'complete':'uploading',nextOffset:upload.data.length};}
          else if (action === 'write') {const upload = uploads.get(params.id), chunk = Buffer.from(params.data,'base64'); assert.equal(params.offset,upload.data.length); assert.ok(chunk.length <= 4096); upload.data.push(...chunk); value = {nextOffset:upload.data.length};if(holdWrites)await new Promise(resolve=>heldWrites.push(resolve));}
          else if (action === 'commit') {const upload = uploads.get(params.id); assert.equal(upload.data.length,upload.size); upload.committed = true; remoteEntries.set(upload.path,{...entry(upload.path),size:upload.size});value = {ok:true};}
          else if (action === 'cancel') {
            const upload=uploads.get(params.id);if(upload){assert.equal(upload.token,params.token);assert.equal(upload.path,params.path);assert.equal(upload.size,params.size);}
            const complete=!!upload?.committed;if(!complete)uploads.delete(params.id);value={ok:true,state:complete?'complete':'cancelled'};
            if(loseCancelAck){loseCancelAck=false;return route.abort('failed');}
            if(holdCancels)await new Promise(resolve=>heldCancels.push(resolve));
          }
          else if (action === 'mkdir') {assert.equal(remoteEntries.has(params.path),false);remoteEntries.set(params.path,entry(params.path,true));value={ok:true};}
          else if (action === 'remove') {const target=remoteEntries.get(params.path);assert.equal(params.confirm,target.name);assert.equal(params.directory,target.directory);assert.equal(params.modified,target.modified);assert.equal(params.size,target.size);let removedCount=0;for(const key of remoteEntries.keys())if(key===params.path||(params.recursive&&key.startsWith(params.path+'/'))){remoteEntries.delete(key);removedCount++;}value={ok:true,removedCount};}
        }
        if (value === undefined) {unexpected.push(method + ' ' + api); return route.fulfill({status:400,json:{error:'Unexpected mock endpoint'}});}
        return route.fulfill({json:structuredClone(value)});
      }
      const name = url.pathname === '/' ? 'index.html' : url.pathname.slice(1), target = path.resolve(web,name);
      if (!target.startsWith(web + path.sep) || !fs.existsSync(target) || !fs.statSync(target).isFile()) {unexpected.push(name); return route.abort();}
      const type = {'.html':'text/html','.css':'text/css','.js':'text/javascript','.json':'application/json','.png':'image/png','.woff2':'font/woff2'}[path.extname(target)];
      if (!type) {unexpected.push(name); return route.abort();}
      return route.fulfill({body:fs.readFileSync(target),contentType:type});
    });
    const page = await context.newPage(); await page.goto(origin + '/#demo-main-token');
    await page.locator('[data-site-id="files"] [data-connect-mode="files"].mode-available').waitFor();
    const buttons = await Promise.all(['terminal','files','desktop'].map(mode => page.locator(`[data-site-id="files"] [data-connect-mode="${mode}"]`).boundingBox()));
    assert.ok(buttons.every(Boolean)); assert.ok(buttons[0].x < buttons[1].x && buttons[1].x < buttons[2].x,'file icon must be between terminal and desktop');
    assert.equal(await page.locator('[data-site-id="older"] [data-connect-mode="files"]').isDisabled(),true);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),true,'main page overflow');
    await page.screenshot({path:path.join(output,'main-820x640.png')});
    await page.locator('[data-site-id="files"] [data-connect-mode="files"]').click();
    await page.waitForFunction(() => !document.querySelector('#connection-progress-dialog').open);
    for (let i=0;i<30 && !actions.some(item => item.api === 'files/window');i++) await page.waitForTimeout(100);
    assert.ok(actions.some(item => item.api === 'viewer/start' && item.body.files === true && item.body.terminal === false));
    assert.ok(actions.some(item => item.api === 'files/window' && item.body.session === 'viewer:files'));

    const filesPage = await context.newPage(); await filesPage.setViewportSize({width:700,height:560});
    await filesPage.addInitScript(() => {
      window.__cancelCount = 0;
      window.yourdeskCancelFile = async () => {window.__cancelCount++;};
      window.__closedCount=0;window.yourdeskCloseFiles = async () => {window.__closedCount++;};
      window.yourdeskPauseFile=async()=>({paused:true,received:4,total:12});
      window.yourdeskResumeFile=async()=>{window.dispatchEvent(new CustomEvent('yourdesk-file-progress',{detail:{state:'running',received:8,total:12,speed:40,eta:1}}));setTimeout(()=>window.__downloadFinish(),40);return{resumed:true};};
      window.__downloadRequests=[];
      window.yourdeskListDirectories=async(path)=>{const current=path||'/Users/Smoke';return {path:current,parent:current==='/'?'':current.split('/').slice(0,-1).join('/')||'/',entries:current==='/Users/Smoke'?[{name:'Downloads',path:'/Users/Smoke/Downloads'}]:[],nextOffset:-1}};
      window.yourdeskPrepareFile = (path,directory) => new Promise(resolve=>{
        window.__downloadRequests.push({path,directory});
        window.__downloadFinish=()=>resolve({name:path.split('/').pop(),size:12,path:directory+'/'+path.split('/').pop()});
        window.dispatchEvent(new CustomEvent('yourdesk-file-progress',{detail:{state:'running',received:4,total:12,name:path.split('/').pop(),speed:40,eta:1}}));
      });
    });
    const fileURL = language => `${origin}/files.html#token=demo-file-token&session=viewer%3Afiles&instance=${currentInstance}&name=Demo&language=${language}`;
    for (const language of ['zh-Hant','en','ja','ko']) {
      await filesPage.goto('about:blank');
      await filesPage.goto(fileURL(language)); await filesPage.locator('#files-list tr').nth(2).waitFor();
      assert.equal(new URL(filesPage.url()).hash,'');
      assert.equal(await filesPage.locator('html').getAttribute('lang'),language);
      assert.equal(await filesPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth),true,`${language} file window overflow`);
      assert.equal(await filesPage.locator('.native-space,.download,.reconnect-hint,[data-text=downloadHint]').count(),0,'移除底部下載區塊與說明');
      assert(!(await filesPage.locator('#download-transfer').isVisible()),'閒置時不顯示下載進度');
      assert.equal(await filesPage.locator('#files-list tr[data-parent] button').isDisabled(),true);
      await filesPage.screenshot({path:path.join(output,`files-${language}-700x560.png`)});
    }
    await filesPage.goto('about:blank'); await filesPage.goto(fileURL('zh-Hant')); await filesPage.locator('#files-list tr').nth(2).waitFor();
    const names=await filesPage.locator('#files-list .entry-name').allTextContents();
    assert.deepEqual(names.slice(0,3),['Alpha','zeta','資料']);assert(!names.some(name=>name.startsWith('.')));
    await filesPage.evaluate(()=>document.fonts.ready);
    assert.equal(await filesPage.evaluate(()=>document.fonts.check('900 18px "YourDesk File Icons"')),true);
    assert.equal(await filesPage.locator('.entry-icon-folder').count(),3);
    assert.equal(await filesPage.locator('.entry-icon-file').first().evaluate(el=>getComputedStyle(el,'::before').content),'"\uf15b"');
    await filesPage.getByRole('button',{name:'z-notes.txt',exact:true}).click();
    await filesPage.getByRole('button',{name:'示範檔案.txt',exact:true}).click({modifiers:['ControlOrMeta']});
    assert.equal(await filesPage.locator('#files-list .entry-check:checked').count(),2);
    await filesPage.screenshot({path:path.join(output,'files-multiple-selection.png')});
    await filesPage.emulateMedia({colorScheme:'dark'});await filesPage.screenshot({path:path.join(output,'files-multiple-selection-dark.png')});await filesPage.emulateMedia({colorScheme:'light'});
    const chooserPromise=filesPage.waitForEvent('filechooser');await filesPage.locator('#files-input').click();
    const chooser=await chooserPromise;assert(chooser.isMultiple());
    await chooser.setFiles([{name:'picker-one.txt',mimeType:'text/plain',buffer:Buffer.from('one')},{name:'picker-two.txt',mimeType:'text/plain',buffer:Buffer.from('two')}]);
    await filesPage.waitForFunction(()=>document.querySelectorAll('#files-queue .transfer-state').length===2&&[...document.querySelectorAll('#files-queue .transfer-state')].every(el=>el.textContent.startsWith('完成')));
    await filesPage.getByRole('button',{name:'示範檔案.txt',exact:true}).click();
    await filesPage.locator('#files-prepare').click();await filesPage.locator('#destination-select').waitFor({state:'visible'});
    await filesPage.waitForFunction(()=>!document.querySelector('#destination-select').disabled);
    assert.equal(await filesPage.locator('#destination-path').inputValue(),'/Users/Smoke');assert.equal(await filesPage.evaluate(()=>window.__downloadRequests.length),0);
    await filesPage.locator('#destination-cancel').click();assert.equal(await filesPage.evaluate(()=>window.__downloadRequests.length),0);
    await filesPage.locator('#files-prepare').click();await filesPage.locator('#destination-list').getByRole('button',{name:'Downloads',exact:true}).click();
    await filesPage.waitForFunction(()=>document.querySelector('#destination-path').value==='/Users/Smoke/Downloads');
    await filesPage.screenshot({path:path.join(output,'files-destination.png')});
    await filesPage.locator('#destination-select').click();await filesPage.locator('#download-pause').click();
    assert.deepEqual(await filesPage.evaluate(()=>window.__downloadRequests),[{path:'示範檔案.txt',directory:'/Users/Smoke/Downloads'}]);
    await filesPage.waitForFunction(()=>document.querySelector('#download-status').textContent.includes('已暫停'));
    assert.match(await filesPage.locator('#download-status').textContent(),/33%/);
    await filesPage.locator('#download-resume').click();await filesPage.waitForFunction(() => document.querySelector('#download-status').textContent.startsWith('下載完成'));
    await filesPage.locator('#files-input').setInputFiles({name:'upload-demo.txt',mimeType:'text/plain',buffer:Buffer.from('Isolated upload fixture')});
    await filesPage.waitForFunction(() => document.querySelector('#files-queue .transfer-state span')?.textContent.startsWith('完成'));
    const uploaded=[...uploads.values()].find(item=>item.path==='upload-demo.txt');assert.equal(uploaded.committed,true);assert.equal(Buffer.from(uploaded.data).toString(),'Isolated upload fixture');
    await filesPage.getByRole('button',{name:'資料',exact:true}).dblclick();
    await filesPage.getByRole('button',{name:'子檔案.txt',exact:true}).waitFor();
    await filesPage.locator('#files-list tr[data-parent] button').click(); await filesPage.locator('#files-list tr').nth(2).waitFor();
    assert.equal(await filesPage.locator('#files-delete').isDisabled(),true,'上一層不是可刪除的項目');
    await filesPage.screenshot({path:path.join(output,'files-upload-download.png')});
    await filesPage.locator('#files-new-folder').click();await filesPage.locator('#folder-name').fill('Smoke Folder');await filesPage.locator('#folder-create').click();
    await filesPage.getByRole('button',{name:'Smoke Folder',exact:true}).click();await filesPage.locator('#files-delete').click();
    await filesPage.locator('#delete-name').fill('wrong');await filesPage.locator('#delete-recursive-check').check();await filesPage.locator('#delete-confirm').click();
    assert.match(await filesPage.locator('#delete-error').textContent(),/確認名稱不符/);
    await filesPage.locator('#delete-name').fill('Smoke Folder');await filesPage.locator('#delete-confirm').click();await filesPage.waitForFunction(()=>!document.querySelector('#delete-dialog').open);
    assert.equal(remoteEntries.has('Smoke Folder'),false);
    holdWrites=true;await filesPage.locator('#files-input').setInputFiles({name:'resume-demo.bin',mimeType:'application/octet-stream',buffer:Buffer.alloc(12000,42)});
    for(let i=0;i<100&&!heldWrites.length;i++)await filesPage.waitForTimeout(20);assert.equal(heldWrites.length,1);
    await filesPage.locator('#files-queue [data-transfer-action="pause"]').click();holdWrites=false;heldWrites.shift()();
    await filesPage.waitForFunction(()=>document.querySelector('#files-queue .transfer-state span')?.textContent.startsWith('已暫停'));
    assert.equal(await filesPage.locator('#files-queue li.failed').count(),0,'paused transfer is not a failed transfer');
    assert.match(await filesPage.locator('#files-queue .transfer-state').textContent(),/34%/);
    await filesPage.screenshot({path:path.join(output,'files-paused.png')});
    fileConnected = false;
    await filesPage.waitForFunction(() => document.querySelector('#files-status').textContent.includes('已中斷'));
    assert.equal(await filesPage.locator('#files-input').isDisabled(),true);
    assert.equal(await filesPage.locator('#files-refresh').isDisabled(),true);
    assert.equal(await filesPage.locator('#files-prepare').isDisabled(),true);
    assert.equal(await filesPage.evaluate(() => window.__cancelCount),0,'completed native download must remain available');
    assert.match(await filesPage.locator('#download-status').textContent(),/下載完成/);
    await filesPage.screenshot({path:path.join(output,'files-disconnected.png')});
    currentInstance='demo-instance-2';fileConnected=true;
    const resumesBefore=actions.filter(item=>item.body?.action==='resume').length;
    await filesPage.evaluate(()=>window.dispatchEvent(new CustomEvent('yourdesk-files-session',{detail:{instance:'demo-instance-2'}})));
    await filesPage.waitForTimeout(100);assert.equal(actions.filter(item=>item.body?.action==='resume').length,resumesBefore);
    await filesPage.locator('#files-queue [data-transfer-action="resume"]').click();await filesPage.waitForFunction(()=>document.querySelector('#files-queue .transfer-state span')?.textContent.startsWith('完成'));
    const continued=[...uploads.values()].find(item=>item.path==='resume-demo.bin');assert.equal(continued.data.length,12000);assert.equal(continued.committed,true);
    assert.match(await filesPage.locator('#files-queue .transfer-state').textContent(),/100%/);
    await filesPage.screenshot({path:path.join(output,'files-resumed.png')});

    // A late response from the old connection must not disable the new one.
    holdLists=true;await filesPage.goto('about:blank');await filesPage.goto(fileURL('en'));
    for(let i=0;i<100&&!heldLists.length;i++)await filesPage.waitForTimeout(20);assert.equal(heldLists.length,1);
    currentInstance='demo-instance-3';
    await filesPage.evaluate(instance=>window.dispatchEvent(new CustomEvent('yourdesk-files-session',{detail:{instance}})),currentInstance);
    holdLists=false;heldLists.shift()();
    await filesPage.waitForFunction(()=>!document.querySelector('#files-input').disabled);
    assert.match(await filesPage.locator('#files-status').textContent(),/Reconnected/);
    await filesPage.locator('#files-refresh').click();await filesPage.waitForFunction(()=>!document.querySelector('#files-input').disabled);

    // Cancel ACK loss preserves a retry button and does not block the next job.
    holdWrites=true;loseCancelAck=true;
    await filesPage.locator('#files-input').setInputFiles([
      {name:'cancel-ack-loss.bin',mimeType:'application/octet-stream',buffer:Buffer.alloc(5000,42)},
      {name:'after-cancel.txt',mimeType:'text/plain',buffer:Buffer.from('next')}
    ]);
    for(let i=0;i<100&&!heldWrites.length;i++)await filesPage.waitForTimeout(20);assert.equal(heldWrites.length,1);
    await filesPage.locator('#files-queue li').nth(0).locator('[data-transfer-action="cancel"]').click();
    holdWrites=false;heldWrites.shift()();
    await filesPage.waitForFunction(()=>document.querySelectorAll('#files-queue .transfer-state')[1]?.textContent.startsWith('Done'));
    assert.match(await filesPage.locator('#files-queue li').nth(0).textContent(),/Choose Cancel to retry/);
    const retryCancel=filesPage.locator('#files-queue li').nth(0).locator('[data-transfer-action="cancel"]');assert.equal(await retryCancel.isEnabled(),true);
    await retryCancel.click();await filesPage.waitForFunction(()=>document.querySelector('#files-queue .transfer-state')?.textContent.startsWith('Cancelled'));

    // The native window close request must wait for the cancellation reply.
    holdWrites=true;await filesPage.locator('#files-input').setInputFiles({name:'close-paused.bin',mimeType:'application/octet-stream',buffer:Buffer.alloc(5000,42)});
    for(let i=0;i<100&&!heldWrites.length;i++)await filesPage.waitForTimeout(20);assert.equal(heldWrites.length,1);
    await filesPage.locator('#files-queue [data-transfer-action="pause"]').click();holdWrites=false;heldWrites.shift()();
    await filesPage.waitForFunction(()=>document.querySelector('#files-queue .transfer-state')?.textContent.startsWith('Paused'));
    holdCancels=true;await filesPage.evaluate(()=>window.dispatchEvent(new CustomEvent('yourdesk-request-close')));
    for(let i=0;i<100&&!heldCancels.length;i++)await filesPage.waitForTimeout(20);assert.equal(heldCancels.length,1);
    assert.equal(await filesPage.evaluate(()=>window.__closedCount),0);assert.equal(await filesPage.locator('#files-close').isDisabled(),true);
    holdCancels=false;heldCancels.shift()();await filesPage.waitForFunction(()=>window.__closedCount===1);
    assert.equal([...uploads.values()].some(upload=>upload.path==='close-paused.bin'),false);
    // 新版遠端提供實際磁碟位置；上一層可離開家目錄，虛擬磁碟清單不允許上傳／刪除。
    filesystem={initial:'C',directories:{
      'C':{entries:[entry('Users',true,'C')],location:{display:'C:\\',parent:'',virtual:false}},
      '':{entries:[{...entry('C',true),root:true},{...entry('D',true),root:true}],location:{display:'',parent:null,virtual:true}},
      'D':{entries:[],location:{display:'D:\\',parent:'',virtual:false}}
    }};
    await filesPage.goto('about:blank');await filesPage.goto(fileURL('zh-Hant'));
    await filesPage.waitForFunction(()=>document.querySelector('#files-path').textContent==='C:\\');
    assert(!(await filesPage.locator('#files-up').isDisabled()));
    await filesPage.locator('#files-list tr[data-parent] button').click();await filesPage.waitForFunction(()=>document.querySelector('#files-path').textContent==='遠端電腦');
    assert(await filesPage.locator('#files-input').isDisabled());assert(await filesPage.locator('#files-new-folder').isDisabled());
    await filesPage.getByRole('button',{name:'D',exact:true}).click();assert(await filesPage.locator('#files-delete').isDisabled());
    await filesPage.locator('#files-open').click();await filesPage.waitForFunction(()=>document.querySelector('#files-path').textContent==='D:\\');
    await filesPage.screenshot({path:path.join(output,'files-windows-drive.png')});
    filesystem={initial:'root/Users/demo',directories:{
      'root/Users/demo':{entries:[],location:{display:'~/',parent:'root/Users',virtual:false}},
      'root/Users':{entries:[],location:{display:'/Users',parent:'root',virtual:false}},
      'root':{entries:[],location:{display:'/',parent:null,virtual:false}}
    }};
    await filesPage.goto('about:blank');await filesPage.goto(fileURL('zh-Hant'));
    await filesPage.waitForFunction(()=>document.querySelector('#files-path').textContent==='~/');
    await filesPage.locator('#files-list tr[data-parent] button').click();await filesPage.waitForFunction(()=>document.querySelector('#files-path').textContent==='/Users');
    await filesPage.locator('#files-up').click();await filesPage.waitForFunction(()=>document.querySelector('#files-path').textContent==='/');
    assert(await filesPage.locator('#files-up').isDisabled());
    await filesPage.screenshot({path:path.join(output,'files-mac-root.png')});
    filesystem.directories['']={entries:[{...entry('root',true),root:true,displayName:'/'},{...entry('volume-test',true),root:true,displayName:'My Code:工作'}],location:{display:'',parent:null,virtual:true}};
    filesystem.directories['volume-test']={entries:[entry('作品.txt',false,'volume-test')],location:{display:'/Volumes/My Code:工作',parent:'',virtual:false}};
    await filesPage.locator('#files-disks').click();await filesPage.getByRole('button',{name:'My Code:工作',exact:true}).waitFor();
    await filesPage.getByRole('button',{name:'My Code:工作',exact:true}).click();assert(await filesPage.locator('#files-delete').isDisabled());
    assert(await filesPage.locator('#files-input').isDisabled());
    await filesPage.screenshot({path:path.join(output,'files-mac-disks.png')});
    await filesPage.locator('#files-open').click();await filesPage.getByRole('button',{name:'作品.txt',exact:true}).waitFor();
    assert.equal(await filesPage.locator('#files-path').textContent(),'/Volumes/My Code:工作');
    assert(!(await filesPage.locator('#files-input').isDisabled()));
    await filesPage.locator('#files-up').click();await filesPage.getByRole('button',{name:'My Code:工作',exact:true}).waitFor();

    assert.deepEqual(errors,[]); assert.deepEqual(unexpected,[]);
    console.log('PASS: Browser mock UI smoke, 4 languages, station icon / capability / connection, bounded upload, cancellation ACK loss, stale rebind reply, bounded close lifecycle, native binding mock, parent entry and local destination picker.');
    console.log('Screenshots:',path.relative(root,output));
    console.log('Not tested: real remote sessions, actual local directory bridge and filesystem access.');
  } finally {await browser.close();}
}
main().catch(error => {console.error(error); process.exitCode = 1;});
