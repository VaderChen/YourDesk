#!/usr/bin/env python3
"""隔離執行真正的使用者更新腳本及 .NET 管線；SCM／安裝器使用替身，不提權。"""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import uuid

parser = argparse.ArgumentParser()
parser.add_argument('--shell', default=shutil.which('pwsh') or shutil.which('powershell.exe'))
args = parser.parse_args()
if not args.shell:
    parser.error('需要 pwsh 或 Windows PowerShell')
root = Path(__file__).resolve().parents[1]
rollback = (root/'internal/clientui/update_payload.go').read_text().split('const windowsRollbackScript = `', 1)[1].rsplit('`', 1)[0]
script = rollback + (root/'internal/clientui/update_service_script.go').read_text().split('const windowsServiceUpdateScript = windowsRollbackScript + `', 1)[1].rsplit('`', 1)[0]
legacy = (root/'internal/clientui/update_install_windows.go').read_text().split('const windowsUpdateScript = `', 1)[1].rsplit('`', 1)[0].replace('` + windowsRollbackScript + `', rollback)
assert 'RunAs' not in script and 'Start-Service' not in script and 'Stop-Service' not in script

def ps(value):
    return "'" + str(value).replace("'", "''") + "'"

server_source = r'''
param($ticket,$record,$failure)
$ErrorActionPreference='Stop'
$phase='ready'
while ($true) {
 $pipe=New-Object IO.Pipes.NamedPipeServerStream(('YourDeskPrelogin-update-'+$ticket),[IO.Pipes.PipeDirection]::InOut,1,[IO.Pipes.PipeTransmissionMode]::Byte,[IO.Pipes.PipeOptions]::Asynchronous)
 try {
  $pipe.WaitForConnection()
  $encoding=New-Object Text.UTF8Encoding($false)
  $reader=New-Object IO.StreamReader($pipe,$encoding,$false,1024,$true)
  $writer=New-Object IO.StreamWriter($pipe,$encoding,1024,$true)
  $request=$reader.ReadLine() | ConvertFrom-Json
  Add-Content -LiteralPath $record -Value $request.operation
  $errorText=''
  switch ($request.operation) {
   'begin' { $phase='stopped' }
   'commit' { $phase='applied'; if ($failure -eq 'service') { $phase='rolled-back'; $errorText='fixture service health failure' } }
   'rollback' { $phase='rolled-back' }
   'finish' { $phase='complete' }
  }
  $writer.WriteLine((@{phase=$phase;error=$errorText} | ConvertTo-Json -Compress));$writer.Flush()
  $reader.Dispose();$writer.Dispose()
 } finally { $pipe.Dispose() }
}
'''

