// 驗證實際 UI 事件註冊的雙向互斥，不修改使用者設定。
const fs=require('fs'),vm=require('vm'),assert=require('assert');
const source=fs.readFileSync(require('path').join(__dirname,'../internal/clientui/web/app.js'),'utf8');
const controls=new Map();let saves=0;
const $=id=>{if(!controls.has(id))controls.set(id,{checked:false,addEventListener(type,fn){this.change=fn}});return controls.get(id)};
const lines=source.split('\n').filter(line=>line.includes("addEventListener('change'")&&(line.startsWith("$('#ui-auto-reconnect')")||line.startsWith("$('#ui-close-on-disconnect')"))).join('\n');
assert(lines.includes('ui-auto-reconnect'));
vm.runInNewContext(lines,{$,savePreferences:()=>saves++});
$('#ui-close-on-disconnect').checked=true;
$('#ui-auto-reconnect').checked=true;$('#ui-auto-reconnect').change();
assert.equal($('#ui-close-on-disconnect').checked,false);
$('#ui-close-on-disconnect').checked=true;$('#ui-close-on-disconnect').change();
assert.equal($('#ui-auto-reconnect').checked,false);
assert.equal(saves,2);
console.log('PASS 自動重連與斷線關閉雙向互斥');
