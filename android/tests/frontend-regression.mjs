// Run: node android/tests/frontend-regression.mjs
// BROWSER may point to a Chromium/Edge binary. Uses only an isolated temp profile,
// local repository assets and a mocked Native bridge; never contacts a Host.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import {fileURLToPath, pathToFileURL} from 'node:url';
import {spawn} from 'node:child_process';

const assets = fileURLToPath(new URL('../app/src/main/assets/', import.meta.url));
const candidates = [process.env.BROWSER,
  '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge',
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/usr/bin/chromium', '/usr/bin/chromium-browser', '/usr/bin/google-chrome'].filter(Boolean);
let executable;
for (const candidate of candidates) { try { await fs.access(candidate); executable = candidate; break; } catch {} }
if (!executable) throw new Error('Set BROWSER to an installed Chromium or Edge executable.');
const profile = await fs.mkdtemp(path.join(os.tmpdir(), 'yourdesk-android-input-test-'));
const browser = spawn(executable, ['--headless=new', '--disable-gpu', '--no-first-run',
  '--no-default-browser-check', '--disable-background-networking', '--remote-debugging-port=0',
  `--user-data-dir=${profile}`, 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe']});
let stderr = '';
browser.stderr.on('data', data => { stderr = (stderr + data).slice(-4096); });
const browserExited = new Promise(resolve => browser.once('exit', resolve));
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
let ws, seq = 0, passed = 0;
const pending = new Map();
const errors = [];
function call(method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = ++seq;
    const timer = setTimeout(() => { pending.delete(id); reject(new Error(`CDP timeout: ${method}`)); }, 10000);
    pending.set(id, {resolve: result => { clearTimeout(timer); resolve(result); }, reject: error => { clearTimeout(timer); reject(error); }});
    ws.send(JSON.stringify({id, method, params}));
  });
}
async function evaluate(expression) {
  const result = await call('Runtime.evaluate', {expression, returnByValue: true, awaitPromise: true});
  if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails));
  return result.result.value;
}
async function navigate(page, ready) {
  await call('Page.navigate', {url: pathToFileURL(path.join(assets, page)).href});
  for (let i = 0; i < 100; i++) {
    try { if (await evaluate(ready)) return; } catch {}
    await delay(30);
  }
  throw new Error(`Page not ready: ${page}`);
}
async function key(key, code, vk, modifiers = 0, text) {
  await call('Input.dispatchKeyEvent', {type: 'keyDown', key, code, windowsVirtualKeyCode: vk, modifiers,
    ...(text === undefined ? {} : {text})});
  await call('Input.dispatchKeyEvent', {type: 'keyUp', key, code, windowsVirtualKeyCode: vk, modifiers});
  await delay(40);
}
async function shellText() {
  await evaluate('writes');
  const chunks = await evaluate('sent');
  return Buffer.concat(chunks.map(value => Buffer.from(value, 'base64'))).toString('utf8');
}
async function compose(text, intermediate = '中') {
  await call('Input.imeSetComposition', {text: intermediate, selectionStart: intermediate.length, selectionEnd: intermediate.length});
  await call('Input.imeSetComposition', {text, selectionStart: text.length, selectionEnd: text.length});
}
function pass(name) { passed++; console.log(`PASS ${name}`); }

