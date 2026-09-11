#!/usr/bin/env python3
"""YourDesk 跨平台建置、macOS DMG／Windows Installer 封裝。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import time
import zipfile
from datetime import datetime
from zoneinfo import ZoneInfo

ROOT = Path(__file__).resolve().parent.parent
DIST = ROOT / 'dist'
DEFAULT_TARGETS = 'darwin/arm64,windows/amd64,windows/arm64'
SUPPORTED_TARGETS = {'darwin/arm64', 'windows/amd64', 'windows/arm64'}


def run(args, env=None, cwd=ROOT):
    subprocess.run([str(a) for a in args], cwd=cwd, env=env, check=True)


def capture(args):
    return subprocess.check_output(args, cwd=ROOT, text=True).strip()


def manifest(folder):
    lines = []
    for item in sorted(folder.rglob('*')):
        if item.is_file() and not item.is_symlink() and item.name != 'SHA256SUMS':
            lines.append(f'{hashlib.sha256(item.read_bytes()).hexdigest()}  {item.relative_to(folder)}\n')
    (folder / 'SHA256SUMS').write_text(''.join(lines))


def version_name(version):
    if not re.fullmatch(r'1\.\d{2}\.\d{4} build \d{4}', version):
        raise ValueError('版本格式必須為 1.YY.MMDD build HHmm')
    return version.replace(' build ', '-build-')


def windows_tool(arch, kind):
    prefix = 'x86_64' if arch == 'amd64' else 'aarch64'
    suffixes = {'CC': ('gcc', 'clang'), 'CXX': ('g++', 'clang++'), 'WINDRES': ('windres',)}[kind]
    names = [f'{prefix}-w64-mingw32-{suffix}' for suffix in suffixes]
    for name in names:
        found = shutil.which(name)
        if found:
            return found
    # 可攜式工具鏈僅存在本機；不將編譯器或個人路徑放入發行套件。
    root = Path(os.environ.get('YOURDESK_LLVM_MINGW', str(Path.home() / '.local/share/yourdesk/toolchains/llvm-mingw')))
    for name in names:
        candidate = root / 'bin' / name
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return str(candidate)
    raise ValueError(f'windows/{arch} 缺少 {kind}；請安裝 LLVM-MinGW，或設定對應的 YOURDESK_{kind}_WINDOWS_{arch.upper()}（windres 使用 YOURDESK_WINDRES_{arch.upper()}）')


def environment(target, gui):
    system, arch = target.split('/')
    env = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED='1' if gui else '0')
    if not gui:
        return env
    host = capture(['go', 'env', 'GOHOSTOS'])
    suffix = target.replace('/', '_').upper()
    for key in ('CC', 'CXX', 'CGO_CFLAGS', 'CGO_CXXFLAGS', 'CGO_LDFLAGS', 'PKG_CONFIG_PATH'):
        if f'YOURDESK_{key}_{suffix}' in os.environ:
            env[key] = os.environ[f'YOURDESK_{key}_{suffix}']
    if system == 'darwin' and host != 'darwin':
        raise ValueError('macOS 桌面版須在 macOS 搭配 Xcode Command Line Tools 建置')
    if system == 'windows' and host != 'windows':
        for key in ('CC', 'CXX'):
            if key not in env:
                env[key] = windows_tool(arch, key)
        for key in ('CC', 'CXX'):
            if not shutil.which(shlex.split(env[key])[0]):
                raise ValueError(f'{target} 缺少 {env[key]}；請設定 YOURDESK_{key}_{suffix}')
    return env


def compile_program(name, folder, target, version):
    system, _ = target.split('/')
    gui = name in ('client', 'remote')
    output = ('YourDesk' if name == 'desktop' else 'yourdesk-' + name) + ('.exe' if system == 'windows' else '')
    flags = f"-s -w -X 'yourdesk/internal/clientui.Version={version}'"
    if system == 'windows':
        flags += ' -H=windowsgui'
    resource = None
    try:
        if system == 'windows':
            arch = target.split('/')[1]
            windres = os.environ.get('YOURDESK_WINDRES_' + arch.upper()) or windows_tool(arch, 'WINDRES')
            if not windres:
                raise ValueError(f'{target} 缺少 windres，無法嵌入圖示')
            resource = ROOT / 'cmd' / name / f'icon_windows_{arch}.syso'
            if resource.exists():
                existing = resource
                resource = None
                raise ValueError(f'資源檔已存在，拒絕覆寫：{existing}')
            icon = Path(os.environ.get('YOURDESK_WINDOWS_ICON_PATH', str(ROOT / 'assets/branding/yourdesk.ico'))).resolve()
            with tempfile.TemporaryDirectory(prefix='yourdesk-resource-') as temporary:
                source = Path(temporary) / 'icon.rc'
                shutil.copy2(icon, Path(temporary) / 'icon.ico')
                source.write_text('1 ICON "icon.ico"\n')
                run([windres, '-i', source, '-O', 'coff', '-o', resource], cwd=Path(temporary))
        run(['go', 'build', '-buildvcs=false', '-trimpath', '-ldflags', flags, '-o', folder / output, './cmd/' + name], environment(target, gui))
        if system == 'darwin':
            run([ROOT / 'scripts/sign-local.sh', folder / output])
    finally:
        if resource is not None:
            resource.unlink(missing_ok=True)


def signing_identity():
    identity = os.environ.get('YOURDESK_CODESIGN_IDENTITY')
    if not identity:
        identities = capture(['security', 'find-identity', '-v', '-p', 'codesigning'])
        match = re.search(r'"(Developer ID Application:[^"\n]+)"', identities)
        identity = match.group(1) if match else ''
        if identity:
            os.environ['YOURDESK_CODESIGN_IDENTITY'] = identity
    if not identity or identity == '-':
        raise ValueError('需要固定 Developer ID 簽章，不使用 ad-hoc')
    return identity


def notary_profile():
    profile = os.environ.get('YOURDESK_NOTARY_PROFILE')
    if not profile:
        raise ValueError('請設定 YOURDESK_NOTARY_PROFILE，指定本機 Keychain 的公證設定')
    return profile


def notarize(target):
    print(f'提交 Apple 公證：{target.name}', flush=True)
    result = capture(['xcrun', 'notarytool', 'submit', str(target), '--keychain-profile', notary_profile(), '--wait', '--output-format', 'json'])
    info = json.loads(result)
    if info.get('status') != 'Accepted':
        raise ValueError(f"Apple 公證未通過：{info.get('status')}；提交 ID：{info.get('id')}")
    print(f'Apple 公證通過：{target.name}', flush=True)


def staple(target):
    for attempt in range(10):
        result = subprocess.run(['xcrun', 'stapler', 'staple', str(target)], capture_output=True, text=True)
        if result.returncode == 0:
            run(['xcrun', 'stapler', 'validate', target])
            return
        if attempt < 9:
            time.sleep(6)
    raise ValueError(f'公證票根附加失敗：{target}\n{result.stderr}')


def notarize_app(app):
    valid = subprocess.run(['xcrun', 'stapler', 'validate', str(app)], capture_output=True).returncode == 0
    if not valid:
        with tempfile.TemporaryDirectory(prefix='yourdesk-notary-') as temporary:
            archive = Path(temporary) / 'YourDesk.zip'
            run(['ditto', '-c', '-k', '--sequesterRsrc', '--keepParent', app, archive])
            notarize(archive)
        staple(app)
    run(['spctl', '--assess', '--type', 'execute', '--verbose=2', app])


def mac_bundle(folder, version):
    app = folder / 'YourDesk.app'
    mac = app / 'Contents/MacOS'
    resources = app / 'Contents/Resources'
    mac.mkdir(parents=True)
    resources.mkdir()
    for name in ('YourDesk', 'yourdesk-client', 'yourdesk-remote'):
        shutil.copy2(folder / name, mac / name)
    numeric = version.split(' build ')[0]
    info = dict(CFBundleIdentifier='com.yourdesk.desktop', CFBundleName='YourDesk',
                CFBundleDisplayName='YourDesk', CFBundleExecutable='YourDesk', CFBundlePackageType='APPL',
                CFBundleShortVersionString=numeric, CFBundleVersion=numeric + '.' + version[-4:],
                CFBundleGetInfoString=version, NSHighResolutionCapable=True, LSMinimumSystemVersion='12.0',
                NSScreenCaptureUsageDescription='YourDesk 需要擷取螢幕以提供遠端桌面。')
    icon = os.environ.get('YOURDESK_MAC_ICON_PATH')
    if icon:
        shutil.copy2(icon, resources / 'AppIcon.icns')
        info['CFBundleIconFile'] = 'AppIcon.icns'
    else:
        # 格式轉換沿用現有圖像；不重新生成品牌圖案。
        with tempfile.TemporaryDirectory(prefix='yourdesk-icon-') as temporary:
            iconset = Path(temporary) / 'AppIcon.iconset'
            iconset.mkdir()
            source = ROOT / 'internal/clientui/web/app-icon.png'
            for size in (16, 32, 128, 256, 512):
                for scale in (1, 2):
                    name = f'icon_{size}x{size}' + ('@2x' if scale == 2 else '') + '.png'
                    run(['sips', '-z', size * scale, size * scale, source, '--out', iconset / name])
            run(['iconutil', '-c', 'icns', iconset, '-o', resources / 'AppIcon.icns'])
        info['CFBundleIconFile'] = 'AppIcon.icns'
    copy_model_licenses(resources)
    (app / 'Contents/Info.plist').write_bytes(plistlib.dumps(info))
    identity = signing_identity()
    signing = ['--timestamp', '--options', 'runtime'] if identity != '-' else []
    for executable in mac.iterdir():
        run(['codesign', '--force', '--sign', identity, *signing, executable])
    run(['codesign', '--force', '--sign', identity, *signing, app])
    run(['codesign', '--verify', '--deep', '--strict', app])
    notarize_app(app)


def copy_model_licenses(folder):
    licenses = folder / 'ThirdPartyLicenses'
    licenses.mkdir(exist_ok=True)
    shutil.copy2(ROOT / 'internal/superres/MODEL_LICENSE.txt', licenses / 'CoreML-SR-LICENSE.txt')
    shutil.copy2(ROOT / 'docs/FSR-LICENSE.txt', licenses / 'FSR-LICENSE.txt')
    for name in ('MODEL_LICENSE.txt', 'MODEL_SOURCE.md', 'MODEL_SHA256SUMS'):
        source = ROOT / 'internal/frameinterp' / name
        shutil.copy2(source, licenses / ('RIFE-' + name))


def write_instructions(folder, system, version):
    if system == 'darwin':
        return
    copy_model_licenses(folder)
    instructions = ROOT / 'docs' / 'README-Windows.txt'
    content = f'YourDesk {version}\n\n' + instructions.read_text(encoding='utf-8')
    for name in ('README.txt', '使用說明.txt'):
        (folder / name).write_text(content, encoding='utf-8-sig')


def reset_dist():
    # 固定清理專案的 dist；目錄內的符號連結只移除連結本身。
    expected = ROOT / 'dist'
    if DIST != expected or DIST.is_symlink() or DIST.resolve() != expected.absolute():
        raise ValueError(f'拒絕清理非預期的 dist 路徑：{DIST}')
    if DIST.exists() and not DIST.is_dir():
        raise ValueError(f'dist 不是目錄：{DIST}')
    DIST.mkdir(exist_ok=True)
    for item in DIST.iterdir():
        if item.is_symlink() or not item.is_dir():
            item.unlink()
        else:
            shutil.rmtree(item)
    print(f'已清空：{DIST}', flush=True)


def build(version, targets):
    version_name(version)  # 在清理既有產物前驗證版本格式。
    release = DIST
    selected = list(dict.fromkeys(targets.split(',')))
    if not selected or any(t not in SUPPORTED_TARGETS for t in selected):
        raise ValueError('不支援的建置目標；macOS 僅支援 arm64，Windows 支援 amd64、arm64；Linux 暫不提供桌面套件')
    # 預先確認原生 UI 工具鏈，禁止用 CGO=0 產生缺少介面的桌面版。
    for target in selected:
        environment(target, True)
    if 'darwin/arm64' in selected:
        signing_identity()
        capture(['xcrun', 'notarytool', 'history', '--keychain-profile', notary_profile(), '--output-format', 'json'])
    reset_dist()
    with tempfile.TemporaryDirectory(prefix='.yourdesk-build-', dir=DIST) as temporary:
        stage = Path(temporary)
        for target in selected:
            print(f'建置 {target}：{version}', flush=True)
            system, arch = target.split('/')
            folder = stage / (('macos' if system == 'darwin' else system) + '-' + arch)
            folder.mkdir()
            if system == 'darwin':
                # 中間執行檔只留在暫存目錄，Apple 發行目錄僅放 App／DMG。
                with tempfile.TemporaryDirectory(prefix='yourdesk-macos-bin-') as temporary:
                    binaries = Path(temporary)
                    for name in ('client', 'remote', 'desktop'):
                        compile_program(name, binaries, target, version)
                    mac_bundle(binaries, version)
                    shutil.move(str(binaries / 'YourDesk.app'), folder / 'YourDesk.app')
            else:
                programs = ('client', 'remote', 'desktop')
                for name in programs:
                    compile_program(name, folder, target, version)
                write_instructions(folder, system, version)
                manifest(folder)
        (stage / 'release.json').write_text(json.dumps(dict(version=version, targets=selected), indent=2))
        manifest(stage)
        # 平台目錄直接位於 dist；中繼資料最後發布，供 --no-build 判斷建置完成。
        for item in stage.iterdir():
            if item.name != "release.json":
                os.rename(item, release / item.name)
        os.rename(stage / "release.json", release / "release.json")
    native = capture(['go', 'env', 'GOHOSTOS']) + '/' + capture(['go', 'env', 'GOHOSTARCH'])
    if native in selected:
        folder = release / native.replace('darwin/', 'macos-').replace('/', '-')
        (ROOT / 'bin').mkdir(exist_ok=True)
        if native.startswith('darwin/'):
            folder = folder / 'YourDesk.app/Contents/MacOS'
        for binary in folder.iterdir():
            if binary.is_file() and (binary.name.startswith('yourdesk-') or binary.name in ('YourDesk', 'YourDesk.exe')):
                # 不覆寫執行中程序映射的 inode，避免 macOS 簽章頁面失效。
                # 暫存檔與目的檔位於同一檔案系統，以原子替換發布完整檔案。
                with tempfile.TemporaryDirectory(prefix='.release-', dir=ROOT / 'bin') as temporary:
                    replacement = Path(temporary) / binary.name
                    shutil.copy2(binary, replacement)
                    os.replace(replacement, ROOT / 'bin' / binary.name)
    print(f'建置完成：{release}', flush=True)
    return release


def clean_apple_output(folder):
    # 同時相容先前版本的 --no-build，移除舊流程留下的附屬檔案。
    for name in ('YourDesk', 'yourdesk-client', 'yourdesk-remote', 'yourdesk-server', 'README.txt', '使用說明.txt', 'SHA256SUMS'):
        item = folder / name
        if item.is_file() or item.is_symlink():
            item.unlink()


def windows_installer(folder, stem, version, arch):
    compiler = os.environ.get('YOURDESK_MAKENSIS') or shutil.which('makensis')
    if not compiler:
        raise ValueError('Windows 封裝需要 NSIS：macOS 執行 brew install nsis；Windows 安裝 NSIS 並將 makensis 加入 PATH，或設定 YOURDESK_MAKENSIS')
    parts = re.fullmatch(r'1\.(\d{2})\.(\d{4}) build (\d{4})', version)
    numeric = '1.' + '.'.join(str(int(part)) for part in parts.groups())
    output = folder / (stem + '-setup.exe')
    with tempfile.TemporaryDirectory(prefix='.installer-', dir=folder) as temporary:
        staged = Path(temporary) / output.name
        run([compiler, '-V2', f'-DPAYLOAD_DIR={folder.resolve()}',
             f'-DOUTPUT_FILE={staged.resolve()}', f'-DAPP_VERSION={version}',
             f'-DNUMERIC_VERSION={numeric}', f'-DAPP_ARCH={arch}',
             ROOT / 'scripts/windows-installer.nsi'])
        os.replace(staged, output)
    old_zip = folder / (stem + '.zip')
    if old_zip.exists():
        old_zip.unlink()


def pack(release, targets=None):
    metadata = json.loads((release / 'release.json').read_text())
    version = metadata['version']
    selected = targets if targets is not None else metadata['targets']
    if not selected or any(t not in SUPPORTED_TARGETS for t in selected):
        raise ValueError('封裝目標不支援；macOS 僅支援 arm64，請重新建置')
    for target in selected:
        system, arch = target.split('/')
        folder = release / (('macos' if system == 'darwin' else system) + '-' + arch)
        write_instructions(folder, system, version)
        stem = f'YourDesk-{version_name(version)}-{folder.name}'
        if system == 'darwin':
            with tempfile.TemporaryDirectory(prefix='yourdesk-dmg-') as temporary:
                stage = Path(temporary)
                app = stage / 'YourDesk.app'
                shutil.copytree(folder / 'YourDesk.app', app)
                identity = signing_identity()
                notarize_app(app)
                (stage / 'Applications').symlink_to('/Applications')
                output = folder / (stem + '.dmg')
                run(['hdiutil', 'create', '-volname', 'YourDesk', '-srcfolder', stage, '-ov', '-format', 'UDZO', output])
                run(['codesign', '--force', '--timestamp', '--sign', identity, output])
                run(['codesign', '--verify', '--strict', output])
                notarize(output)
                staple(output)
                run(['spctl', '--assess', '--type', 'open', '--context', 'context:primary-signature', '--verbose=2', output])
        elif system == 'windows':
            windows_installer(folder, stem, version, arch)
        if system == 'darwin':
            clean_apple_output(folder)
        else:
            manifest(folder)
    manifest(release)
    print(f'封裝完成：{release}')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=('build', 'pack'))
    parser.add_argument('--no-build', action='store_true', help='使用 dist 中已完成的建置產物打包')
    args = parser.parse_args()
    if DIST.is_symlink():
        raise ValueError('dist 不可為符號連結')
    version = os.environ.get('YOURDESK_VERSION')
    if args.mode == 'pack' and args.no_build:
        release = DIST
        metadata_path = release / 'release.json'
        if not metadata_path.is_file() or metadata_path.is_symlink():
            raise ValueError('找不到 dist/release.json，請先執行 buildMac.command 或 buildWin.command；舊版版本子目錄需重新建置')
        metadata = json.loads(metadata_path.read_text())
        if version:
            version_name(version)
            if metadata.get('version') != version:
                raise ValueError('指定版本與 dist 現有建置版本不同，請重新建置')
    else:
        version = version or datetime.now(ZoneInfo('Asia/Taipei')).strftime('1.%y.%m%d build %H%M')
        release = build(version, os.environ.get('YOURDESK_BUILD_TARGETS', DEFAULT_TARGETS))
    if args.mode == 'pack':
        pack(release)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f'錯誤：{error}', file=sys.stderr)
        sys.exit(1)
