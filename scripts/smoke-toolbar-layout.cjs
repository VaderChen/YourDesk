const {chromium}=require(process.env.YOURDESK_PLAYWRIGHT_MODULE || 'playwright');
const fs=require('fs');
(async()=>{const b=await chromium.launch({executablePath:process.env.YOURDESK_SMOKE_BROWSER,headless:true});const p=await b.newPage();
const root=require('path').resolve(__dirname,'../internal/clientui/web')+'/';
await p.setContent(fs.readFileSync(root+'index.html','utf8').replace(/<script\b[^>]*>[\s\S]*?<\/script>/g,''));await p.addStyleTag({path:root+'style.css'});
for(const width of [640,800,1280]){await p.setViewportSize({width,height:800});let before;
for(const hidden of [true,false,true]){await p.evaluate(h=>document.querySelector('#stop-incoming').hidden=h,hidden);const boxes=await p.evaluate(()=>Object.fromEntries(['.section-heading','#quick-form','#add-site'].map(sel=>{const r=document.querySelector(sel).getBoundingClientRect();return [sel,{x:r.x,y:r.y,w:r.width,h:r.height}]})));if(!before)before=boxes;else if(JSON.stringify(boxes)!==JSON.stringify(before))throw Error('layout shifted: '+width+' '+JSON.stringify(boxes));}
}await b.close();console.log('PASS: 640/800/1280px show/hide stop icon preserves toolbar geometry');})().catch(e=>{console.error(e);process.exit(1)});
