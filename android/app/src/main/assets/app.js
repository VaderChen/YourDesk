'use strict';
const strings={
 'zh-Hant':{brand:'YourDesk',address:'貼上遠端 ID 或 IP',connect:'連線 ↗',add:'＋ 新增站台',all:'所有站台',search:'搜尋名稱、裝置 ID 或備註',empty:'沒有符合的站台',preview:'',count:'{n} 個站台',close:'關閉',delete:'刪除',edit:'編輯',terminal:'終端機',desktop:'遠端桌面',online:'上線',offline:'離線'},
 en:{brand:'YourDesk',address:'Remote ID or IP',connect:'Connect ↗',add:'+ Add site',all:'All sites',search:'Search name, device ID or notes',empty:'No matching sites',preview:'',count:'{n} sites',close:'Close',delete:'Delete',edit:'Edit',terminal:'Terminal',desktop:'Remote desktop',online:'Online',offline:'Offline'},
 ja:{brand:'YourDesk',address:'リモート ID または IP',connect:'接続 ↗',add:'＋ サイト追加',all:'すべてのサイト',search:'名前、ID、メモを検索',empty:'該当するサイトなし',preview:'',count:'{n} サイト',close:'閉じる',delete:'削除',edit:'編集',terminal:'ターミナル',desktop:'リモートデスクトップ',online:'オンライン',offline:'オフライン'},
 ko:{brand:'YourDesk',address:'원격 ID 또는 IP',connect:'연결 ↗',add:'＋ 사이트 추가',all:'모든 사이트',search:'이름, ID 또는 메모 검색',empty:'일치하는 사이트 없음',preview:'',count:'{n}개 사이트',close:'닫기',delete:'삭제',edit:'편집',terminal:'터미널',desktop:'원격 데스크톱',online:'온라인',offline:'오프라인'}
};
const requested=(navigator.language||'zh-Hant').toLowerCase();
const autoLanguage=requested.startsWith('zh')?'zh-Hant':strings[requested.slice(0,2)]?requested.slice(0,2):'en';
const language={value:autoLanguage};
const t=key=>(strings[language.value]||strings['zh-Hant'])[key]||key;
const element=id=>document.getElementById(id);
let sites=[];
function loadSites(){
 const saved=JSON.parse(window.YourDesk?.loadSites?.()||'[]');
 if(!Array.isArray(saved))throw Error('站台資料格式無效');
 // 保留原有資料；格式損壞時顯示錯誤，不以空清單覆寫。
 sites=saved.map(value=>{
  try{return SiteModel.site(value);}catch{
   // 舊版曾允許無效信令位址；保留該站台供使用者修正或刪除，不阻擋整份清單。
   return {id:String(value?.id||''),name:String(value?.name||value?.id||'待修正站台'),note:'設定無效，請編輯或刪除此站台',signal:String(value?.signal||''),terminal:false,desktop:false};
  }
 });
}
const paths={desktop:'<rect x="3" y="3" width="18" height="13" rx="1"/><path d="M8 21h8M10 16v5m4-5v5"/>',terminal:'<rect x="3" y="4" width="18" height="16" rx="1"/><path d="m6 8 4 4-4 4m7 0h4"/>',edit:'<path d="m4 15 12-12 5 5-12 12H4zm10-10 5 5"/>',delete:'<path d="M4 6h16M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7m4-7v7"/>'};
const icon=key=>`<svg viewBox="0 0 24 24" aria-hidden="true">${paths[key]}</svg>`;
window.showNotice=message=>{element('message').textContent=message;element('notice').hidden=false;};
const languageSelect=element('language-select'),languageOptions=element('language-options');
languageSelect.onclick=()=>{languageOptions.hidden=!languageOptions.hidden;};
languageOptions.querySelectorAll('button').forEach(button=>button.onclick=()=>{
 language.value=button.dataset.value==='auto'?autoLanguage:button.dataset.value;
 languageSelect.textContent=button.textContent;languageOptions.hidden=true;render();
});
function render(){
 document.documentElement.lang=language.value;
 document.querySelectorAll('[data-i18n]').forEach(el=>el.textContent=t(el.dataset.i18n));
 document.querySelectorAll('[data-placeholder]').forEach(el=>{el.placeholder=t(el.dataset.placeholder);el.setAttribute('aria-label',el.placeholder);});
 const query=element('search').value.trim().toLocaleLowerCase();
 const filtered=sites.filter(site=>[site.name,site.note,site.id,site.signal].join(' ').toLocaleLowerCase().includes(query));
 element('count').textContent=t('count').replace('{n}',filtered.length);
 element('empty').hidden=filtered.length!==0;
 const list=element('sites');list.replaceChildren();
 for(const site of filtered){
  const card=document.createElement('article');card.className='site';
  card.innerHTML=`<div class="identity"><span class="device">${icon('desktop')}</span><div><div class="name"></div><div class="note"></div></div></div><div class="identifier"></div><div class="actions"></div>`;
  card.querySelector('.name').textContent=site.name;card.querySelector('.note').textContent=site.note;
  card.querySelector('.identifier').textContent=site.id;
  for(const action of ['delete','edit','terminal','desktop']){
   const button=document.createElement('button');button.type='button';button.className=action;button.innerHTML=icon(action);button.title=t(action);button.setAttribute('aria-label',`${t(action)} · ${site.name}`);
   button.disabled=(action==='terminal'||action==='desktop')&&!site[action];
   button.onclick=()=>{
    if(action==='terminal'||action==='desktop')openConnection(site.id,action==='terminal'?'shell':'desktop',site.signal);
    else if(action==='edit')openSite(site);
    else {deleting=site;element('delete-name').textContent=site.name;element('delete-status').textContent='';element('delete-dialog').hidden=false;}
   };card.querySelector('.actions').append(button);
  }
  list.append(card);
 }
}
element('search').oninput=render;
element('quick-form').onsubmit=event=>{event.preventDefault();openConnection(element('address').value.trim(),element('quick-mode').value);};
const siteDialog=element('site-dialog'),siteForm=element('site-form');
const siteTabs=siteForm.querySelectorAll('[data-tab]');
let siteTab='manual',editing='',deleting=null;
function selectSiteTab(tab){
 siteTab=tab;siteTabs.forEach(button=>button.classList.toggle('active',button.dataset.tab===tab));
 element('site-manual').hidden=tab!=='manual';element('site-qrcode').hidden=tab!=='qrcode';
 element('site-name').required=tab==='manual';element('site-room').required=tab==='manual';
 if(tab==='qrcode'){element('site-status').textContent='正在啟動相機掃描…';window.YourDesk?.startQrScanner?.();}
 else window.YourDesk?.stopQrScanner?.();
}
function fillSite(site){for(const [id,key] of [['site-name','name'],['site-room','id'],['site-signal','signal'],['site-note','note']])element(id).value=site[key]||'';}
function openSite(site){
 siteForm.reset();editing=site?SiteModel.identity(site):'';element('site-title').textContent=site?'編輯站台':'新增站台';
 element('site-secret').placeholder=site?'留白保留原端點密碼':'選填，也可連線時輸入';
 if(site)fillSite(site);
 selectSiteTab('manual');element('site-status').textContent='';siteDialog.hidden=false;window.YourDesk?.setSiteDialogVisible?.(true);
}
function closeSite(){
 siteDialog.hidden=true;selectSiteTab('manual');element('site-secret').value='';element('site-qr').value='';element('site-camera').removeAttribute('src');
 window.YourDesk?.setSiteDialogVisible?.(false);
}
window.cameraFrame=value=>{if(!siteDialog.hidden&&siteTab==='qrcode')element('site-camera').src=value;};
window.cameraStatus=message=>{if(!siteDialog.hidden)element('site-status').textContent=message;};
window.qrCodeDetected=value=>{
 if(siteDialog.hidden||siteTab!=='qrcode')return;
 try{const site=SiteModel.qr(value);fillSite(site);element('site-qr').value=value;selectSiteTab('manual');element('site-status').textContent='已讀取站台，請確認名稱與信令伺服器後儲存。';}
 catch(error){element('site-status').textContent=error.message+'；可按重新掃描。';}
};
siteTabs.forEach(button=>button.onclick=()=>selectSiteTab(button.dataset.tab));
element('scan-again').onclick=()=>selectSiteTab('qrcode');
element('add').onclick=()=>openSite(null);
element('site-cancel').onclick=closeSite;
siteForm.onsubmit=event=>{
 event.preventDefault();
 try{
  const site=siteTab==='qrcode'?SiteModel.qr(element('site-qr').value):SiteModel.site({id:element('site-room').value,name:element('site-name').value,note:element('site-note').value,signal:element('site-signal').value});
  if(sites.some(item=>SiteModel.identity(item)!==editing&&SiteModel.identity(item)===SiteModel.identity(site)))throw Error('此站台已存在');
  const result=window.YourDesk?.saveSite?.(JSON.stringify(site),editing,element('site-secret').value,element('site-forget').checked);
  if(result!== '')throw Error(result||'站台儲存失敗，請重試');
  loadSites();closeSite();render();
 }catch(error){element('site-status').textContent=error.message;}
};
element('delete-cancel').onclick=()=>{element('delete-dialog').hidden=true;deleting=null;};
element('delete-confirm').onclick=()=>{
 try{
  if(!deleting||window.YourDesk?.deleteSite?.(SiteModel.identity(deleting))!==true)throw Error('刪除失敗，請重試');
  loadSites();element('delete-dialog').hidden=true;deleting=null;render();
 }catch(error){element('delete-status').textContent=error.message;}
};
element('notice-close').onclick=()=>{element('notice').hidden=true;};
window.dismissOverlay=()=>{
 if(!element('notice').hidden){element('notice').hidden=true;return true;}
 if(!element('delete-dialog').hidden){element('delete-cancel').click();return true;}
 if(!siteDialog.hidden){closeSite();return true;}
 if(!element('connect-progress').hidden){element('connect-abort').click();return true;}
 if(!element('connect-overlay').hidden){element('connect-cancel').click();return true;}
 if(!languageOptions.hidden){languageOptions.hidden=true;return true;}
 return false;
};
window.addEventListener('pagehide',()=>{window.YourDesk?.stopQrScanner?.();element('site-secret').value='';});
try{loadSites();}catch{showNotice('站台資料無法讀取，原有資料已保留。');element('add').disabled=true;}
try{element('app-version').textContent=window.YourDesk?.appVersion?.()||'';}catch{}
render();
