#!/usr/bin/env python3
"""建置 macOS App Intents Extension 與 Siri metadata；由正式封裝流程呼叫。"""
import json
import os
from pathlib import Path
import plistlib
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]

def run(args):
    subprocess.run([str(x) for x in args], check=True)

def build(app, version='1.0', identity='-'):
    app = Path(app).resolve()
    extension = app / 'Contents/Extensions/YourDeskSiri.appex'
    contents = extension / 'Contents'
    mac = contents / 'MacOS'
    resources = contents / 'Resources'
    mac.mkdir(parents=True, exist_ok=True)
    resources.mkdir(exist_ok=True)
    source = ROOT / 'platform/macos/Siri/YourDeskIntents.swift'
    sdk = subprocess.check_output(['xcrun', '--show-sdk-path'], text=True).strip()
    compiler = Path(subprocess.check_output(['xcrun', '--find', 'swiftc'], text=True).strip())
    toolchain = compiler.parents[2]
    sdk_version = subprocess.check_output(['xcrun', '--show-sdk-version'], text=True).strip()
    if int(sdk_version.split('.')[0]) < 27:
        raise RuntimeError('Siri AI schema 需要 Xcode 27 SDK；不可默默產生缺少新版能力的發行包')
    with tempfile.TemporaryDirectory(prefix='yourdesk-siri-') as folder:
        tmp = Path(folder)
        protocols = json.loads((toolchain / 'usr/share/swift/SwiftConstantValues/AppIntents.json').read_text())
        (tmp/'protocols.json').write_text(json.dumps(protocols.get('constValueProtocols', []) if isinstance(protocols,dict) else protocols))
        const = tmp/'YourDeskSiri.swiftconstvalues'
        run([compiler, '-parse-as-library', '-module-name', 'YourDeskSiri', '-target', 'arm64-apple-macos13.0', '-sdk', sdk,
             '-Xfrontend', '-emit-const-values-path', '-Xfrontend', const,
             '-Xfrontend', '-const-gather-protocols-file', '-Xfrontend', tmp/'protocols.json', source, '-o', mac/'YourDeskSiri'])
        (tmp/'sources').write_text(str(source)+'\n')
        (tmp/'consts').write_text(str(const)+'\n')
        xcode_version = subprocess.check_output(['xcodebuild','-version'],text=True).split('Build version ')[-1].strip()
        run(['xcrun','appintentsmetadataprocessor','--output',resources,'--toolchain-dir',toolchain,'--module-name','YourDeskSiri',
             '--sdk-root',sdk,'--xcode-version',xcode_version,'--platform-family','macOS','--deployment-target','13.0',
             '--target-triple','arm64-apple-macos13.0','--source-file-list',tmp/'sources','--swift-const-vals-list',tmp/'consts'])
        if not (resources/'Metadata.appintents').is_dir():
            raise RuntimeError('App Intents metadata 未產生')
        metadata = json.loads((resources/'Metadata.appintents/extract.actionsdata').read_text())
        required = {'OpenYourDesk', 'ConnectDevice', 'DisconnectDevice', 'ConnectionStatus', 'OpenSavedDevice'}
        if not required.issubset(metadata.get('actions', {})) or len(metadata.get('autoShortcuts', [])) != 4:
            raise RuntimeError('Siri 動作或 App Shortcuts metadata 不完整')
        if not metadata['actions']['OpenSavedDevice'].get('assistantDefinedSchemas'):
            raise RuntimeError('新版 Siri AI open schema 未輸出')
        info = dict(CFBundleIdentifier='com.yourdesk.desktop.siri', CFBundleExecutable='YourDeskSiri',CFBundleName='YourDesk',
                    CFBundleDisplayName='YourDesk',CFBundlePackageType='XPC!',CFBundleVersion=version,CFBundleShortVersionString=version,
                    LSMinimumSystemVersion='13.0',EXAppExtensionAttributes={'EXExtensionPointIdentifier':'com.apple.appintents-extension'})
        (contents/'Info.plist').write_bytes(plistlib.dumps(info))
        entitlements = {'com.apple.security.app-sandbox':True,'com.apple.security.network.client':True,
                        'com.apple.security.temporary-exception.files.home-relative-path.read-only':['Library/Application Support/YourDesk/siri-bridge.json']}
        (tmp/'entitlements.plist').write_bytes(plistlib.dumps(entitlements))
        signing = ['--timestamp','--options','runtime'] if identity != '-' else []
        run(['codesign','--force','--sign',identity,*signing,'--entitlements',tmp/'entitlements.plist',extension])
        run(['codesign','--verify','--strict',extension])
    return extension

if __name__ == '__main__':
    import argparse
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app',type=Path)
    parser.add_argument('--version',default='1.0')
    args=parser.parse_args()
    build(args.app,args.version,os.environ.get('YOURDESK_CODESIGN_IDENTITY','-'))
