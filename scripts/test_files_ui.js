'use strict';
// Mock API / browser tests only; native OS drag-and-drop needs desktop testing.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const files = require('../internal/clientui/web/files-core.js');
const source = ['files-core.js','files-locales.js','files.js'].map(name => fs.readFileSync(path.join(__dirname,'../internal/clientui/web',name),'utf8')).join('\n');
const {webcrypto} = require('node:crypto');
const file = (name, data = 'abc') => Object.assign(new Blob([data]), {name});
const entryFile = value => ({name:value.name,isFile:true,isDirectory:false,file:resolve => resolve(value)});
const entryDir = (name,batches) => ({name,isFile:false,isDirectory:true,createReader:() => {let i = 0; return {readEntries:resolve => resolve(batches[i++] || [])};}});
const drop = entry => ({kind:'file',webkitGetAsEntry:() => entry});
const isCode = code => error => error.code === code;

test('accepts relative Unicode paths and refuses absolute/traversal paths', () => {
  assert.equal(files.join('工作 文件','月報 🗂.txt'),'工作 文件/月報 🗂.txt');
  for (const value of ['/home/file','../file','a/../b','a//b','C:\\file','a\u0000b']) assert.throws(() => files.relative(value),isCode('invalidName'));
  assert.equal(files.relative('.'),'');
});
test('collects files with bounded metadata without reading whole files', async () => {
  const calls = [], value = {name:'大檔',size:files.MAX_BYTES,slice:(start,end) => {calls.push([start,end]); return new Blob(['x']);}};
  const out = await files.collect(null,[value],'資料');
  assert.equal(out[0].path,'資料/大檔'); assert.deepEqual(calls,[[0,1]]);
});
test('directories preserve hierarchy and empty folders across reader batches', async () => {
  const root = entryDir('資料',[[entryDir('空',[])],[entryFile(file('檔案.txt'))]]);
  const result = await files.collect([drop(root)],[],'目的');
  assert.deepEqual(result.map(item => [item.path,item.directory]),[['目的/資料',true],['目的/資料/空',true],['目的/資料/檔案.txt',false]]);
});
test('captures every native entry before asynchronous file reads', async () => {
  let valid = true;
  const items = [0,1].map(n => ({kind:'file',webkitGetAsEntry:() => { assert.ok(valid); return {...entryFile(file(String(n))),file:resolve => {valid = false; resolve(file(String(n)));}};}}));
  assert.equal((await files.collect(items,[],'')).length,2);
});
test('rejects unsupported folders without flattening', async () => {
  await assert.rejects(files.collect([{kind:'file',webkitGetAsEntry:() => null}],[],''),isCode('unsupportedDirectory'));
  await assert.rejects(files.collect(null,[Object.assign(file('leaf'),{webkitRelativePath:'folder/leaf'})],''),isCode('unsupportedDirectory'));
});
test('limits count including directory entries and limits depth', async () => {
  await assert.rejects(files.collect(null,Array.from({length:65},(_,i) => file(String(i))),''),isCode('tooMany'));
  const large = entryDir('folder',[Array.from({length:64},(_,i) => entryFile(file(String(i))))]);
  await assert.rejects(files.collect([drop(large)],[],''),isCode('tooMany'));
  let nested = entryFile(file('leaf')); for (let i = 0; i < 16; i++) nested = entryDir('folder',[ [nested] ]);
  await assert.rejects(files.collect([drop(nested)],[],''),isCode('tooDeep'));
});
test('enforces per-file and aggregate 2 GiB, existing queue budget, duplicate destinations', async () => {
  const sized = size => ({name:String(size),size,slice:() => new Blob()});
  await assert.rejects(files.collect(null,[sized(files.MAX_BYTES + 1)],''),isCode('tooLarge'));
  await assert.rejects(files.collect(null,[sized(files.MAX_BYTES),file('other')],''),isCode('tooLarge'));
  await assert.rejects(files.collect(null,[file('a')],'',3,2),isCode('tooLarge'));
  await assert.rejects(files.collect(null,[file('a'),file('a')],''),isCode('duplicate'));
});

