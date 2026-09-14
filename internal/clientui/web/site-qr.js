'use strict';
let siteQRGeneration=0,siteQRLink='';
const siteQRDialog=$('#site-qr-dialog');
siteQRDialog.addEventListener('close',()=>{siteQRGeneration++;siteQRLink='';$('#site-qr-image').removeAttribute('src');$('#site-qr-image').hidden=true;$('#copy-site-link').disabled=true;});
$('#share-site-qr').addEventListener('click',async()=>{
 if(siteQRDialog.open)return;
 const generation=++siteQRGeneration;
 siteQRLink='';$('#copy-site-link').disabled=true;$('#site-qr-image').hidden=true;
 $('#site-qr-name').textContent='';$('#site-qr-room').textContent='';
 $('#site-qr-status').textContent=i18n.t('正在產生 QR Code…');
 openDialog('#site-qr-dialog');
 try{
  const result=await api('site-qr');
  if(generation!==siteQRGeneration||!siteQRDialog.open)return;
  siteQRLink=result.uri;$('#site-qr-image').src=result.image;$('#site-qr-image').hidden=false;
  $('#site-qr-name').textContent=result.name;$('#site-qr-room').textContent=result.room;
  $('#site-qr-status').textContent='';$('#copy-site-link').disabled=false;
 }catch(error){if(generation===siteQRGeneration)$('#site-qr-status').textContent=i18n.t(error.message);}
});
$('#copy-site-link').addEventListener('click',()=>action(async()=>{
 if(!siteQRLink)return;
 await navigator.clipboard.writeText(siteQRLink);toast(i18n.t('站台連結已複製'));
}));
