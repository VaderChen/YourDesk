'use strict';
(() => {
  const F = YourDeskFiles, dictionaries = YourDeskFileLocales, $ = id => document.getElementById(id);
  const settings = new URLSearchParams(location.hash.slice(1));
  const token = settings.get('token'), session = settings.get('session');
  let instance = settings.get('instance');
  history.replaceState(null,'',location.pathname);
  const requested = settings.get('language') === 'auto' ? navigator.language : settings.get('language');
  const locale = /^zh/i.test(requested || '') ? 'zh-Hant' : /^ja/i.test(requested || '') ? 'ja' : /^ko/i.test(requested || '') ? 'ko' : 'en';
  const t = (key,values={}) => Object.entries(values).reduce((text,[name,value]) => text.replaceAll('{' + name + '}',String(value)),dictionaries[locale][key] || dictionaries.en[key] || key);
  document.documentElement.lang = locale;
  for (const element of document.querySelectorAll('[data-text]')) element.textContent = t(element.dataset.text);
  document.title = (settings.get('name') || 'YourDesk') + ' · ' + t('title'); $('files-title').textContent = document.title;
  const state = {closed:false,closing:false,initializing:true,connected:true,reconnected:false,path:'',selected:new Set(),anchor:null,entries:[],next:-1,listing:false,generation:0,queue:[],batch:0,running:false,cancelPassInstance:'',scanning:false,mutating:false,deleteTarget:null,download:{state:'idle',pending:false,generation:0,path:'',received:0,total:0}};
  let statusTimer, checkingStatus = false, checkClose;
  const destination={path:'',parent:'',next:-1,loading:false,generation:0,remote:null,instance:''};
  let parentPath=null,virtualDirectory=false,filesystemAvailable=false;
  const selectedEntries=()=>state.entries.filter(entry=>state.selected.has(entry.path));
  const terminal = item => ['done','cancelled'].includes(item.state);
  const sizeText = bytes => {if (!Number.isFinite(bytes)) return '—'; const units=['B','KiB','MiB','GiB']; let i=0; while(bytes>=1024 && i<3){bytes/=1024;i++;} return bytes.toFixed(i?1:0)+' '+units[i];};
  const progressText = (received,total,complete=false,speed=0,eta) => {
    let text = F.percent(received,total,complete)+'% · '+sizeText(received)+' / '+sizeText(total);
    if (Number.isFinite(speed) && speed > 0) text += ' · '+sizeText(speed)+'/s';
    if (Number.isFinite(eta) && eta >= 0) text += ' · '+t('eta')+' '+Math.ceil(eta)+' '+t('seconds');
    return text;
  };
  const errorText = error => error?.code === 'partialRemove' ? t('partialRemove',{count:error.removedCount}) : t(error?.code || 'failed');
  const status = (text,error=false) => {$('files-status').textContent=state.connected?text:t('disconnected'); $('files-status').classList.toggle('error',error||!state.connected);};
  async function call(action,params={}) {
    if (!token || !session || !instance || state.closed || (!state.connected && action !== 'status')) throw new F.FileError('interrupted');
    const requestInstance=instance, controller=new AbortController(), timer=setTimeout(()=>controller.abort(),30000);
    try {
      const response=await fetch('/api/files',{method:'POST',headers:{'Content-Type':'application/json','X-YourDesk-Token':token},body:JSON.stringify({session,instance:requestInstance,action,params}),signal:controller.signal,cache:'no-store'});
      if ([401,403,410].includes(response.status)) throw new F.FileError('interrupted');
      const out=await response.json();
      if(out?.code==='files_session_unavailable')throw new F.FileError('interrupted');
      if (out?.partial === true && Number.isSafeInteger(out.removedCount) && out.removedCount >= 0) {const error=new F.FileError('partialRemove'); error.removedCount=out.removedCount; throw error;}
      if (!response.ok || !out || out.error) throw new F.FileError('failed');
      return out;
    } finally {clearTimeout(timer);}
  }
  function protectedPath(path) {
    return state.queue.some(item=>!terminal(item)&&F.contains(path,item.path)) ||
      (state.download.pending && (F.contains(path,state.download.path)||(state.download.remaining||[]).some(entry=>F.contains(path,entry.path))));
  }
  function controls() {
    const unavailable=state.closed||state.closing||state.initializing||!state.connected||state.mutating;
    $('files-close').disabled=state.closing;
    $('files-disks').hidden=!filesystemAvailable; $('files-disks').disabled=unavailable||state.listing||virtualDirectory;
    $('files-up').disabled=unavailable||state.listing||parentPath===null;
    $('files-refresh').disabled=unavailable||state.listing;
    $('files-more').disabled=unavailable||state.listing;
    $('files-new-folder').disabled=unavailable||state.listing||state.scanning||virtualDirectory;
    const pickingDisabled=unavailable||state.scanning||state.listing||virtualDirectory;
    $('files-pick').disabled=pickingDisabled;$('files-pick').classList.toggle('disabled',pickingDisabled);$('files-input').disabled=pickingDisabled;
    const selected=selectedEntries();
    $('files-open').disabled=unavailable||selected.length!==1||!selected[0].directory;
    $('files-delete').disabled=unavailable||!selected.length||selected.some(entry=>entry.root===true||protectedPath(entry.path));
    $('files-prepare').disabled=unavailable||!selected.length||selected.some(entry=>entry.directory)||state.download.pending;
    $('files-select-all').disabled=unavailable||state.listing||!state.entries.length;
    $('files-select-all').checked=state.entries.length>0&&selected.length===state.entries.length;$('files-select-all').indeterminate=selected.length>0&&selected.length<state.entries.length;
    $('files-selection').hidden=!selected.length;$('files-selection').textContent=t('selectedCount',{count:selected.length});
    for(const row of $('files-list').children){row.querySelector('button').disabled=unavailable||state.listing||(row.dataset.parent==='true'&&parentPath===null);const check=row.querySelector('input');if(check)check.disabled=unavailable||state.listing;}
    $('destination-select').disabled=unavailable||destination.loading||!destination.path||$('destination-path').value!==destination.path;
    for(const item of state.queue) if(item.ui) {
      item.ui.pause.hidden=terminal(item)||item.state==='cancelling'||!['queued','running'].includes(item.state);
      item.ui.pause.disabled=unavailable||item.control.paused;
      item.ui.resume.hidden=!['paused','interrupted','error'].includes(item.state)||item.blocked||item.error==='unknownDirectory';
      item.ui.resume.disabled=unavailable||item.active;
      item.ui.cancel.hidden=terminal(item);
      item.ui.cancel.disabled=state.closing||(item.control.cancelled&&item.state!=='cancelFailed');
    }
    const download=state.download, running=download.pending&&download.state==='running';
    $('download-pause').hidden=!running; $('download-pause').disabled=unavailable;
    $('download-resume').hidden=!download.pending||!['paused','interrupted'].includes(download.state);
    $('download-resume').disabled=unavailable;
    $('download-cancel').hidden=!download.pending;
    $('download-cancel').disabled=download.state==='cancelling';
  }
  function queueSummary() {
    const items=state.queue.filter(item=>!item.directory&&item.state!=='cancelled');
    const total=items.reduce((sum,item)=>sum+item.size,0), received=items.reduce((sum,item)=>sum+(item.transfer?.offset||0),0);
    $('queue-summary').hidden=!items.length; $('queue-progress').hidden=!items.length;
    $('queue-summary').textContent=progressText(received,total,items.length>0&&items.every(item=>item.state==='done'));
    $('queue-progress').max=total||1; $('queue-progress').value=received||(items.length>0&&items.every(item=>item.state==='done')?1:0);
  }
  function renderItem(item) {
    const record=F.transfer(item), speed=item.active ? Math.max(0,(record.offset-item.meter.bytes)/Math.max(.001,(Date.now()-item.meter.time)/1000)) : 0;
    const stage=item.error?errorText({code:item.error}):t(item.state==='queued'?'waiting':item.state==='running'?(item.control.paused?'pausing':'uploading'):item.state==='cancelling'?'cancelPending':item.state);
    item.ui.text.textContent=stage+(item.directory?'':' · '+progressText(record.offset,item.size,item.state==='done',speed,speed>0?(item.size-record.offset)/speed:undefined));
    item.ui.progress.max=item.size||1; item.ui.progress.value=record.offset||(item.state==='done'?1:0);
    item.ui.li.classList.toggle('failed',item.state==='error'||!!item.blocked);
    queueSummary(); controls();checkClose?.();
  }
  function interruptUploads() {
    for(const item of state.queue) if(!terminal(item)&&!item.control.cancelled) {
      item.control.interrupted=true;
      if(['queued','running'].includes(item.state)) item.state='interrupted';
      renderItem(item);
    }
  }
  function disconnected() {
    if(!state.connected||state.closed)return;
    state.connected=false; clearTimeout(statusTimer); ++state.generation;
    interruptUploads();
    if(state.download.pending && state.download.state==='running') void pauseDownload('interrupted');
    status(t('disconnected'),true); controls();
  }
  function scheduleStatus() {
    clearTimeout(statusTimer);
    if(!state.closed&&state.connected&&!checkingStatus)statusTimer=setTimeout(checkConnection,3000);
  }
  async function checkConnection() {
    if(state.closed||checkingStatus)return;
    checkingStatus=true; const checkedInstance=instance;
    try {const out=await call('status'); if(checkedInstance===instance&&out.connected!==true)disconnected();}
    catch {if(checkedInstance===instance)disconnected();}
    finally {checkingStatus=false;scheduleStatus();}
  }
  function renderList() {
    const body=$('files-list'); body.replaceChildren();
    const parentRow=document.createElement('tr'),parentCell=document.createElement('td'),parentButton=document.createElement('button');
    parentRow.dataset.parent='true';parentCell.colSpan=3;parentButton.type='button';parentButton.className='entry-button';parentButton.textContent='..';parentButton.title=t('up');parentButton.setAttribute('aria-label','.. · '+t('up'));
    parentButton.addEventListener('click',()=>{if(parentPath!==null)void list(parentPath);});
    parentCell.append(parentButton);parentRow.append(parentCell);body.append(parentRow);
    for(const entry of state.entries) {
      const displayName=entry.root===true&&entry.displayName?entry.displayName:entry.name;
      const row=document.createElement('tr'), cell=document.createElement('td'), wrap=document.createElement('div'),check=document.createElement('input'), button=document.createElement('button'), icon=document.createElement('span'), label=document.createElement('bdi');
      row.dataset.path=entry.path;wrap.className='entry-cell';check.type='checkbox';check.className='entry-check';check.checked=state.selected.has(entry.path);check.setAttribute('aria-label',t('selectEntry',{name:displayName}));
      button.type='button';button.className='entry-button';button.title=displayName+(entry.directory?' · '+t('folderHint'):'');
      icon.className='entry-icon '+(entry.directory?'entry-icon-folder':'entry-icon-file');icon.setAttribute('aria-hidden','true');label.className='entry-name';label.textContent=displayName;
      button.append(icon,label);wrap.append(check,button);cell.append(wrap);row.append(cell);
      const size=document.createElement('td'),modified=document.createElement('td'),date=new Date(entry.modified);
      size.textContent=entry.directory?t('folder'):sizeText(entry.size);
      modified.textContent=entry.modified&&!Number.isNaN(date.getTime())?date.toLocaleDateString(locale):'—';row.append(size,modified);body.append(row);
      button.setAttribute('aria-pressed',String(state.selected.has(entry.path)));row.classList.toggle('selected',state.selected.has(entry.path));
      const select=(event={},toggle=false)=>{
        if(state.closed||!state.connected||state.mutating)return;
        const additive=toggle||event.metaKey||event.ctrlKey;
        if(event.shiftKey&&state.anchor!==null){
          const start=state.entries.findIndex(item=>item.path===state.anchor),end=state.entries.indexOf(entry);
          if(!additive)state.selected.clear();
          for(const item of state.entries.slice(Math.max(0,Math.min(start,end)),Math.max(start,end)+1))state.selected.add(item.path);
        }else{if(!additive)state.selected.clear();if(additive&&state.selected.has(entry.path))state.selected.delete(entry.path);else state.selected.add(entry.path);state.anchor=entry.path;}
        updateSelection();
      };
      button.addEventListener('click',event=>select(event));check.addEventListener('click',event=>select(event,true));
      button.addEventListener('mousedown',event=>{if(event.shiftKey)event.preventDefault();});
      button.addEventListener('dblclick',()=>{if(entry.directory)void list(entry.path);});
    }
    $('files-empty').hidden=state.entries.length!==0||state.listing;
    $('files-more').hidden=state.next<0||state.entries.length>=1000;
    controls();
  }
  function updateSelection(){
    for(const row of $('files-list').children){const selected=state.selected.has(row.dataset.path);row.classList.toggle('selected',selected);row.querySelector('button').setAttribute('aria-pressed',String(selected));const check=row.querySelector('input');if(check)check.checked=selected;}
    controls();
  }
  $('files-select-all').setAttribute('aria-label',t('selectAll'));
  $('files-input').setAttribute('aria-label',t('pick'));
  $('files-select-all').addEventListener('change',event=>{state.selected=new Set(event.target.checked?state.entries.map(entry=>entry.path):[]);state.anchor=null;updateSelection();});
  $('files-list').addEventListener('keydown',event=>{if((event.metaKey||event.ctrlKey)&&event.key.toLowerCase()==='a'){event.preventDefault();state.selected=new Set(state.entries.map(entry=>entry.path));updateSelection();}});
  async function list(path,append=false) {
    if(state.initializing||state.listing||state.closed||state.closing||!state.connected||state.mutating)return;
    state.listing=true;controls();status(t('loading'));const generation=++state.generation;
    try {
      const clean=F.relative(path),offset=append?state.next:0,out=await call('list',{path:clean,offset});
      if(state.closed||generation!==state.generation)return;
      if(F.relative(out.path)!==clean||!Array.isArray(out.entries)||out.entries.length>1000||!Number.isSafeInteger(out.nextOffset)||(out.nextOffset!==-1&&out.nextOffset<=offset))throw new F.FileError('invalidReply');
      const entries=out.entries.map(entry=>{
        if(F.join(clean,entry.name)!==F.relative(entry.path)||typeof entry.directory!=='boolean'||!Number.isSafeInteger(entry.size)||entry.size<0)throw new F.FileError('invalidReply');
        if(entry.displayName!==undefined&&(entry.root!==true||typeof entry.displayName!=='string'||entry.displayName.length>1024))throw new F.FileError('invalidReply');
        return entry;
      });
      const location=out.location;
      if(location&&(typeof location.display!=='string'||typeof location.virtual!=='boolean'||(location.parent!==null&&typeof location.parent!=='string')))throw new F.FileError('invalidReply');
      filesystemAvailable=!!location;
      parentPath=location?(location.parent===null?null:F.relative(location.parent)):(clean?clean.split('/').slice(0,-1).join('/'):null);
      virtualDirectory=location?.virtual===true;
      // 舊 Host 的分頁可能尚未排序；已載入的項目仍統一排序並隱藏點開頭名稱。
      state.path=clean;state.entries=(append?state.entries:[]).concat(entries).filter(entry=>!entry.name.startsWith('.')).sort((a,b)=>{
        if(a.directory!==b.directory)return a.directory?-1:1;
        const left=(a.displayName||a.name).toLowerCase(),right=(b.displayName||b.name).toLowerCase();
        return left<right?-1:left>right?1:a.name<b.name?-1:a.name>b.name?1:0;
      }).slice(0,1000);state.next=out.nextOffset;if(!append){state.selected.clear();state.anchor=null;}
      $('files-path').textContent=location?(virtualDirectory?t('computer'):location.display):t('home')+(clean?' / '+clean:'');status(t(state.entries.length>=1000&&state.next>=0?'listLimit':state.reconnected?'reconnected':'ready'));
    }catch(error){if(!state.closed&&generation===state.generation){if(error.code==='interrupted')disconnected();status(errorText(error),true);}}
    finally{state.listing=false;renderList();}
  }
  function blockChildren(item) {
    for(const child of state.queue)if(child.batch===item.batch&&child.path.startsWith(item.path+'/')&&!terminal(child)){
      child.blocked=true;child.state='error';child.error='parentFailed';renderItem(child);
    }
  }
  function dependenciesReady(item) {
    return !state.queue.some(parent=>parent!==item&&parent.directory&&parent.batch===item.batch&&F.contains(parent.path,item.path)&&parent.state!=='done');
  }
  function queueView(item) {
    const li=document.createElement('li'),name=document.createElement('bdi'),line=document.createElement('div'),text=document.createElement('span'),actions=document.createElement('div'),pause=document.createElement('button'),resume=document.createElement('button'),cancel=document.createElement('button'),progress=document.createElement('progress');
    name.className='transfer-name';name.textContent=item.path;line.className='transfer-state';actions.className='transfer-actions';
    for(const [button,key]of[[pause,'pause'],[resume,'resume'],[cancel,'cancel']]){button.type='button';button.textContent=t(key);button.dataset.transferAction=key;actions.append(button);}
    progress.setAttribute('aria-label',t('percent'));
    pause.addEventListener('click',()=>{if(terminal(item)||item.control.cancelled)return;item.control.paused=true;if(!item.active)item.state='paused';renderItem(item);});
    resume.addEventListener('click',()=>{
      if(!state.connected||item.active||item.control.cancelled||item.blocked||item.error==='unknownDirectory')return;
      item.control.paused=false;item.control.interrupted=false;item.error='';item.state='queued';item.resuming=!!item.transfer?.started;renderItem(item);void runQueue();
    });
    cancel.addEventListener('click',()=>{
      if(terminal(item))return;item.control.cancelled=true;item.state='cancelling';item.error='';renderItem(item);
      if(item.directory)blockChildren(item);
      void runQueue();
    });
    line.append(text);li.append(name,progress,line,actions);$('files-queue').append(li);item.ui={li,text,pause,resume,cancel,progress};renderItem(item);
  }
  async function runQueue() {
    if(state.running||state.closed||!state.connected)return;state.running=true;
    try{
      for(;;){
        if(state.closed||!state.connected)break;
        const slots=state.queue.filter(item=>item.transfer?.id&&!item.transfer.complete&&!terminal(item)).length;
        const item=state.queue.find(item=>!item.active&&(item.state==='cancelling'||(item.state==='queued'&&dependenciesReady(item)&&(item.directory||item.transfer?.started||slots<4))));
        if(!item)break;
        const activeInstance=instance,cancelling=item.control.cancelled;item.active=true;item.meter={time:Date.now(),bytes:item.transfer?.offset||0};renderItem(item);
        const boundCall=async(action,params)=>{
          if(activeInstance!==instance)throw new F.FileError('interrupted');
          return call(action,params);
        };
        try{
          if(item.control.cancelled){
            const result=await F.cancelUpload(item,boundCall);item.state=result.complete?'done':'cancelled';item.error='';
          }else{
            item.state='running';renderItem(item);
            await F.upload(item,boundCall,()=>renderItem(item),item.control);
            item.state='done';item.error='';
          }
        }catch(error){
          if(item.control.cancelled){item.state=cancelling?'cancelFailed':'cancelling';item.error=cancelling?'cancelFailed':'';}
          else if(error.code==='paused'||item.control.paused){item.state='paused';item.error='';}
          else if(error.code==='interrupted'||item.control.interrupted){item.state='interrupted';item.error='';}
          else{item.state='error';item.error=error.code==='unknownDirectory'?'unknownDirectory':item.resuming?'resumeFailed':error.code||'failed';}
          if(error.code==='interrupted'&&activeInstance===instance)disconnected();
          if(item.directory&&item.state==='error')blockChildren(item);
          // Retain a failed cancellation for explicit retry/reconnection, but
          // keep processing unrelated queued work. Never replay file bytes.
        }finally{
          item.active=false;if(terminal(item))item.file=null;renderItem(item);
        }
      }
    }finally{
      state.running=false;
      if(!state.closed&&state.connected){
        if(state.cancelPassInstance===instance){state.cancelPassInstance='';for(const item of state.queue)if(item.state==='cancelFailed')item.state='cancelling';void runQueue();}
        else void list(state.path);
      }
      checkClose?.();
    }
  }
  async function enqueue(items,files) {
    if(state.closed||state.closing||state.initializing||!state.connected||state.mutating||virtualDirectory)return;
    if(state.scanning||state.listing){status(t('busy'),true);return;}
    state.scanning=true;controls();status(t('scanning'));
    try{
      const pending=state.queue.filter(item=>!terminal(item)),bytes=pending.reduce((sum,item)=>sum+item.size,0);
      const collected=await F.collect(items,files,state.path,F.MAX_ITEMS-pending.length,F.MAX_BYTES-bytes);
      if(state.closed||state.closing||!state.connected||state.mutating)return;
      const paths=new Set(pending.map(item=>item.path));if(collected.some(item=>paths.has(item.path)))throw new F.FileError('duplicate');
      for(const old of state.queue)if(terminal(old))old.ui.li.remove();
      state.queue=state.queue.filter(item=>!terminal(item));const batch=++state.batch;
      for(const item of collected){item.batch=batch;item.state='queued';item.control={cancelled:false,paused:false,interrupted:false};state.queue.push(item);queueView(item);}
      $('queue-empty').hidden=state.queue.length>0;status(t('ready'));void runQueue();
    }catch(error){status(errorText(error),true);}
    finally{state.scanning=false;controls();}
  }
  $('files-input').addEventListener('change',event=>{
    if(event.target.files.length>F.MAX_ITEMS){status(t('tooMany'),true);event.target.value='';return;}
    const files=Array.from(event.target.files);event.target.value='';void enqueue(null,files);
  });
  const dropTarget=$('drop-target');
  document.addEventListener('dragover',event=>{event.preventDefault();if(event.dataTransfer)event.dataTransfer.dropEffect='none';});
  document.addEventListener('drop',event=>event.preventDefault());
  dropTarget.addEventListener('dragover',event=>{event.preventDefault();event.stopPropagation();const ready=state.connected&&!state.initializing&&!state.mutating&&!virtualDirectory&&!state.listing;if(event.dataTransfer)event.dataTransfer.dropEffect=ready?'copy':'none';if(ready)dropTarget.classList.add('drag-over');});
  dropTarget.addEventListener('dragleave',event=>{if(!dropTarget.contains(event.relatedTarget))dropTarget.classList.remove('drag-over');});
  dropTarget.addEventListener('drop',event=>{event.preventDefault();event.stopPropagation();dropTarget.classList.remove('drag-over');if(event.dataTransfer)void enqueue(event.dataTransfer.items,event.dataTransfer.files);});
  $('files-disks').addEventListener('click',()=>void list(''));
  $('files-up').addEventListener('click',()=>{if(parentPath!==null)void list(parentPath);});
  $('files-refresh').addEventListener('click',()=>void list(state.path));
  $('files-more').addEventListener('click',()=>void list(state.path,true));
  $('files-open').addEventListener('click',()=>{const selected=selectedEntries();if(selected.length===1&&selected[0].directory)void list(selected[0].path);});
  $('files-new-folder').addEventListener('click',()=>{
    if(!state.connected||state.mutating||virtualDirectory)return;$('folder-name').value='';$('folder-error').textContent='';$('folder-dialog').showModal();$('folder-name').focus();
  });
  $('folder-cancel').addEventListener('click',()=>$('folder-dialog').close());
  $('folder-form').addEventListener('submit',async event=>{
    event.preventDefault();if(!state.connected||state.mutating||virtualDirectory)return;
    state.mutating=true;controls();$('folder-create').disabled=true;
    try{
      const path=F.join(state.path,$('folder-name').value);
      const result=await call('mkdir',{path});if(result.ok!==true)throw new F.FileError('failed');
      $('folder-dialog').close();state.mutating=false;await list(state.path);status(t('created'));
    }catch(error){$('folder-error').textContent=errorText(error);}
    finally{state.mutating=false;$('folder-create').disabled=false;controls();}
  });
  $('files-delete').addEventListener('click',()=>{
    const targets=selectedEntries();if(!targets.length||targets.some(target=>target.root===true)||!state.connected||state.mutating)return;
    if(targets.some(target=>protectedPath(target.path))){status(t('removeBusy'),true);return;}
    state.deleteTarget=targets.map(target=>({...target}));const directories=targets.filter(target=>target.directory);
    $('delete-target').textContent=targets.map(target=>target.path).join('\n');$('delete-error').textContent='';$('delete-name').value='';$('delete-recursive-check').checked=false;$('delete-confirm').disabled=false;
    $('delete-confirm-name').textContent=t(directories.length>1?'confirmNames':'confirmName');
    $('delete-recursive').hidden=!directories.length;$('delete-dialog').showModal();(directories.length?$('delete-name'):$('delete-cancel')).focus();
  });
  $('delete-cancel').addEventListener('click',()=>$('delete-dialog').close());
  $('delete-form').addEventListener('submit',async event=>{
    event.preventDefault();const targets=state.deleteTarget;if(!targets?.length||targets.some(target=>target.root===true)||!state.connected||state.mutating)return;
    if(targets.some(target=>protectedPath(target.path))){$('delete-error').textContent=t('removeBusy');return;}
    const directories=targets.filter(target=>target.directory);
    if(directories.length&&($('delete-name').value!==directories.map(target=>target.name).join('\n')||!$('delete-recursive-check').checked)){$('delete-error').textContent=t('confirmMismatch');return;}
    state.mutating=true;controls();$('delete-confirm').disabled=true;
    let removed=0;
    try{
      for(const target of targets){
        const result=await call('remove',{path:target.path,directory:target.directory,modified:target.modified,size:target.size,confirm:target.name,recursive:target.directory});
        if(result.ok!==true||!Number.isSafeInteger(result.removedCount)||result.removedCount<0)throw new F.FileError('failed');
        removed+=result.removedCount;
      }
      $('delete-dialog').close();state.mutating=false;await list(state.path);status(t('removed',{count:removed}));
    }catch(error){
      if(error.code==='partialRemove')error.removedCount+=removed;
      $('delete-error').textContent=error.code==='partialRemove'?errorText(error):t('removeUnknown');
      if(error.code==='partialRemove'){state.mutating=false;await list(state.path);status(errorText(error),true);}
    }finally{state.mutating=false;$('delete-confirm').disabled=true;controls();}
  });
  function renderDownload() {
    const item=state.download;
    $('download-transfer').hidden=item.state==='idle';
    if(item.state==='idle')return;
    $('download-progress').hidden=false;$('download-progress').max=item.total||1;$('download-progress').value=item.received||(item.state==='complete'?1:0);
    if(item.state==='complete')$('download-status').textContent=t('downloadReady')+' '+(item.localPath||item.name)+' · '+progressText(item.received,item.total,true);
    else $('download-status').textContent=(item.error?errorText({code:item.error}):t(item.state==='running'?'preparing':item.state))+' · '+progressText(item.received,item.total,false,item.state==='running'?item.speed:0,item.state==='running'?item.eta:undefined);
    if(item.count>1)$('download-status').textContent=t('downloadBatch',{done:item.completed,count:item.count})+' · '+item.name+' · '+$('download-status').textContent;
    controls();checkClose?.();
  }
  $('files-prepare').addEventListener('click',async()=>{
    const selected=selectedEntries();if(state.closed||!state.connected||!selected.length||selected.some(entry=>entry.directory)||state.download.pending)return;
    if(selected.length>F.MAX_ITEMS){status(t('tooMany'),true);return;}
    if(selected.reduce((sum,entry)=>sum+entry.size,0)>F.MAX_BYTES){status(t('tooLarge'),true);return;}
    if(typeof window.yourdeskPrepareFile!=='function'||typeof window.yourdeskCancelFile!=='function'||typeof window.yourdeskListDirectories!=='function'){$('download-transfer').hidden=false;$('download-status').textContent=t('nativeMissing');return;}
    destination.remote=selected.map(entry=>({...entry}));destination.instance=instance;destination.path='';
    $('destination-dialog').showModal();void loadDestination('');
  });
  async function loadDestination(path,append=false){
    if(destination.loading||state.closed)return;
    const generation=++destination.generation;destination.loading=true;
    $('destination-go').disabled=true;$('destination-more').disabled=true;controls();$('destination-error').textContent='';
    try{
      const out=await window.yourdeskListDirectories(path,append?destination.next:0);
      if(generation!==destination.generation||!$('destination-dialog').open)return;
      if(!out||typeof out.path!=='string'||!out.path||typeof out.parent!=='string'||!Array.isArray(out.entries)||out.entries.length>256||!Number.isSafeInteger(out.nextOffset))throw new F.FileError('invalidReply');
      destination.path=out.path;destination.parent=out.parent;destination.next=out.nextOffset;$('destination-path').value=out.path;
      const list=$('destination-list');if(!append){
        list.replaceChildren();const up=document.createElement('button');up.type='button';up.textContent='..';up.title=t('up');up.disabled=!out.parent;up.addEventListener('click',()=>void loadDestination(out.parent));list.append(up);
      }
      for(const entry of out.entries){
        if(typeof entry.name!=='string'||typeof entry.path!=='string')throw new F.FileError('invalidReply');
        const button=document.createElement('button');button.type='button';button.textContent=entry.name;button.title=entry.path;button.addEventListener('click',()=>void loadDestination(entry.path));list.append(button);
      }
      $('destination-more').hidden=out.nextOffset<0;
    }catch(error){if(generation===destination.generation)$('destination-error').textContent=t('destinationFailed');}
    finally{if(generation===destination.generation){destination.loading=false;$('destination-go').disabled=false;$('destination-more').disabled=false;controls();}}
  }
  $('destination-go').addEventListener('click',()=>void loadDestination($('destination-path').value));
  $('destination-path').addEventListener('input',()=>{$('destination-select').disabled=true;});
  $('destination-path').addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();void loadDestination(event.target.value);}});
  $('destination-more').addEventListener('click',()=>void loadDestination(destination.path,true));
  $('destination-cancel').addEventListener('click',()=>$('destination-dialog').close());
  $('destination-dialog').addEventListener('close',()=>{destination.generation++;destination.loading=false;destination.remote=null;});
  $('destination-select').addEventListener('click',async()=>{
    if(state.closed||state.closing||!state.connected||state.download.pending||destination.loading||!destination.remote||!destination.path||$('destination-path').value!==destination.path||destination.instance!==instance)return;
    const remote=destination.remote,directory=destination.path;$('destination-dialog').close();
    const generation=state.download.generation+1;
    state.download={state:'running',pending:true,generation,path:remote[0].path,name:remote[0].name,received:0,total:remote[0].size,speed:0,remaining:remote.slice(1),completed:0,count:remote.length};renderDownload();
    const download=state.download;
    try{
      for(let index=0;index<remote.length;index++){
        if(state.closed||download.cancelRequested||generation!==state.download.generation)return;
        const entry=remote[index];Object.assign(download,{path:entry.path,name:entry.name,received:0,total:entry.size,speed:0,remaining:remote.slice(index+1)});renderDownload();
        const out=await window.yourdeskPrepareFile(entry.path,directory);
        if(state.closed||generation!==state.download.generation)return;
        if(!out||typeof out.name!=='string'||F.join('',out.name)!==out.name||!Number.isSafeInteger(out.size)||out.size<0||out.size>F.MAX_BYTES)throw new F.FileError('invalidReply');
        Object.assign(download,{completed:index+1,name:out.name,localPath:out.path,total:out.size,received:out.size,error:''});
        // 目前檔案剛完成時若已有暫停／斷線要求，下一檔仍須等待明確續傳。
        if(index+1<remote.length&&['paused','interrupted'].includes(download.state)){
          await new Promise(resolve=>{download.continueBatch=resolve;renderDownload();});
          download.continueBatch=null;
        }
      }
      Object.assign(download,{state:'complete',pending:false,remaining:[]});renderDownload();
    }catch(error){
      if(generation===state.download.generation){state.download.pending=false;state.download.state='error';state.download.error=error.code||'downloadFailed';renderDownload();}
    }finally{
      if(state.download===download&&download.cancelRequested){download.pending=false;download.state='cancelled';download.error='';if(!state.closed)renderDownload();}
    }
  });
  window.addEventListener('yourdesk-file-progress',event=>{
    const value=event.detail,item=state.download;
    if(state.closed||!item.pending||item.state==='cancelling'||!value||!Number.isSafeInteger(value.received)||!Number.isSafeInteger(value.total)||value.received<0||value.total<value.received)return;
    item.received=value.received;item.total=value.total;
    if(['running','paused','interrupted'].includes(value.state))item.state=value.state;
    if(value.state==='complete')item.received=value.total;
    item.speed=value.speed;item.eta=value.eta;renderDownload();
  });
  async function pauseDownload(reason='paused') {
    const item=state.download;if(!item.pending||item.state==='cancelling')return;
    const generation=item.generation,path=item.path, current=()=>!state.closed&&state.download===item&&item.pending&&item.generation===generation&&item.path===path;
    item.state=reason;renderDownload();
    if(typeof window.yourdeskPauseFile!=='function'){item.error='nativeMissing';renderDownload();return;}
    try{const out=await window.yourdeskPauseFile();if(!current())return;if(Number.isSafeInteger(out.received)&&Number.isSafeInteger(out.total)&&out.received>=0&&out.total>=out.received){item.received=out.received;item.total=out.total;}item.state=state.connected?reason:'interrupted';}
    catch{if(!current())return;item.state='interrupted';}renderDownload();
  }
  $('download-pause').addEventListener('click',()=>pauseDownload());
  $('download-resume').addEventListener('click',async()=>{
    const item=state.download;if(!state.connected||!item.pending||!['paused','interrupted'].includes(item.state))return;
    const generation=item.generation,path=item.path, current=()=>!state.closed&&state.download===item&&item.pending&&item.generation===generation&&item.path===path;
    if(item.continueBatch){item.state='running';item.error='';item.continueBatch();renderDownload();return;}
    if(typeof window.yourdeskResumeFile!=='function'){item.error='nativeMissing';renderDownload();return;}
    $('download-resume').disabled=true;
    try{const out=await window.yourdeskResumeFile();if(!current())return;if(out.resumed!==true)throw new F.FileError('resumeFailed');item.state='running';item.error='';}
    catch{if(!current())return;item.state='interrupted';item.error='resumeFailed';}renderDownload();
  });
  async function cancelDownload() {
    const item=state.download;if(!item.pending||item.state==='cancelling')return;
    ++item.generation;item.cancelRequested=true;item.state='cancelling';renderDownload();
    item.continueBatch?.();
    try{await window.yourdeskCancelFile();}catch{}
    // Native cancellation schedules worker shutdown. Only the original Prepare
    // promise settling releases its slot, so keep new Prepare disabled until then.
  }
  $('download-cancel').addEventListener('click',()=>cancelDownload());
  window.addEventListener('yourdesk-files-session',event=>{
    const next=event.detail?.instance;
    if(state.closed||typeof next!=='string'||!next||next.length>256||next===instance)return;
    interruptUploads();instance=next;state.connected=true;state.reconnected=true;++state.generation;
    if(state.download.pending&&state.download.state==='running'){state.download.state='interrupted';renderDownload();}
    status(t('reconnected'));controls();scheduleStatus();
    // Explicit cancellations may finish after reconnecting, but paused/interrupted
    // transfers stay paused until the user presses Resume.
    if(state.queue.some(item=>item.control.cancelled&&!terminal(item))){
      for(const item of state.queue)if(item.control.cancelled&&!terminal(item)&&!item.active)item.state='cancelling';
      if(state.running)state.cancelPassInstance=instance;
      else void runQueue();
    }
  });
  async function close() {
    if(state.closed||state.closing)return;
    state.closing=true;controls();status(t('closing'));
    for(const item of state.queue)if(!terminal(item)){item.control.cancelled=true;item.state='cancelling';item.error='';renderItem(item);}
    const cleaned=await new Promise(resolve=>{
      let timer;
      const finish=ok=>{clearTimeout(timer);checkClose=null;resolve(ok);};
      checkClose=()=>{
        if(!state.queue.some(item=>!terminal(item))&&!state.download.pending)finish(true);
        else if(!state.connected||(!state.running&&state.queue.some(item=>item.state==='cancelFailed')))finish(false);
      };
      // Keep ownership tokens and the window if cleanup cannot finish promptly.
      timer=setTimeout(()=>finish(false),8000);
      void runQueue();if(state.download.pending)void cancelDownload();checkClose?.();
    });
    if(!cleaned){state.closing=false;controls();status(t('closeFailed'),true);return;}
    state.closed=true;clearTimeout(statusTimer);++state.generation;
    if(typeof window.yourdeskCloseFiles==='function')await window.yourdeskCloseFiles();
  }
  $('files-close').addEventListener('click',()=>void close());
  window.addEventListener('yourdesk-request-close',()=>void close());
  window.addEventListener('keydown',event=>{if((event.metaKey||event.ctrlKey)&&event.key.toLowerCase()==='w'){event.preventDefault();void close();}});
  async function initialize() {
    controls();const initial=instance;
    // Native close interception starts only after the JS close listener exists.
    if(typeof window.yourdeskFilesReady==='function'){
      try{await window.yourdeskFilesReady();}catch{ /* A missing bridge must not block browser initialization. */ }
    }
    if(typeof window.yourdeskFilesSession==='function') {
      try {
        const latest=await window.yourdeskFilesSession();
        if(instance===initial&&typeof latest?.instance==='string'&&latest.instance&&latest.instance.length<=256)instance=latest.instance;
      } catch { /* A stale fragment is rejected by the API; no credentials are exposed. */ }
    }
    let start='';
    try {const location=await call('location');start=F.relative(location.path);}catch{ /* 舊版 Host 沿用原家目錄起點。 */ }
    if(state.closed)return;
    state.initializing=false;controls();void list(start);scheduleStatus();
  }
  void initialize();
})();