function host() {
  const calls=[],uploads=new Map();let hook=null;
  const call=async(action,params={})=>{
    calls.push({action,params});let out;
    if(action==='begin'){
      assert.match(params.token,/^[a-f0-9]{64}$/);const id=params.token.slice(0,32);
      const item=uploads.get(id)||{id,token:params.token,path:params.path,size:params.size,bytes:[],complete:false};uploads.set(id,item);
      out={id,resumeToken:params.token,state:item.complete?'complete':'uploading',nextOffset:item.bytes.length};
    }else if(action==='resume'){
      const item=uploads.get(params.id);if(!item)throw Error('missing');
      assert.equal(params.token,item.token);assert.equal(params.path,item.path);assert.equal(params.size,item.size);
      out={state:item.complete?'complete':'uploading',nextOffset:item.bytes.length};
    }else if(action==='write'){
      const item=uploads.get(params.id),bytes=Buffer.from(params.data,'base64');assert.equal(params.offset,item.bytes.length);assert.ok(bytes.length<=4096);
      item.bytes.push(...bytes);out={nextOffset:item.bytes.length};
    }else if(action==='commit'){const item=uploads.get(params.id);assert.equal(item.bytes.length,item.size);item.complete=true;out={ok:true};}
    else if(action==='cancel'){
      const item=uploads.get(params.id);
      if(item){assert.equal(params.token,item.token);assert.equal(params.path,item.path);assert.equal(params.size,item.size);}
      const complete=!!item?.complete;if(!complete)uploads.delete(params.id);out={ok:true,state:complete?'complete':'cancelled'};
    }
    else if(action==='mkdir')out={ok:true};
    else if(action==='list')out={path:params.path,entries:[],nextOffset:-1};
    if(hook)await hook(action,params,out);return out;
  };
  return{call,calls,uploads,setHook:value=>{hook=value;}};
}
const task=(name='a',data='abc')=>{const blob=file(name,data);return{path:name,size:blob.size,file:blob};};
test('uploads in 4 KiB chunks with client-generated begin token and percentage',async()=>{
  const h=host(),item=task('large',new Uint8Array(10000).map((_,i)=>i%251)),progress=[];
  await files.upload(item,h.call,n=>progress.push(n),{});
  assert.deepEqual(h.calls.map(call=>call.action),['begin','write','write','write','commit']);
  assert.equal(item.transfer.id,item.transfer.token.slice(0,32));assert.equal(item.transfer.complete,true);
  assert.equal(h.uploads.get(item.transfer.id).bytes.length,10000);assert.equal(files.percent(10000,10000),100);
});
test('manual pause keeps token/File and resumes only from confirmed offset',async()=>{
  const h=host(),item=task('large',new Uint8Array(9000)),control={};
  await assert.rejects(files.upload(item,h.call,n=>{if(n>=4096)control.paused=true;},control),isCode('paused'));
  assert.equal(item.transfer.offset,4096);assert.ok(item.file);assert.equal(h.calls.some(call=>call.action==='cancel'),false);
  control.paused=false;const before=h.calls.length;await files.upload(item,h.call,()=>{},control);
  assert.equal(h.calls[before].action,'resume');assert.equal(h.calls[before+1].params.offset,4096);assert.equal(item.transfer.complete,true);
});
test('lost write ACK resumes confirmed server bytes, never blindly resending',async()=>{
  const h=host(),item=task('large',new Uint8Array(9000));let lost=true;
  h.setHook(action=>{if(action==='write'&&lost){lost=false;throw Error('lost ACK');}});
  await assert.rejects(files.upload(item,h.call,()=>{},{}));assert.equal(item.transfer.offset,0);
  const start=h.calls.length;await files.upload(item,h.call,()=>{},{});
  assert.equal(h.calls[start].action,'resume');assert.equal(h.calls[start+1].params.offset,4096);
  assert.equal(h.uploads.get(item.transfer.id).bytes.length,9000);
});
test('lost begin ACK retries the same idempotent begin token on manual continuation',async()=>{
  const h=host(),item=task();let lost=true;h.setHook(action=>{if(action==='begin'&&lost){lost=false;throw Error('lost ACK');}});
  await assert.rejects(files.upload(item,h.call,()=>{},{}));assert.match(item.transfer.token,/^[a-f0-9]{64}$/);
  await files.upload(item,h.call,()=>{},{});const begins=h.calls.filter(call=>call.action==='begin');assert.equal(begins.length,2);assert.equal(begins[0].params.token,begins[1].params.token);assert.equal(item.transfer.complete,true);
});
test('begin never received can be retried manually with the same idempotent token',async()=>{
  const h=host(),item=task(),call=(action,params)=>action==='begin'?Promise.reject(Error('offline')):h.call(action,params);
  await assert.rejects(files.upload(item,call,()=>{},{}));assert.ok(item.transfer.id);
  const token=item.transfer.token;await files.upload(item,h.call,()=>{},{});assert.equal(h.calls[0].action,'begin');assert.equal(h.calls[0].params.token,token);assert.equal(item.transfer.complete,true);
});
test('lost commit ACK resolves through complete receipt without rereading/resending',async()=>{
  const h=host(),item=task();let lost=true;h.setHook(action=>{if(action==='commit'&&lost){lost=false;throw Error('lost ACK');}});
  await assert.rejects(files.upload(item,h.call,()=>{},{}));const start=h.calls.length;
  item.file={slice:()=>assert.fail('no reread')};await files.upload(item,h.call,()=>{},{});
  assert.deepEqual(h.calls.slice(start).map(call=>call.action),['resume']);assert.equal(item.transfer.complete,true);
});
test('cancel proves ownership directly, but never deletes a completed file',async()=>{
  const h=host(),item=task('a',new Uint8Array(5000)),control={};
  await assert.rejects(files.upload(item,h.call,n=>{if(n>0)control.paused=true;},control),isCode('paused'));
  const start=h.calls.length;await files.cancelUpload(item,h.call);assert.deepEqual(h.calls.slice(start).map(call=>call.action),['cancel']);
  const completed=task('complete');await files.upload(completed,h.call,()=>{},{});
  completed.transfer.complete=false;const startComplete=h.calls.length;assert.equal((await files.cancelUpload(completed,h.call)).complete,true);
  assert.deepEqual(h.calls.slice(startComplete).map(call=>call.action),['cancel']);
});
test('empty file commits with 100 percent and empty directory is created',async()=>{
  const h=host(),item=task('empty','');await files.upload(item,h.call,()=>{},{});
  assert.equal(files.percent(0,0,item.transfer.complete),100);
  const directory={path:'folder',directory:true,size:0};await files.upload(directory,h.call,()=>{},{});assert.equal(directory.transfer.complete,true);
});
test('directory acknowledgement loss never automatically merges on retry',async()=>{
  const h=host(),item={path:'folder',directory:true,size:0};h.setHook(action=>{if(action==='mkdir')throw Error('lost');});
  await assert.rejects(files.upload(item,h.call,()=>{},{}),isCode('unknownDirectory'));
  await assert.rejects(files.upload(item,h.call,()=>{},{}),isCode('unknownDirectory'));assert.equal(h.calls.length,1);
});
test('bad acknowledgement retains upload for explicit resume or cancel',async()=>{
  const h=host(),item=task(),call=async(action,params)=>{const out=await h.call(action,params);return action==='write'?{nextOffset:99999}:out;};
  await assert.rejects(files.upload(item,call,()=>{},{}),isCode('invalidReply'));assert.ok(item.transfer.id);
  assert.equal(h.calls.some(call=>call.action==='cancel'),false);
});
test('pausing before any request does not create remote state',async()=>{
  await assert.rejects(files.upload(task(),()=>assert.fail('no request'),()=>{},{paused:true}),isCode('paused'));
});
class Element {
  constructor(tag='div'){this.tagName=tag;this.children=[];this.events=new Map();this.dataset={};this.hidden=false;this.textContent='';this.classList={toggle:()=>{},add:()=>{},remove:()=>{}};}
  append(...children){for(const child of children){child.parent=this;this.children.push(child);}}
  replaceChildren(...children){this.children=[];this.append(...children);}
  remove(){if(this.parent)this.parent.children=this.parent.children.filter(item=>item!==this);}
  setAttribute(name,value){this[name]=value;}
  addEventListener(name,callback){this.events.set(name,callback);}
  querySelector(tag){for(const child of this.children){if(child.tagName===tag)return child;const found=child.querySelector(tag);if(found)return found;}return null;}
  showModal(){this.open=true;}close(){this.open=false;}focus(){}click(){this.events.get('click')?.();}
}
function browser(respond,language='en',bindings={}){
  const elements=new Map(),calls=[],events=new Map(),timers=new Map(),closeTimers=new Map();let replaced='',connection=true;
  const get=id=>{if(!elements.has(id))elements.set(id,new Element());return elements.get(id);};
  const document={documentElement:{},getElementById:get,querySelectorAll:()=>[],createElement:tag=>new Element(tag),addEventListener:()=>{}};
  const window={...bindings,addEventListener:(name,callback)=>events.set(name,callback)};
  const schedule=(callback,delay)=>{if(delay!==3000&&delay!==8000)return setTimeout(callback,delay);const id=Symbol('timer');(delay===3000?timers:closeTimers).set(id,callback);return id;};
  const clear=id=>{if(typeof id==='symbol'){timers.delete(id);closeTimers.delete(id);}else clearTimeout(id);};
  const context={document,window,location:{hash:'#token=secret-token&session=demo&instance=one&language='+language+'&name=Example',pathname:'/files.html'},history:{replaceState:(_,__,value)=>{replaced=value;}},navigator:{language:'en'},URLSearchParams,AbortController,TextEncoder,setTimeout:schedule,clearTimeout:clear,Uint8Array,btoa,crypto:webcrypto,fetch:async(url,options)=>{
    calls.push({url,options});const body=JSON.parse(options.body);
    if(body.action==='status'){const connected=typeof connection==='function'?await connection():connection;return{ok:true,status:200,json:async()=>({connected})};}
    return respond(body);
  }};
  vm.runInNewContext(source,context);
  return{get,calls,events,document,window,replaced,setConnection:value=>{connection=value;},pendingPolls:()=>timers.size,expireClose:()=>{for(const callback of [...closeTimers.values()])callback();},poll:async()=>{const next=timers.entries().next().value;if(!next)return false;timers.delete(next[0]);await next[1]();return true;}};
}
const flush=()=>new Promise(resolve=>setImmediate(resolve));
const response=value=>({ok:true,status:200,json:async()=>value});
const fileEntry=(name='a',directory=false)=>({name,path:name,directory,size:directory?0:3,modified:'2026-01-01T00:00:00Z'});
function uiHost(entries=[fileEntry()]){
  const h=host(),env=browser(async request=>response(request.action==='list'?{path:request.params.path,entries:request.params.path?[]:entries,nextOffset:-1}:await h.call(request.action,request.params)));
  return{...env,h};
}
function input(env,...files){env.get('files-input').events.get('change')({target:{files,value:''}});}
function action(env,index,name){return env.get('files-queue').children[index].children[3].children.find(button=>button.dataset.transferAction===name).events.get('click')();}
const rowText=(env,index=0)=>env.get('files-queue').children[index].children[2].children[0].textContent;
test('browser strips fragment token and renders only relative plain-text names',async()=>{
  const entry=fileEntry('Unicode & 文字.txt'),env=uiHost([entry]);await flush();
  assert.equal(env.replaced,'/files.html');assert.equal(env.calls[0].options.headers['X-YourDesk-Token'],'secret-token');
  assert.equal(env.get('files-list').children[0].querySelector('bdi').textContent,entry.name);
  assert.equal(env.get('files-status').textContent,'Connected');
});
test('folder single-click selects, explicit Open browses, and create validates a single name',async()=>{
  const env=uiHost([fileEntry('folder',true)]);await flush();const button=env.get('files-list').children[0].querySelector('button');button.click();
  assert.equal(env.calls.length,1);assert.equal(env.get('files-open').disabled,false);env.get('files-open').click();await flush();
  assert.equal(JSON.parse(env.calls.at(-1).options.body).params.path,'folder');
  env.get('files-new-folder').click();env.get('folder-name').value='../escape';await env.get('folder-form').events.get('submit')({preventDefault(){}});
  assert.match(env.get('folder-error').textContent,/Unsupported/);assert.equal(env.h.calls.some(call=>call.action==='mkdir'),false);
  env.get('folder-name').value='new';await env.get('folder-form').events.get('submit')({preventDefault(){}});
  assert.ok(env.h.calls.some(call=>call.action==='mkdir'&&call.params.path==='folder/new'));
});
test('folder delete requires typed name and recursive confirmation, preserving metadata',async()=>{
  const requests=[],entry=fileEntry('folder',true),env=browser(async request=>{
    requests.push(request);return response(request.action==='list'?{path:'',entries:[entry],nextOffset:-1}:{ok:true,removedCount:2});
  });await flush();env.get('files-list').children[0].querySelector('button').click();env.get('files-delete').click();
  env.get('delete-name').value='wrong';env.get('delete-recursive-check').checked=true;await env.get('delete-form').events.get('submit')({preventDefault(){}});
  assert.equal(requests.some(request=>request.action==='remove'),false);
  env.get('delete-name').value='folder';await env.get('delete-form').events.get('submit')({preventDefault(){}});
  assert.deepEqual(requests.find(request=>request.action==='remove').params,{path:'folder',directory:true,modified:entry.modified,size:0,confirm:'folder',recursive:true});
});
test('partial delete reports removed count; unknown outcome never promises rollback',async()=>{
  for(const partial of [true,false]){
    const env=browser(async request=>request.action==='list'?response({path:'',entries:[fileEntry()],nextOffset:-1}):response(partial?{ok:false,partial:true,removedCount:2,error:'private details'}:{error:'private details'}));
    await flush();env.get('files-list').children[0].querySelector('button').click();env.get('files-delete').click();
    await env.get('delete-form').events.get('submit')({preventDefault(){}});
    assert.match(env.get('delete-error').textContent,partial?/2 items were deleted/:/outcome is unknown/);assert.doesNotMatch(env.get('delete-error').textContent,/private/);assert.equal(env.get('delete-confirm').disabled,true);
  }
});
test('UI pause retains job and gives explicit resume button, with percentage',async()=>{
  const env=uiHost();await flush();let release;env.h.setHook(actionName=>{if(actionName==='write')return new Promise(resolve=>{release=resolve;});});
  input(env,file('large',new Uint8Array(9000)));await flush();action(env,0,'pause');release();await flush();await flush();
  assert.match(rowText(env),/Paused.*45%/);assert.equal(env.h.calls.some(call=>call.action==='cancel'),false);
  env.h.setHook(null);action(env,0,'resume');await flush();await flush();assert.match(rowText(env),/Done.*100%/);
  assert.equal(env.h.calls.filter(call=>call.action==='resume').length,1);
});
test('disconnect preserves upload and new session does not auto-resume',async()=>{
  const env=uiHost();await flush();let release;env.h.setHook(actionName=>{if(actionName==='write')return new Promise(resolve=>{release=resolve;});});
  input(env,file('large',new Uint8Array(9000)));await flush();env.setConnection(false);await env.poll();release();await flush();await flush();
  assert.match(rowText(env),/Interrupted/);assert.equal(env.h.calls.some(call=>call.action==='cancel'),false);assert.equal(env.pendingPolls(),0);
  env.events.get('yourdesk-files-session')({detail:{instance:'two'}});await flush();assert.equal(env.get('files-pick').disabled,false);
  assert.equal(env.h.calls.filter(call=>call.action==='resume').length,0);
  env.h.setHook(null);action(env,0,'resume');await flush();await flush();assert.match(rowText(env),/Done.*100%/);
  const resumed=env.calls.find(call=>JSON.parse(call.options.body).action==='resume');assert.equal(JSON.parse(resumed.options.body).instance,'two');
});
test('a paused queued descendant prevents deleting its ancestor',async()=>{
  const env=uiHost([fileEntry('folder',true)]);await flush();let release;env.h.setHook(actionName=>{if(actionName==='write')return new Promise(resolve=>{release=resolve;});});
  env.get('files-list').children[0].querySelector('button').click();env.get('files-open').click();await flush();
  input(env,file('a'));await flush();action(env,0,'pause');release();await flush();await flush();env.get('files-up').click();await flush();
  env.get('files-list').children[0].querySelector('button').click();assert.equal(env.get('files-delete').disabled,true);
});
test('directory creation failure blocks children without starting them',async()=>{
  const env=uiHost();await flush();env.h.setHook(actionName=>{if(actionName==='mkdir')throw Error('exists');});
  const root=entryDir('existing',[[entryFile(file('a'))]]);
  env.get('drop-target').events.get('drop')({preventDefault(){},stopPropagation(){},dataTransfer:{items:[drop(root)],files:[]}});await flush();await flush();
  assert.deepEqual(env.h.calls.map(call=>call.action),['mkdir']);assert.match(rowText(env,1),/parent folder was not created/);
});
test('native download pause/resume and disconnect preserve pending promise',async()=>{
  const env=uiHost();await flush();env.get('files-list').children[0].querySelector('button').click();
  let finish,cancelled=0,paused=0,resumed=0;
  env.window.yourdeskPrepareFile=()=>new Promise(resolve=>{finish=resolve;});
  env.window.yourdeskPauseFile=async()=>{paused++;return{paused:true,received:1,total:3};};
  env.window.yourdeskResumeFile=async()=>{resumed++;return{resumed:true};};
  env.window.yourdeskCancelFile=async()=>{cancelled++;};
  const prepare=env.get('files-prepare').events.get('click')();await env.get('download-pause').events.get('click')();
  assert.match(env.get('download-status').textContent,/Paused.*33%/);await env.get('download-resume').events.get('click')();assert.equal(resumed,1);
  env.setConnection(false);await env.poll();await flush();assert.equal(cancelled,0);assert.equal(paused,2);
  env.events.get('yourdesk-files-session')({detail:{instance:'two'}});assert.equal(resumed,1);
  await env.get('download-resume').events.get('click')();finish({name:'a',size:3});await prepare;
  assert.match(env.get('download-status').textContent,/Download complete.*100%/);assert.equal(cancelled,0);
});
test('completed native drag source survives disconnect and status polling is single-flight',async()=>{
  const env=uiHost();await flush();env.get('files-list').children[0].querySelector('button').click();let cancelled=0;
  env.window.yourdeskPrepareFile=async()=>({name:'a',size:3});env.window.yourdeskCancelFile=async()=>{cancelled++;};
  await env.get('files-prepare').events.get('click')();let release;env.setConnection(()=>new Promise(resolve=>{release=resolve;}));
  const check=env.poll();assert.equal(await env.poll(),false);release(false);await check;
  assert.equal(cancelled,0);assert.match(env.get('download-status').textContent,/Download complete/);assert.equal(env.pendingPolls(),0);
});
test('portable path validation mirrors byte limits and reserved names',()=>{
  for(const value of ['x*','a?','a<','a>','a"','a|','end.','end ','CON','con.txt','COM0','LPT9.doc','.YOURDESK-transfer-x','中'.repeat(86)])assert.throws(()=>files.component(value),isCode('invalidName'));
  assert.equal(files.component('中'.repeat(85)).length,85);
  assert.throws(()=>files.relative(Array.from({length:5},()=> '中'.repeat(80)).join('/')),isCode('invalidName'));
});
test('acknowledged expired upload never silently starts a replacement',async()=>{
  const h=host(),item=task('a',new Uint8Array(5000)),control={};
  await assert.rejects(files.upload(item,h.call,n=>{if(n>0)control.paused=true;},control),isCode('paused'));
  h.uploads.clear();control.paused=false;const start=h.calls.length;await assert.rejects(files.upload(item,h.call,()=>{},control));
  assert.deepEqual(h.calls.slice(start).map(call=>call.action),['resume']);
});
test('rebind during in-flight cancellation schedules one cleanup pass on new instance',async()=>{
  const env=uiHost();await flush();let release;
  env.h.setHook(actionName=>{if(actionName==='write')return new Promise(resolve=>{release=resolve;});});
  input(env,file('a',new Uint8Array(5000)));await flush();action(env,0,'pause');release();await flush();await flush();
  let failOld;env.h.setHook(actionName=>{if(actionName==='cancel')return new Promise((_,reject)=>{failOld=reject;});});
  action(env,0,'cancel');await flush();assert.equal(typeof failOld,'function');
  env.events.get('yourdesk-files-session')({detail:{instance:'two'}});env.h.setHook(null);failOld(Error('old connection'));await flush();await flush();await flush();
  assert.match(rowText(env),/Cancelled/);
  const cancel=env.calls.filter(call=>JSON.parse(call.options.body).action==='cancel');assert.equal(cancel.length,2);assert.equal(JSON.parse(cancel[1].options.body).instance,'two');
});
test('native cancel keeps Prepare disabled until original promise settles',async()=>{
  const env=uiHost();await flush();env.get('files-list').children[0].querySelector('button').click();let rejectPrepare;
  env.window.yourdeskPrepareFile=()=>new Promise((_,reject)=>{rejectPrepare=reject;});env.window.yourdeskCancelFile=async()=>({cancelled:true});
  const prepare=env.get('files-prepare').events.get('click')();await env.get('download-cancel').events.get('click')();
  assert.equal(env.get('files-prepare').disabled,true);assert.match(env.get('download-status').textContent,/Cancelling/);
  rejectPrepare(Error('cancelled'));await prepare;assert.equal(env.get('files-prepare').disabled,false);assert.match(env.get('download-status').textContent,/Cancelled/);
});
test('stable local unavailable code changes to interrupted rather than a generic failure',async()=>{
  const env=browser(async()=>({ok:false,status:400,json:async()=>({code:'files_session_unavailable',error:'private backend details'})}));
  await flush();assert.match(env.get('files-status').textContent,/Disconnected/);assert.equal(env.get('files-pick').disabled,true);assert.doesNotMatch(env.get('files-status').textContent,/private/);
});
test('native initial session snapshot cannot overwrite a newer rebind event',async()=>{
  let release;
  const env=browser(async()=>response({path:'',entries:[],nextOffset:-1}),'en',{yourdeskFilesSession:()=>new Promise(resolve=>{release=resolve;})});
  assert.equal(env.calls.length,0);assert.equal(env.get('files-pick').disabled,true);
  env.events.get('yourdesk-files-session')({detail:{instance:'newest'}});release({instance:'older'});await flush();
  assert.equal(JSON.parse(env.calls[0].options.body).instance,'newest');
});

