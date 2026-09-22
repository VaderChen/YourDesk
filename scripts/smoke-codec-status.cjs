// 實際工具列資產，隔離原生橋接；檢查編碼資訊及解碼簡化顯示。
const {chromium}=require('playwright'),path=require('path'),assert=require('assert');
(async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  const page=await browser.newPage({viewport:{width:900,height:400}}),errors=[];
  page.on('pageerror',error=>errors.push(error.message));
  await page.setContent('<script>window.ydTitlebar=async()=>{};<\/script>'+require('./smoke-titlebar-fixture.cjs')());
  await page.addScriptTag({path:path.resolve(__dirname,'../cmd/remote/web/titlebar_windows.js')});
  const rows=await page.evaluate(()=>{
   setTitlebarState({crop:0,sourceEncoding:'software',receiverDecoding:'hardware',videoCodec:'HEVC',audio:{enabled:true,source:{enabled:true,hardware:false,codec:'opus'},receiver:{enabled:true,hardware:false,codec:'opus'}}});
   const rows=document.querySelector('#codec-status').tooltipRows;
   windowsTitlebarBridge({tooltip:'編解碼',rows,x:20,width:320});return rows;
  });
  assert.equal(rows[0].value,'軟體 · HEVC');assert.equal(rows[1].value,'軟體 · Opus');
  assert(rows[2].separator);assert.equal(rows[3].value,'硬體');assert.equal(rows[4].value,'軟體');
  const gap=await page.locator('.win-popup').evaluate(popup=>{
   const line=popup.querySelector('hr').getBoundingClientRect(),values=popup.querySelectorAll('.status-row span');
   return [line.top-values[1].getBoundingClientRect().bottom,values[2].getBoundingClientRect().top-line.bottom];
  });
  assert(Math.abs(gap[0]-gap[1])<1,'分隔線上下間隔須一致：'+gap);
  await page.locator('.win-popup').screenshot({path:path.resolve(__dirname,'../.local-run/codec-status-smoke.png')});
  await page.evaluate(()=>setTitlebarState({crop:0,receiverDecoding:'software',audio:{enabled:false}}));
  assert.equal(await page.locator('#codec-status').evaluate(el=>el.tooltipRows[4].value),'關閉');
  assert.deepEqual(errors,[]);console.log('PASS: 編碼保留格式、解碼僅硬體／軟體、關閉狀態及分隔線上下等距');
 }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