with tempfile.TemporaryDirectory(prefix='ydp-', dir=None if os.name == 'nt' else '/tmp') as temporary:
    base = Path(temporary)
    env = dict(os.environ, TMPDIR=str(base))  # Unix domain socket 路徑上限；不改產品管線名稱。
    # 使用 PowerShell 自己的 parser 檢查新版／舊版 migration 腳本。
    for name, source in [('service', script), ('legacy', legacy)]:
        candidate = base/(name+'.ps1'); candidate.write_text(source)
        check = "$tokens=$null;$errors=$null;[Management.Automation.Language.Parser]::ParseFile("+ps(candidate)+",[ref]$tokens,[ref]$errors)|Out-Null;if($errors.Count){$errors|Out-String|Write-Error;exit 1}"
        subprocess.run([args.shell,'-NoProfile','-NonInteractive','-Command',check],env=env,check=True,timeout=30)
    server_file=base/'server.ps1';server_file.write_text(server_source)
    for failure in ('', 'installer', 'service', 'launch'):
        folder=base/(failure or 'success');folder.mkdir()
        target=folder/'target';target.mkdir()
        for name in ('YourDesk.exe','yourdesk-client.exe','yourdesk-remote.exe','avcodec-62.dll'):
            (target/name).write_text('old')
        (target/'personal.txt').write_text('preserve')
        (folder/'setup.exe').write_text('fixture')
        ticket=uuid.uuid4().hex
        (folder/'config.json').write_text(json.dumps({'parent':os.getpid(),'target':str(target),'serviceTicket':ticket,'files':['YourDesk.exe','yourdesk-client.exe','yourdesk-remote.exe','avcodec-62.dll','avutil-60.dll']}))
        records,launches=folder/'operations',folder/'launches'
        shim=r'''
$fixtureFailure=FIXTURE_FAILURE
function Get-Process { param($Id,$ErrorAction); $p=New-Object PSObject; $p | Add-Member -MemberType ScriptMethod -Name WaitForExit -Value {param($timeout) return $true}; return $p }
function Start-Process {
 param($FilePath,$ArgumentList,$WorkingDirectory,[switch]$PassThru,[switch]$Wait,$ErrorAction,$Verb)
 if ($Verb) { throw 'Unexpected elevation in ordinary update' }
 if ([IO.Path]::GetFileName($FilePath) -eq 'setup.exe') {
  [IO.File]::WriteAllText((Join-Path $config.target 'YourDesk.exe'),'new')
  [IO.File]::WriteAllText((Join-Path $config.target 'avcodec-62.dll'),'new')
  [IO.File]::WriteAllText((Join-Path $config.target 'avutil-60.dll'),'new')
  return [pscustomobject]@{ExitCode=$(if($fixtureFailure -eq 'installer'){1}else{0})}
 }
 $v=[IO.File]::ReadAllText($FilePath);Add-Content -LiteralPath FIXTURE_LAUNCHES -Value $v
 if ($fixtureFailure -eq 'launch' -and $v -eq 'new') { throw 'fixture launch failed' }
}
Microsoft.PowerShell.Utility\Add-Type -TypeDefinition 'namespace System.Windows { public class MessageBox { public static void Show(string a,string b) {} } }'
function Add-Type { param($AssemblyName) }
'''.replace('FIXTURE_FAILURE',ps(failure)).replace('FIXTURE_LAUNCHES',ps(launches))
        # 記錄放在暫存更新目錄外，成功刪除 staging 後仍可檢查。
        stage=folder/'stage';stage.mkdir()
        for name in ('config.json','setup.exe'): (folder/name).rename(stage/name)
        (stage/'install.ps1').write_text(shim+script)
        server=subprocess.Popen([args.shell,'-NoProfile','-NonInteractive','-File',str(server_file),ticket,str(records),failure],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
        try:
            result=subprocess.run([args.shell,'-NoProfile','-NonInteractive','-File',str(stage/'install.ps1')],env=env,capture_output=True,text=True,timeout=35)
        finally:
            server.terminate();server_out,server_err=server.communicate(timeout=10)
        log=(stage/'update.log').read_text() if (stage/'update.log').exists() else ''
        assert (result.returncode != 0) == bool(failure), (failure,result.returncode,result.stdout,result.stderr,log,server_out,server_err)
        assert (target/'YourDesk.exe').read_text() == ('old' if failure else 'new')
        assert (target/'avcodec-62.dll').read_text() == ('old' if failure else 'new')
        assert (target/'avutil-60.dll').exists() == (not failure)
        assert (target/'personal.txt').read_text() == 'preserve'
        calls=records.read_text().splitlines()
        assert calls[:2] == ['status','begin'], calls
        assert calls[-1] == ('rollback' if failure else 'finish'), calls
        assert launches.read_text().splitlines() == (['new','old'] if failure == 'launch' else ['old'] if failure else ['new'])
        if failure: assert (stage/'update.log').is_file()
        else: assert not stage.exists()
print('PASS: PowerShell 語法、真實管線交接、更新成功、安裝失敗、服務失敗、啟動失敗及原帳號重開流程（未執行 Windows SCM）')
