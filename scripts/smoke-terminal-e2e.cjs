const {chromium}=require(process.env.YOURDESK_PLAYWRIGHT_MODULE);
(async()=>{
 const browser=await chromium.launch({executablePath:process.env.YOURDESK_SMOKE_BROWSER,headless:true});
 try{
  const pages=[],errors=[];
  for(const id of ['one','two']){
   const page=await browser.newPage();pages.push(page);page.on('pageerror',e=>errors.push(e.message));
   await page.goto(`${process.env.YOURDESK_TERMINAL_TEST_URL}/terminal.html#token=local-e2e-token&session=viewer%3A${id}&instance=${id}&name=${id}&language=zh-Hant`);
   await page.waitForFunction(()=>document.querySelector('#terminal-status').textContent.includes('已連線'),{timeout:15000});
   await page.locator('.xterm-helper-textarea').pressSequentially(`printf 'HELLO_${id}_%s\\n' OK`,{delay:1});await page.keyboard.press('Enter');
   await page.waitForFunction(id=>document.querySelector('.xterm-rows')?.textContent.includes(`HELLO_${id}_OK`),id);
  }
  await pages[0].click('#terminal-close');
  await pages[1].locator('.xterm-helper-textarea').pressSequentially("printf 'STILL_%s\\n' ALIVE",{delay:1});await pages[1].keyboard.press('Enter');
  await pages[1].waitForFunction(()=>document.querySelector('.xterm-rows')?.textContent.includes('STILL_ALIVE'));
  await pages[1].click('#terminal-close');
  if(errors.length)throw Error(errors.join('\n'));
  console.log('PASS: 兩個獨立頁面 → 正式 API → Remote → P2P → PTY，真實 Prompt／輸入輸出；關閉一個不影響另一個。');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
