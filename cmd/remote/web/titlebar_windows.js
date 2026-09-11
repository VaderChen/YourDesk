// Windows 共用 Mac 的工具列內容；僅視窗按鈕、原生橋接與泡泡容器因平台而異。
(() => {
 'use strict';
 document.documentElement.classList.add('windows');
 const style=document.createElement('style');
 style.textContent=`
 html.windows, .windows body{height:100%;background:transparent;font-family:'Segoe UI',sans-serif}
 .windows header{background:var(--bg)}
 .windows .title{padding-left:14px}
 .windows .window{width:138px;padding:0;gap:0;align-self:stretch;border-left:1px solid var(--edge)}
 .windows .window button{width:46px;height:51px;border:0;border-radius:0;background:transparent;color:var(--ink);font-size:16px;line-height:1}
 .windows .window button:hover{background:var(--hover)}
 .windows .window .close:hover{background:#c42b1c;color:white}
 .windows .actions{width:158px;padding-right:10px}
 .win-popup{position:fixed;z-index:10;min-width:190px;max-width:calc(100vw - 20px);padding:6px;background:var(--bg);border:1px solid var(--edge);border-radius:12px;box-shadow:0 4px 10px #0003;overflow:auto;max-height:calc(100vh - 72px)}
 .win-popup[hidden]{display:none}
 .win-popup.tip{padding:12px 16px;min-width:0;white-space:pre-line;font-size:12px;line-height:1.6;pointer-events:none}
 .win-popup button{display:flex;align-items:center;gap:12px;width:100%;height:34px;text-align:left;padding:0 10px}
 .win-popup button .check{width:16px;color:var(--accent)}
 .win-popup button[aria-checked=true]{background:var(--hover)}
 .win-popup hr{border:0;border-top:1px solid var(--edge);margin:6px 4px}
 .win-popup .status-row{display:grid;grid-template-columns:auto 1fr;gap:18px;white-space:nowrap}
 .win-popup .status-row b{font-weight:600;color:var(--muted)}
 @media(max-width:760px){.windows .traffic{width:157px;gap:7px;padding-right:8px}.windows .traffic output{min-width:50px}.windows .fps{width:60px;margin-right:8px;padding-right:8px}.windows .title{padding-left:8px;padding-right:6px}}
 @media(max-width:640px){.windows .title svg{display:none}.windows .window{width:108px}.windows .window button{width:36px}.windows .actions{width:140px;gap:3px}.windows .title{font-size:11px}}
 `;
 document.head.append(style);
 const controls=document.querySelector('.window');
 const close=controls.querySelector('.close'), minimize=controls.querySelector('.minimize'), maximize=controls.querySelector('.maximize');
 minimize.textContent='−';maximize.innerHTML='<svg viewBox="0 0 20 20"><rect x="4" y="4" width="12" height="12"/></svg>';
 maximize.dataset.action='8';maximize.dataset.tip='最大化／還原';maximize.setAttribute('aria-label','最大化／還原');
 close.textContent='×';controls.replaceChildren(minimize,maximize,close);document.querySelector('header').append(controls);
 const popup=document.createElement('div');popup.className='win-popup';popup.hidden=true;document.body.append(popup);
 let state={},kind='';
 const tr=source=>(state.strings||{})[source]||source;
 const send=message=>window.ydTitlebar(message).catch(()=>{});
 const publishRegion=()=>{
  if(popup.hidden){send({popup:true,menu:false,width:0,height:0});return}
  const r=popup.getBoundingClientRect();
  send({popup:true,menu:kind==='menu',x:r.x,y:r.y,width:r.width,height:r.height});
 };
 window.closeWindowsPopup=()=>{popup.hidden=true;kind='';publishRegion()};
 const position=rect=>{
  popup.hidden=false;popup.style.left='10px';popup.style.top='62px';
  popup.style.left=Math.max(10,Math.min(rect.right-popup.offsetWidth,innerWidth-popup.offsetWidth-10))+'px';
  publishRegion();
 };
 window.windowsTitlebarBridge=message=>{
  if(typeof message==='number'){send({action:message});return}
  if(message.menu){
   window.closeWindowsPopup();
   const rect=document.querySelector('[data-menu="'+message.menu+'"]').getBoundingClientRect();
   send({nativeMenu:message.menu,x:rect.right,y:rect.bottom+10});return;
  }
  if('tooltip' in message){
   if(!message.tooltip){if(kind==='tooltip')window.closeWindowsPopup();return}
   kind='tooltip';popup.className='win-popup tip';popup.setAttribute('role','status');popup.replaceChildren();
   if(Array.isArray(message.rows)) message.rows.forEach(row=>{
    const line=document.createElement('div');line.className='status-row';const label=document.createElement('b'),value=document.createElement('span');label.textContent=row.label;value.textContent=row.value;line.append(label,value);popup.append(line);
   }); else popup.textContent=message.tooltip;
   position({right:message.x+message.width});
  }
 };
 window.updateWindowsTitlebar=value=>{
  state=value;
  maximize.innerHTML=state.maximized?'<svg viewBox="0 0 20 20"><path d="M7 4h9v9M4 7h9v9H4z"/></svg>':'<svg viewBox="0 0 20 20"><rect x="4" y="4" width="12" height="12"/></svg>';
  maximize.setAttribute('aria-disabled',String(!!state.fullscreen));
 };
 document.addEventListener('pointerdown',event=>{
  if(event.button===0&&event.target.closest('header')&&!event.target.closest('button')){window.closeWindowsPopup();send({action:9});event.preventDefault()}
 });
 document.querySelector('.title').addEventListener('dblclick',()=>send({action:8}));
 document.addEventListener('keydown',event=>{if(event.key==='Escape')window.closeWindowsPopup()});
 window.addEventListener('blur',window.closeWindowsPopup);
 window.addEventListener('resize',window.closeWindowsPopup);
 document.addEventListener('contextmenu',event=>event.preventDefault());
 window.addEventListener('DOMContentLoaded',()=>send({ready:true}));
})();
