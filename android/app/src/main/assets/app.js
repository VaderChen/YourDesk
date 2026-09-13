'use strict';
const strings={
 'zh-Hant':{brand:'YourDesk',address:'貼上遠端 ID 或 IP',connect:'連線 ↗',add:'＋ 新增站台',all:'所有站台',search:'搜尋名稱、裝置 ID 或備註',empty:'沒有符合的站台',preview:'介面預覽 · 範例站台',count:'{n} 個站台',close:'關閉',pending:'遠端連線尚未接入，目前為介面預覽。',manage:'站台管理尚未接入，目前為範例資料。',delete:'刪除',edit:'編輯',terminal:'終端機',desktop:'遠端桌面',online:'上線',offline:'離線'},
 en:{brand:'YourDesk',address:'Remote ID or IP',connect:'Connect ↗',add:'+ Add site',all:'All sites',search:'Search name, device ID or notes',empty:'No matching sites',preview:'UI preview · Sample sites',count:'{n} sites',close:'Close',pending:'Remote connections are not connected yet. This is a UI preview.',manage:'Site management is not connected yet. These are sample sites.',delete:'Delete',edit:'Edit',terminal:'Terminal',desktop:'Remote desktop',online:'Online',offline:'Offline'},
 ja:{brand:'YourDesk',address:'リモート ID または IP',connect:'接続 ↗',add:'＋ サイト追加',all:'すべてのサイト',search:'名前、ID、メモを検索',empty:'該当するサイトなし',preview:'画面プレビュー · サンプル',count:'{n} サイト',close:'閉じる',pending:'リモート接続は未実装です。画面プレビューです。',manage:'サイト管理は未実装です。サンプルデータです。',delete:'削除',edit:'編集',terminal:'ターミナル',desktop:'リモートデスクトップ',online:'オンライン',offline:'オフライン'},
 ko:{brand:'YourDesk',address:'원격 ID 또는 IP',connect:'연결 ↗',add:'＋ 사이트 추가',all:'모든 사이트',search:'이름, ID 또는 메모 검색',empty:'일치하는 사이트 없음',preview:'화면 미리보기 · 예제 사이트',count:'{n}개 사이트',close:'닫기',pending:'원격 연결은 아직 구현되지 않았습니다.',manage:'사이트 관리는 아직 구현되지 않았습니다. 예제 데이터입니다.',delete:'삭제',edit:'편집',terminal:'터미널',desktop:'원격 데스크톱',online:'온라인',offline:'오프라인'}
};
const t=key=>(strings[language.value]||strings['zh-Hant'])[key]||key;
const sites=[
 {name:'本機',note:'公司的',id:'YD-4U3T-KO7Y-UHKE-MHFI-A6SA',online:true,terminal:true,desktop:true},
 {name:'家裡的Mac',note:'家裡的',id:'YD-UE6E-HUVB-MVH5-K6E4-UGRA',online:true,terminal:true,desktop:true},
 {name:'公司 Windows',note:'公司的',id:'YD-DNMM-CASI-J5SG-OMVI-VTRQ',online:true,terminal:false,desktop:true},
 {name:'BBS 主機',note:'雲端服務',id:'YD-WQNA-FYZZ-BWCB-QJXN-SC3Q',online:true,terminal:true,desktop:false},
 {name:'我的筆電',note:'公司的',id:'YD-4TGE-BOZI-S2E2-BKBL-HD6A',online:false,terminal:false,desktop:false}
];
const paths={desktop:'<rect x="3" y="3" width="18" height="13" rx="1"/><path d="M8 21h8M10 16v5m4-5v5"/>',terminal:'<rect x="3" y="4" width="18" height="16" rx="1"/><path d="m6 8 4 4-4 4m7 0h4"/>',edit:'<path d="m4 15 12-12 5 5-12 12H4zm10-10 5 5"/>',delete:'<path d="M4 6h16M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7m4-7v7"/>'};
const icon=key=>`<svg viewBox="0 0 24 24" aria-hidden="true">${paths[key]}</svg>`;
const requested=(navigator.language||'zh-Hant').toLowerCase(); const autoLanguage=requested.startsWith('zh')?'zh-Hant':strings[requested.slice(0,2)]?requested.slice(0,2):'en'; const language={value:autoLanguage}; const languageSelect=document.getElementById('language-select'); const languageOptions=document.getElementById('language-options'); languageSelect.onclick=()=>{languageOptions.hidden=!languageOptions.hidden;}; languageOptions.querySelectorAll('button').forEach(b=>b.onclick=()=>{language.value=b.dataset.value==='auto'?autoLanguage:b.dataset.value;languageSelect.textContent=b.textContent;languageOptions.hidden=true;render();});
function notice(key){document.getElementById('message').textContent=t(key);document.getElementById('notice').hidden=false;}
function render(){
 document.documentElement.lang=language.value;
 document.querySelectorAll('[data-i18n]').forEach(el=>el.textContent=t(el.dataset.i18n));
 document.querySelectorAll('[data-placeholder]').forEach(el=>{el.placeholder=t(el.dataset.placeholder);el.setAttribute('aria-label',el.placeholder);});
 const query=document.getElementById('search').value.trim().toLocaleLowerCase();
 const filtered=sites.filter(site=>[site.name,site.note,site.id].join(' ').toLocaleLowerCase().includes(query));
 document.getElementById('count').textContent=t('count').replace('{n}',filtered.length);
 document.getElementById('empty').hidden=filtered.length!==0;
 const list=document.getElementById('sites');list.replaceChildren();
 for(const site of filtered){
  const card=document.createElement('article');card.className='site'+(site.online?' online':'');
  card.innerHTML=`<div class="identity"><span class="device">${icon('desktop')}</span><div><div class="name"></div><div class="note"></div></div></div><div class="identifier"></div><div class="actions"></div>`;
  card.querySelector('.name').textContent=site.name;card.querySelector('.note').textContent=site.note;
  card.querySelector('.device').setAttribute('aria-label',t(site.online?'online':'offline'));
  card.querySelector('.identifier').textContent=site.id;
  for(const action of ['delete','edit','terminal','desktop']){
   const button=document.createElement('button');button.type='button';button.className=action;button.innerHTML=icon(action);button.title=t(action);button.setAttribute('aria-label',`${t(action)} · ${site.name}`);
   button.disabled=(action==='terminal'||action==='desktop')&&!site[action];
   button.onclick=()=>{if(action==='terminal'&&site[action]){openConnection(site.id,'shell');return} if(action==='desktop'&&site[action]){openConnection(site.id,'desktop');return} notice(action==='delete'||action==='edit'?'manage':'pending')};card.querySelector('.actions').append(button);
  }
  list.append(card);
 }
}
document.getElementById('search').oninput=render;
document.getElementById('quick-form').onsubmit=event=>{event.preventDefault();openConnection(document.getElementById('address').value.trim(),'shell');};
document.getElementById('add').onclick=()=>notice('manage');document.getElementById('notice-close').onclick=()=>{document.getElementById('notice').hidden=true;};render();
