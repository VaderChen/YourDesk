// 全尺寸 ANSI 鍵盤；按鍵名稱共用 rawkey 對照，不以文字輸入代替實體鍵。
(() => {
 const css=`
 .vk{font:12px -apple-system,'Segoe UI',sans-serif;color:CanvasText;background:Canvas;padding:12px;box-sizing:border-box;min-width:690px}
 .vk *{box-sizing:border-box}.vk .vk-head{display:flex;align-items:center;gap:12px;margin-bottom:10px}.vk .vk-head{cursor:move;touch-action:none;user-select:none}.vk.vk-numeric{min-width:0}.vk.vk-numeric .vk-body{display:block}.vk.vk-numeric .vk-function,.vk.vk-numeric .vk-main,.vk.vk-numeric .vk-nav{display:none}.vk .vk-head strong{margin-right:auto}.vk .vk-head select{font:inherit;padding:4px}
 .vk .vk-body{display:grid;grid-template-columns:minmax(430px,15fr) minmax(100px,3fr) minmax(135px,4fr);gap:10px}
 .vk .vk-row{display:flex;gap:3px;margin-bottom:4px}.vk button{display:flex;align-items:center;justify-content:center;min-width:0;width:auto;height:34px;flex:1;padding:0 3px;font:inherit;border:1px solid #8886;border-radius:4px;color:CanvasText;background:ButtonFace;cursor:pointer;white-space:nowrap}
 .vk button:hover{background:#739d8d44}.vk button[aria-pressed=true]{background:#216b50;color:white;border-color:#216b50}.vk button:focus-visible{outline:2px solid #216b50}.vk button:disabled{opacity:.4;cursor:default}
 .vk .vk-function{display:flex;gap:4px;margin-bottom:10px}.vk .vk-function button{height:28px;font-size:11px}.vk .vk-nav,.vk .vk-pad{display:grid;grid-template-columns:repeat(3,1fr);grid-auto-rows:34px;gap:4px}.vk .vk-pad{grid-template-columns:repeat(4,1fr)}
 .vk .vk-close{flex:none;width:30px;height:28px}.vk .vk-empty{visibility:hidden}
 `;
 function mount(root,request,send,labels,ui=()=>{}){
  root.className='vk';let platform=request.platform==='darwin'?'darwin':'windows',mods=0,caps=false,layout=platform;
  const head=document.createElement('div');head.className='vk-head';
  const title=document.createElement('strong');title.textContent=labels.title;
  const select=document.createElement('select');select.setAttribute('aria-label',labels.title);
  for(const [value,text] of [['darwin','Mac'],['windows','Windows'],['zhuyin',labels.zhuyin],['numeric',labels.numeric]]){
   if(value==='zhuyin'||value==='numeric')select.append(document.createElement('hr'));
   const o=document.createElement('option');o.value=value;o.textContent=text;select.append(o);
  }select.value=platform;
  const close=document.createElement('button');close.className='vk-close';close.textContent='×';close.setAttribute('aria-label',labels.close);close.onclick=()=>send(42);
  head.append(title,select,close);root.append(head);
  let drag=null;
  head.addEventListener('pointerdown',e=>{
   if(e.button!==0||e.target.closest('button,select'))return;
   e.preventDefault();head.setPointerCapture(e.pointerId);drag={x:e.screenX,y:e.screenY};ui({keyboardDrag:'start'});
  });
  head.addEventListener('pointermove',e=>{if(drag)ui({keyboardDrag:'move',dx:e.screenX-drag.x,dy:e.screenY-drag.y})});
  const endDrag=()=>{if(drag){drag=null;ui({keyboardDrag:'end'})}};
  head.addEventListener('pointerup',endDrag);head.addEventListener('pointercancel',endDrag);head.addEventListener('lostpointercapture',endDrag);
  const zhuyin={ '1':'ㄅ',q:'ㄆ',a:'ㄇ',z:'ㄈ','2':'ㄉ',w:'ㄊ',s:'ㄋ',x:'ㄌ',e:'ㄍ',d:'ㄎ',c:'ㄏ',r:'ㄐ',f:'ㄑ',v:'ㄒ','5':'ㄓ',t:'ㄔ',g:'ㄕ',b:'ㄖ',y:'ㄗ',h:'ㄘ',n:'ㄙ',u:'ㄧ',j:'ㄨ',m:'ㄩ','8':'ㄚ',i:'ㄛ',k:'ㄜ',comma:'ㄝ','9':'ㄞ',o:'ㄟ',l:'ㄠ',period:'ㄡ','0':'ㄢ',p:'ㄣ',semicolon:'ㄤ',slash:'ㄥ',minus:'ㄦ','3':'ˇ','4':'ˋ','6':'ˊ','7':'˙'};
  const keys=document.createElement('div');root.append(keys);
  function reset(){mods=0;root.querySelectorAll('[data-mod]').forEach(b=>b.setAttribute('aria-pressed','false'))}
  function key(name,label,width=1,bit=0){
   const b=document.createElement('button');b.type='button';b.dataset.key=name;b.textContent=(label||name.toUpperCase())+(layout==='zhuyin'&&zhuyin[name]?' '+zhuyin[name]:'');b.style.flex=String(width);
   const code=request.codes[platform][name];b.disabled=code===undefined&&!bit;
   if(bit || name==='capslock')b.setAttribute('aria-pressed',String(name==='capslock'&&caps));
   b.onclick=()=>{
    if(bit){mods^=bit;root.querySelectorAll('[data-mod]').forEach(k=>k.setAttribute('aria-pressed',String(!!(mods&Number(k.dataset.mod)))));return}
    if(name==='capslock'){caps=!caps;b.setAttribute('aria-pressed',String(caps))}
    send(1000+(platform==='windows'?8192:0)+(caps?16384:0)+(mods&15)*512+(mods&32?32768:0)+code);reset();
   };
   if(bit)b.dataset.mod=bit;
   return b;
  }
  function render(){
   reset();root.classList.toggle('vk-numeric',layout==='numeric');keys.replaceChildren();const mac=platform==='darwin';
   const functions=document.createElement('div');functions.className='vk-function';functions.append(key('escape','Esc'));
   for(let i=1;i<=(mac?19:12);i++)functions.append(key('f'+i,'F'+i));
   if(!mac)for(const [n,l] of [['printscreen','PrtSc'],['scrolllock','ScrLk'],['pause','Pause']])functions.append(key(n,l));
   keys.append(functions);
   const body=document.createElement('div');body.className='vk-body';keys.append(body);
   const main=document.createElement('div');main.className='vk-main';body.append(main);
   const rows=[
    [['graveaccent','`'],...Array.from('1234567890',n=>[n,n]),['minus','−'],['equal','='],['backspace',mac?'Delete':'Backspace',2]],
    [['tab','Tab',1.5],...Array.from('qwertyuiop',n=>[n]),['leftbracket','['],['rightbracket',']'],['backslash','\\',1.5]],
    [['capslock','Caps Lock',1.8],...Array.from('asdfghjkl',n=>[n]),['semicolon',';'],['apostrophe',"'"],['enter',mac?'Return':'Enter',2.2]],
    [['leftshift','Shift',2.3,1],...Array.from('zxcvbnm',n=>[n]),['comma',','],['period','.'],['slash','/'],['rightshift','Shift',2.7,1]],
    mac?[['leftcontrol','Control',1.5,2],['leftalt','Option',1.5,4],['leftsuper','⌘',1.5,8],['space','',6],['rightsuper','⌘',1.5,8],['rightalt','Option',1.5,4],['rightcontrol','Control',1.5,2]]:
    [['leftcontrol','Ctrl',1.4,2],['leftsuper','Win',1.2,8],['leftalt','Alt',1.2,4],['space','',6],['rightalt','Alt',1.2,4],['rightsuper','Win',1.2,8],['menu','Menu',1.2],['rightcontrol','Ctrl',1.4,2]]
   ];
   rows.forEach(row=>{const el=document.createElement('div');el.className='vk-row';row.forEach(args=>el.append(key(...args)));main.append(el)});
   const nav=document.createElement('div');nav.className='vk-nav';body.append(nav);
   for(const [name,label] of [[mac?'fn':'insert',mac?'Fn':'Insert'],['home','Home'],['pageup','PgUp'],['delete','Del'],['end','End'],['pagedown','PgDn'],['',''],['',''],['',''],['',''],['up','↑'],['',''],['left','←'],['down','↓'],['right','→']]){const b=key(name,label,1,name==='fn'?32:0);if(!name)b.className='vk-empty';nav.append(b)}
   const pad=document.createElement('div');pad.className='vk-pad';body.append(pad);
   const padRows=mac?[
    ['numlock','Clear'],['kpequal','='],['kpdivide','/'],['kpmultiply','×'],['kp7','7'],['kp8','8'],['kp9','9'],['kpsubtract','−'],['kp4','4'],['kp5','5'],['kp6','6'],['kpadd','+'],['kp1','1'],['kp2','2'],['kp3','3'],['kpenter','Enter',1,2],['kp0','0',2],['kpdecimal','.']
   ]:[['numlock','Num'],['kpdivide','/'],['kpmultiply','×'],['kpsubtract','−'],['kp7','7'],['kp8','8'],['kp9','9'],['kpadd','+',1,2],['kp4','4'],['kp5','5'],['kp6','6'],['kp1','1'],['kp2','2'],['kp3','3'],['kpenter','Enter',1,2],['kp0','0',2],['kpdecimal','.']];
   padRows.forEach(([n,l,w=1,h=1])=>{const b=key(n,l);if(w>1)b.style.gridColumn='span '+w;if(h>1){b.style.gridRow='span '+h;b.style.height='auto'}pad.append(b)});
  }
  select.onchange=()=>{layout=select.value;if(layout==='darwin'||layout==='windows')platform=layout;render();ui({keyboardLayout:layout})};render();
  const notice=document.createElement('div');notice.setAttribute('role','status');notice.hidden=true;notice.style.cssText='font-size:12px;line-height:1.5;margin-top:8px;white-space:normal';root.append(notice);
  const status=message=>{notice.textContent=message;notice.hidden=!message};window.virtualKeyboardStatus=status;
  root.addEventListener('keydown' ,e=>{if(e.key==='Escape'){e.preventDefault();send(42)}});
  window.addEventListener('blur',reset);
  const observer=new ResizeObserver(()=>ui({keyboardHeight:Math.ceil(root.getBoundingClientRect().height)}));observer.observe(root);
  return ()=>{observer.disconnect();window.removeEventListener('blur',reset);if(window.virtualKeyboardStatus===status)delete window.virtualKeyboardStatus};
 }
 const labels=()=>({title:t('虛擬鍵盤'),numeric:t('數字鍵盤'),zhuyin:t('注音鍵盤'),close:t('取消')});
 window.mountVirtualKeyboard=(root,request,send,ui)=>{const style=document.createElement('style');style.textContent=css;root.append(style);const content=document.createElement('div');root.append(content);return mount(content,request,send,labels(),ui)};
 window.virtualKeyboardDocument=request=>{
  const json=value=>JSON.stringify(value).replace(/</g,'\\u003c');
  return '<!doctype html><meta charset="utf-8"><style>:root{color-scheme:light dark}body{margin:0}'+css+'</style><div id="keyboard"></div><script>('+mount.toString()+')(document.getElementById("keyboard"),'+json(request)+',action=>window.webkit.messageHandlers.titlebar.postMessage(action),'+json(labels())+',message=>window.webkit.messageHandlers.titlebar.postMessage(message));<'+ '/script>';
 };
})();