try {
  let port;
  for (let i = 0; i < 200; i++) {
    try { port = (await fs.readFile(path.join(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0]; break; } catch {}
    if (browser.exitCode !== null) throw new Error(`Browser exited: ${stderr}`);
    await delay(25);
  }
  if (!port) throw new Error(`Browser startup timeout: ${stderr}`);
  const pages = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  ws = new WebSocket(pages.find(page => page.type === 'page').webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { ws.addEventListener('open', resolve, {once: true}); ws.addEventListener('error', reject, {once: true}); });
  ws.addEventListener('message', ({data}) => {
    const message = JSON.parse(data), waiter = pending.get(message.id);
    if (waiter) { pending.delete(message.id); message.error ? waiter.reject(message.error) : waiter.resolve(message.result); }
    if (message.method === 'Runtime.exceptionThrown') errors.push(message.params.exceptionDetails);
  });
  await call('Page.enable');
  await call('Runtime.enable');
  await call('Page.addScriptToEvaluateOnNewDocument', {source: `
    window.sent=[];window.controls=[];window.connections=[];window.textSupported=true;
    window.YourDesk={showKeyboard(){},toggleKeyboard(){},closeTerminal(){},desktopLayout(){},
      request(raw){const message=JSON.parse(raw);if(message.method==='write')sent.push(message.data);
        setTimeout(()=>window.shellReply?.({id:message.id,result:message.method==='read'?{}:null}),0);},
      sendControlJSON(raw){controls.push(JSON.parse(raw));return 'ok';},
      supportsTextInput(){return window.textSupported;},
      remembered(){return '{}';},rememberCredentials(){return true;},connectSession(raw){connections.push(JSON.parse(raw));},
      loadSites(){return JSON.stringify([{id:'fixture-peer',name:'Fixture',note:'',signal:'wss://custom.example/ws',terminal:true,desktop:true}]);},
      saveSites(){return true;},stopQrScanner(){},startQrScanner(){},requestCameraPermission(){},setSiteDialogVisible(){},abortConnection(){}
    };`});

  await navigate('terminal.html', "document.activeElement?.id==='input-proxy' && document.getElementById('status')?.textContent==='Shell 已連線'");
  assert.equal(await evaluate("document.querySelectorAll('#input-proxy').length"), 1);
  assert.equal(await evaluate('document.activeElement===term.textarea'), true);
  await key('a', 'KeyA', 65, 0, 'a');
  await key('Enter', 'Enter', 13, 0, '\r');
  await key('Backspace', 'Backspace', 8);
  await key('c', 'KeyC', 67, 2);
  assert.equal(await shellText(), 'a\r\x7f\x03');
  pass('Shell physical typing, Enter, Backspace and Ctrl+C exactly once');

  await evaluate('sent=[]');
  await compose('中文');
  assert.equal(await shellText(), '');
  await call('Input.insertText', {text: '中文'});
  await delay(80);
  assert.equal(await shellText(), '中文');
  pass('Shell Chinese composition commits once, without preedit leakage');

  await evaluate('sent=[]');
  const paste = '中文😀abc'.repeat(1500);
  await call('Input.insertText', {text: paste});
  await delay(80);
  assert.equal(await shellText(), paste);
  assert.equal(await evaluate('sent.every(value=>atob(value).length<=2048)'), true);
  pass('Shell large UTF-8 paste preserves all bytes with bounded writes');

  await navigate('desktop.html', "typeof window.desktopFrameStatus==='function'");
  await evaluate("document.getElementById('keyboard-proxy').focus()");
  await call('Input.insertText', {text: '中文😀'});
  assert.deepEqual(await evaluate('controls'), [{type: 'text', text: '中文😀'}]);
  pass('Desktop direct IME text reaches the Unicode control protocol');

  await evaluate('controls=[]');
  await key('a', 'KeyA', 65, 0, 'a');
  assert.deepEqual(await evaluate('controls'), [{type: 'text', text: 'a'}]);
  pass('Desktop physical character does not send both key and text');

  await evaluate('controls=[]');
  await compose('中文');
  assert.deepEqual(await evaluate('controls'), []);
  await call('Input.insertText', {text: '中文'});
  await delay(40);
  assert.deepEqual(await evaluate('controls'), [{type: 'text', text: '中文'}]);
  pass('Desktop composition sends only one final commit');

  await evaluate(`controls=[];(()=>{const proxy=document.getElementById('keyboard-proxy');
    proxy.dispatchEvent(new CompositionEvent('compositionstart',{data:''}));proxy.value='測試';
    proxy.dispatchEvent(new InputEvent('input',{data:'測試',isComposing:true,inputType:'insertCompositionText'}));
    proxy.dispatchEvent(new CompositionEvent('compositionend',{data:'測試'}));
    proxy.dispatchEvent(new InputEvent('input',{data:'測試',inputType:'insertFromComposition'}));})();`);
  await delay(40);
  assert.deepEqual(await evaluate('controls'), [{type: 'text', text: '測試'}]);
  pass('Desktop trailing input after compositionend is not double-sent');

  await evaluate('controls=[]');
  await compose('取消');
  await call('Input.imeSetComposition', {text: '', selectionStart: 0, selectionEnd: 0});
  await delay(40);
  assert.deepEqual(await evaluate('controls'), []);
  pass('Desktop canceled composition does not leak preedit text');

  await evaluate('controls=[]');
  await key('Enter', 'Enter', 13, 0, '\r');
  await key('Backspace', 'Backspace', 8);
  await key('c', 'KeyC', 67, 2);
  assert.deepEqual(await evaluate('controls'), ['enter', 'backspace', 'c'].flatMap(key => [
    {type: 'key', key, down: true}, {type: 'key', key, down: false}]));
  pass('Desktop special keys are normalized and are not duplicated by beforeinput');

  await evaluate(`controls=[];document.getElementById('keyboard-proxy').dispatchEvent(new InputEvent('beforeinput',{inputType:'deleteContentBackward',cancelable:true}));`);
  assert.deepEqual(await evaluate('controls'), [{type: 'key', key: 'backspace', down: true}, {type: 'key', key: 'backspace', down: false}]);
  pass('Desktop Android-style IME delete works without a keydown');

  await evaluate('controls=[]');
  await call('Input.dispatchKeyEvent', {type:'keyDown',key:'Control',code:'ControlLeft',windowsVirtualKeyCode:17,modifiers:2});
  await key('c', 'KeyC', 67, 2);
  await evaluate("window.dispatchEvent(new Event('blur'))");
  assert.deepEqual(await evaluate('controls'), [
    {type:'key',key:'control',down:true},{type:'key',key:'c',down:true},
    {type:'key',key:'c',down:false},{type:'key',key:'control',down:false}]);
  await call('Input.dispatchKeyEvent', {type:'keyUp',key:'Control',code:'ControlLeft',windowsVirtualKeyCode:17});
  pass('Desktop shortcut modifiers are released on focus loss');

  await evaluate('controls=[]');
  await call('Input.insertText', {text: paste});
  const messages = await evaluate('controls');
  assert.equal(messages.map(message => message.text).join(''), paste);
  assert.ok(messages.length > 1);
  assert.ok(messages.every(message => message.type === 'text' && Buffer.byteLength(message.text, 'utf8') <= 16384 && !message.text.includes('\ufffd')));
  pass('Desktop large Unicode paste uses valid <=16 KiB chunks');

  await evaluate('controls=[];textSupported=false;window.desktopInputCapabilities(false)');
  await key('a', 'KeyA', 65, 0, 'a');
  assert.deepEqual(await evaluate('controls'), [{type:'key',key:'a',down:true},{type:'key',key:'a',down:false}]);
  await evaluate('controls=[]');
  await call('Input.insertText', {text:'Ab 1'});
  assert.deepEqual(await evaluate('controls'), [
    {type:'key',key:'shift',down:true},{type:'key',key:'a',down:true},{type:'key',key:'a',down:false},{type:'key',key:'shift',down:false},
    ...['b','space','1'].flatMap(key=>[{type:'key',key,down:true},{type:'key',key,down:false}])]);
  pass('Legacy Host retains physical and IME ASCII key input');

  await evaluate('controls=[]');
  await call('Input.insertText', {text:'中文'});
  assert.deepEqual(await evaluate('controls'), []);
  assert.match(await evaluate("document.getElementById('status').textContent"), /請更新 Host/);
  await evaluate('textSupported=true;window.desktopInputCapabilities(true)');
  await call('Input.insertText', {text:'中文'});
  assert.deepEqual(await evaluate('controls'), [{type:'text',text:'中文'}]);
  pass('Legacy Host gets an explicit upgrade notice; late capability enables Unicode');

  await navigate('index.html', "typeof openConnection==='function'");
  await evaluate(`document.querySelector('#sites .terminal').click();document.getElementById('connect-secret').value='fixture-only';document.getElementById('connection-form').dispatchEvent(new Event('submit',{cancelable:true,bubbles:true}));`);
  assert.equal((await evaluate('connections[0]')).signal, 'wss://custom.example/ws');
  await evaluate(`window.connectionFailed();document.getElementById('connect-room').value='another-peer';document.getElementById('connect-secret').value='fixture-only';document.getElementById('connection-form').dispatchEvent(new Event('submit',{cancelable:true,bubbles:true}));`);
  assert.equal((await evaluate('connections[1]')).signal, '');
  pass('Selected site signaling survives payload and does not leak into a different room');

  await evaluate(`window.connectionFailed();openConnection('quick-peer','desktop');document.getElementById('connect-secret').value='fixture-only';document.getElementById('connection-form').dispatchEvent(new Event('submit',{cancelable:true,bubbles:true}));`);
  assert.equal((await evaluate('connections[2]')).signal, '');
  pass('Quick connection uses the native default signaling');
  assert.deepEqual(errors, []);
  console.log(`${passed} browser regression cases passed; real Android IMEs and native injection still require device validation.`);
} finally {
  if (ws?.readyState === WebSocket.OPEN) {
    try { await call('Browser.close'); } catch {}
    ws.close();
  }
  if (browser.exitCode === null) browser.kill('SIGTERM');
  await Promise.race([browserExited, delay(2000)]);
  if (browser.exitCode === null) { browser.kill('SIGKILL'); await browserExited; }
  await fs.rm(profile, {recursive: true, force: true});
}
