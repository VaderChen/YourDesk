// 由 Go 產品入口輸出完整資產，讓瀏覽器測到 App 真正載入的文件。
const fs=require('fs'),os=require('os'),path=require('path'),{execFileSync}=require('child_process');
module.exports=()=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'yourdesk-titlebar-'));
 try {
  const output=path.join(dir,'titlebar.html');
  execFileSync('go',['test','./cmd/remote','-run','^TestTitlebarHTMLSmokeExport$','-count=1'],{
   cwd:path.resolve(__dirname,'..'),env:{...process.env,YOURDESK_TITLEBAR_SMOKE_HTML:output},stdio:'pipe'
  });
  return fs.readFileSync(output,'utf8');
 } finally {fs.rmSync(dir,{recursive:true,force:true})}
};
