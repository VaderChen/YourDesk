'use strict';
let pendingTerminalSite=null, terminalSession=null;
function chooseConnection(site){
 $('#connection-choice-name').textContent=site.name;
 $('#connection-choice-desktop').disabled=siteCapability(site,'desktop')===false;
 $('#connection-choice-terminal').disabled=siteCapability(site,'terminal')===false;
 $('#connection-choice-desktop').onclick=()=>{$('#connection-choice').close();connect(site,false)};
 $('#connection-choice-terminal').onclick=()=>{$('#connection-choice').close();connect(site,true)};
 openDialog('#connection-choice');
}
$('#connection-choice-close').onclick=()=>$('#connection-choice').close();
async function openTerminal(site){
 try { await api('terminal/window','POST',{session:`viewer:${site.id}`}); }
 catch(error){toast(i18n.t(error.message),true);await api('stop','POST',{key:`viewer:${site.id}`}).catch(()=>{});}
}