test('stale list failure cannot disconnect or stop polling the rebound session',async()=>{
  let resolveOld;
  const env=browser(async request=>request.instance==='one'?new Promise(resolve=>{resolveOld=resolve;}):response({path:'',entries:[],nextOffset:-1}));
  await flush();env.events.get('yourdesk-files-session')({detail:{instance:'two'}});
  resolveOld({ok:false,status:400,json:async()=>({code:'files_session_unavailable'})});await flush();await flush();
  assert.match(env.get('files-status').textContent,/Reconnected/);assert.equal(env.get('files-pick').disabled,false);assert.equal(env.pendingPolls(),1);
  await env.poll();assert.equal(env.pendingPolls(),1);
});
test('lost cancel ACK is manually retryable and does not block unrelated queued uploads',async()=>{
  const env=uiHost();await flush();let release,lost=true;
  env.h.setHook(actionName=>{
    if(actionName==='write'&&!release)return new Promise(resolve=>{release=resolve;});
    if(actionName==='cancel'&&lost){lost=false;throw new TypeError('lost cancel ACK');}
  });
  input(env,file('a',new Uint8Array(5000)),file('b','next'));await flush();action(env,0,'cancel');release();
  await flush();await flush();await flush();await env.poll();
  assert.match(rowText(env,0),/Choose Cancel to retry/);assert.match(rowText(env,1),/Done/);
  const cancel=env.get('files-queue').children[0].children[3].children.find(button=>button.dataset.transferAction==='cancel');
  assert.equal(cancel.disabled,false);assert.equal(env.h.calls.filter(call=>call.action==='cancel').length,1);
  action(env,0,'cancel');await flush();await flush();assert.match(rowText(env,0),/Cancelled/);
  assert.equal(env.h.calls.filter(call=>call.action==='cancel').length,2);
});
test('cancel ACK loss after commit preserves the complete receipt on manual retry',async()=>{
  const h=host(),item=task();await files.upload(item,h.call,()=>{},{});item.transfer.complete=false;
  let lost=true;h.setHook(actionName=>{if(actionName==='cancel'&&lost){lost=false;throw Error('lost ACK');}});
  await assert.rejects(files.cancelUpload(item,h.call));assert.equal((await files.cancelUpload(item,h.call)).complete,true);
  assert.ok(h.uploads.get(item.transfer.id).complete);
});
async function pausedUI(){
  const env=uiHost();await flush();let release;
  env.h.setHook(actionName=>actionName==='write'?new Promise(resolve=>{release=resolve;}):undefined);
  input(env,file('a',new Uint8Array(5000)));await flush();action(env,0,'pause');release();await flush();await flush();env.h.setHook(null);return env;
}
test('button, keyboard and native close request wait for confirmed upload cancellation',async()=>{
  for(const trigger of [env=>env.get('files-close').click(),env=>env.events.get('keydown')({ctrlKey:true,key:'w',preventDefault(){}}),env=>env.events.get('yourdesk-request-close')()]){
    const env=await pausedUI();let release,closed=false;
    env.window.yourdeskCloseFiles=async()=>{closed=true;};env.h.setHook(actionName=>actionName==='cancel'?new Promise(resolve=>{release=resolve;}):undefined);
    trigger(env);await flush();assert.equal(closed,false);assert.equal(env.get('files-close').disabled,true);
    assert.equal(env.h.calls.at(-1).action,'cancel');release();await flush();await flush();
    assert.equal(closed,true);assert.equal(env.h.uploads.size,0);assert.equal(env.pendingPolls(),0);
  }
});
test('failed close cleanup retains tokens and window and allows explicit close retry',async()=>{
  const env=await pausedUI();let closed=false,lost=true;
  env.window.yourdeskCloseFiles=async()=>{closed=true;};env.h.setHook(actionName=>{if(actionName==='cancel'&&lost){lost=false;throw Error('lost ACK');}});
  env.get('files-close').click();await flush();await flush();
  assert.equal(closed,false);assert.equal(env.get('files-close').disabled,false);assert.match(env.get('files-status').textContent,/window and resume data were kept/);
  env.get('files-close').click();await flush();await flush();assert.equal(closed,true);
  const attempts=env.h.calls.filter(call=>call.action==='cancel');assert.equal(attempts.length,2);assert.deepEqual(attempts[0].params,attempts[1].params);
});
test('bounded close deadline keeps window responsive without losing pending cleanup',async()=>{
  const env=await pausedUI();let release,closed=false;
  env.window.yourdeskCloseFiles=async()=>{closed=true;};env.h.setHook(actionName=>actionName==='cancel'?new Promise(resolve=>{release=resolve;}):undefined);
  env.get('files-close').click();await flush();env.expireClose();await flush();
  assert.equal(closed,false);assert.equal(env.get('files-close').disabled,false);assert.match(env.get('files-status').textContent,/window and resume data were kept/);
  release();await flush();await flush();env.get('files-close').click();await flush();assert.equal(closed,true);
});
test('closing while disconnected keeps unconfirmed transfers and their cancellation intent',async()=>{
  const env=await pausedUI();let closed=false;env.window.yourdeskCloseFiles=async()=>{closed=true;};
  env.setConnection(false);await env.poll();env.get('files-close').click();await flush();
  assert.equal(closed,false);assert.equal(env.h.uploads.size,1);assert.equal(env.get('files-close').disabled,false);
  env.events.get('yourdesk-files-session')({detail:{instance:'two'}});await flush();await flush();
  assert.match(rowText(env),/Cancelled/);env.get('files-close').click();await flush();assert.equal(closed,true);
});
test('late native pause and resume results cannot overwrite completed download state',async()=>{
  for(const operation of ['pause','resume']){
    const env=uiHost();await flush();env.get('files-list').children[0].querySelector('button').click();let finish,rejectControl;
    env.window.yourdeskPrepareFile=()=>new Promise(resolve=>{finish=resolve;});env.window.yourdeskCancelFile=async()=>{};
    env.window.yourdeskPauseFile=async()=>({paused:true,received:0,total:3});
    const preparing=env.get('files-prepare').events.get('click')();
    if(operation==='resume')await env.get('download-pause').events.get('click')();
    env.window[operation==='pause'?'yourdeskPauseFile':'yourdeskResumeFile']=()=>new Promise((_,reject)=>{rejectControl=reject;});
    const control=env.get('download-'+operation).events.get('click')();finish({name:'a',size:3});await preparing;
    rejectControl(Error('no current download'));await control;
    assert.match(env.get('download-status').textContent,/Download complete/);assert.equal(env.get('download-resume').hidden,true);
  }
});
test('late pause reply cannot overwrite a cancellation already in progress',async()=>{
  const env=uiHost();await flush();env.get('files-list').children[0].querySelector('button').click();let rejectPrepare,finishPause;
  env.window.yourdeskPrepareFile=()=>new Promise((_,reject)=>{rejectPrepare=reject;});env.window.yourdeskCancelFile=async()=>{};
  env.window.yourdeskPauseFile=()=>new Promise(resolve=>{finishPause=resolve;});
  const preparing=env.get('files-prepare').events.get('click')(),pausing=env.get('download-pause').events.get('click')();
  await env.get('download-cancel').events.get('click')();finishPause({paused:true,received:1,total:3});await pausing;
  assert.match(env.get('download-status').textContent,/Cancelling/);assert.equal(env.get('download-resume').hidden,true);
  rejectPrepare(Error('cancelled'));await preparing;assert.match(env.get('download-status').textContent,/Cancelled/);
});
test('native ready notification enables intercepted close only after the listener exists',async()=>{
  let ready=0;
  const env=browser(async()=>response({path:'',entries:[],nextOffset:-1}),'en',{yourdeskFilesReady:async()=>{ready++;}});
  assert.equal(typeof env.events.get('yourdesk-request-close'),'function');await flush();assert.equal(ready,1);assert.equal(env.get('files-pick').disabled,false);
});
test('close during a write waits for the active worker then cancels its upload',async()=>{
  const env=uiHost();await flush();let release,closed=false;
  env.window.yourdeskCloseFiles=async()=>{closed=true;};env.h.setHook(actionName=>actionName==='write'?new Promise(resolve=>{release=resolve;}):undefined);
  input(env,file('active',new Uint8Array(5000)));await flush();env.get('files-close').click();await flush();assert.equal(closed,false);
  release();await flush();await flush();assert.equal(closed,true);assert.equal(env.h.uploads.size,0);
  assert.deepEqual(env.h.calls.map(call=>call.action),['begin','write','cancel']);
});
