#!/usr/bin/env python3
"""建置並驗證 Android APK，封裝為附四語說明的發行 ZIP；不會上傳或安裝。"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import zipfile

import android_core as core
import android_native as native

ROOT = Path(__file__).resolve().parents[1]
ANDROID = ROOT / 'android'


def version():
    values = dict(line.split('=', 1) for line in (ANDROID / 'version.properties').read_text().splitlines()
                  if line and not line.startswith('#') and '=' in line)
    name, code = values['versionName'], int(values['versionCode'])
    if code <= 0 or not re.fullmatch(r'\d+\.\d{2}\.\d{4} build \d{4}', name):
        raise ValueError('Android 版本格式無效')
    return name, code


def sdk_tools():
    location = os.environ.get('ANDROID_HOME') or os.environ.get('ANDROID_SDK_ROOT')
    if not location:
        local = ANDROID / 'local.properties'
        if local.exists():
            location = next((line[8:] for line in local.read_text().splitlines() if line.startswith('sdk.dir=')), None)
    if not location:
        raise ValueError('請設定 ANDROID_HOME 指向 Android SDK')
    sdk = Path(location).expanduser().resolve()
    versions = [p for p in (sdk / 'build-tools').iterdir() if re.fullmatch(r'\d+\.\d+\.\d+', p.name)]
    if not versions:
        raise ValueError('需要 Android SDK Build-Tools 35 以上')
    selected = max(versions, key=lambda p: tuple(map(int, p.name.split('.'))))
    if int(selected.name.split('.')[0]) < 35:
        raise ValueError('需要 Android SDK Build-Tools 35 以上')
    return sdk, selected


def run(command, capture=False, **kwargs):
    return subprocess.run([str(value) for value in command], cwd=ROOT, check=True,
                          text=True, stdout=subprocess.PIPE if capture else None, **kwargs)


def verify_apk(apk, build_tools):
    core.verify_native(apk, apk=True)
    core.verify_private_paths(apk)
    expected_name, expected_code = version()
    suffix = '.exe' if os.name == 'nt' else ''
    badging = run([build_tools / ('aapt2' + suffix), 'dump', 'badging', apk], capture=True).stdout
    package = re.search(r"^package: name='([^']+)' versionCode='([^']+)' versionName='([^']+)'", badging, re.M)
    if not package or package.groups() != ('com.yourdesk.android', str(expected_code), expected_name):
        raise ValueError('APK package／版本與 version.properties 不符')
    if 'application-debuggable' in badging:
        raise ValueError('不可交付可偵錯的 APK')
    with zipfile.ZipFile(apk) as archive:
        names = archive.namelist()
        if any('.bak' in Path(name).name or name.endswith(('.jks', '.keystore')) for name in names):
            raise ValueError('APK 含備份或簽章私鑰檔案')
        abis = {name.split('/')[1] for name in names if name.startswith('lib/') and name.endswith('.so')}
        if abis != {'arm64-v8a'}:
            raise ValueError('APK ABI 不符合目前的 arm64-v8a 發行範圍')
    run([build_tools / ('zipalign' + suffix), '-c', '-P', '16', '4', apk], capture=True)
    signature = run([build_tools / ('apksigner.bat' if os.name == 'nt' else 'apksigner'), 'verify',
                     '--verbose', '--print-certs', apk], capture=True).stdout
    if not all(re.search(r'Verified using v' + v + r' scheme .*: true', signature) for v in ('2', '3')):
        raise ValueError('APK 必須通過 v2 與 v3 簽章驗證')
    if 'Android Debug' in signature:
        raise ValueError('不可使用 Android debug 簽章發布')
    fingerprint = re.search(r'Signer #1 certificate SHA-256 digest: ([a-fA-F0-9]+)', signature)
    if not fingerprint:
        raise ValueError('無法確認 APK 簽章憑證')
    return fingerprint.group(1).lower()


def verify_archive(archive, manifest):
    record = json.loads(manifest.read_text(encoding='utf-8'))
    package = record['archive']
    if (package['fileName'] != archive.name or package['sha256'] != core.digest(archive)
            or package['size'] != archive.stat().st_size):
        raise ValueError('發行 ZIP 與封裝紀錄不符')
    with zipfile.ZipFile(archive) as zipped:
        entries = package['entries']
        if sorted(zipped.namelist()) != sorted([*entries, 'SHA256SUMS']):
            raise ValueError('發行 ZIP 含有缺漏或非預期檔案')
        for name, expected in entries.items():
            if hashlib.sha256(zipped.read(name)).hexdigest() != expected:
                raise ValueError('發行 ZIP 內容校驗失敗：' + name)
        sums = ''.join(f'{entries[name]}  {name}\n' for name in sorted(entries))
        if zipped.read('SHA256SUMS') != sums.encode('utf-8'):
            raise ValueError('發行 ZIP 內部 SHA256SUMS 不符')
        if entries.get(record['apkFile']) != record['sha256']:
            raise ValueError('發行 ZIP 內的 APK 與建置紀錄不符')


def pack(apk, build_tools, record_path=None):
    """先驗證既有簽章 APK，再封裝；檔案白名單避免把金鑰或備份打包。"""
    signer = verify_apk(apk, build_tools)
    name, code = version()
    expected_name = 'YourDesk-' + name.replace(' ', '-') + '-android-arm64.apk'
    if apk.name != expected_name:
        raise ValueError('APK 檔名與正式版本不符')
    record_bytes = (record_path or apk.with_suffix('.json')).read_bytes()
    record = json.loads(record_bytes)
    expected = {'versionName': name, 'versionCode': code, 'applicationId': 'com.yourdesk.android',
                'abi': 'arm64-v8a', 'sha256': core.digest(apk), 'signerSha256': signer}
    if not isinstance(record, dict) or any(record.get(key) != value for key, value in expected.items()):
        raise ValueError('APK 與建置紀錄不符，不能封裝')
    files = {apk.name: apk.read_bytes(), 'BUILD.json': record_bytes,
             'README.md': (ANDROID / 'RELEASE-README.md').read_bytes()}
    for license_name in ('LICENSE.md', 'LICENSE.en.md', 'LICENSE.ja.md', 'LICENSE.ko.md'):
        files[license_name] = (ROOT / license_name).read_bytes()
    entries = {name: hashlib.sha256(data).hexdigest() for name, data in files.items()}
    if entries[apk.name] != record['sha256']:
        raise ValueError('讀取期間 APK 已變更，不能封裝')
    files['SHA256SUMS'] = ''.join(f'{entries[name]}  {name}\n' for name in sorted(entries)).encode('utf-8')
    target = apk.with_suffix('.zip')
    manifest = target.with_suffix('.zip.json')
    checksum = target.with_suffix('.zip.sha256')
    with tempfile.TemporaryDirectory(prefix='.package-', dir=apk.parent) as temp:
        temp = Path(temp)
        candidate = temp / target.name
        with zipfile.ZipFile(candidate, 'w') as zipped:
            for filename in sorted(files):
                # 固定時間及權限，使相同輸入產生相同 ZIP 雜湊。
                info = zipfile.ZipInfo(filename, date_time=(1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = 0o100644 << 16
                zipped.writestr(info, files[filename], compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
        packaged = {**record, 'apkFile': apk.name,
                    'archive': {'fileName': target.name, 'sha256': core.digest(candidate),
                                'size': candidate.stat().st_size, 'entries': entries}}
        staged = temp / manifest.name
        staged.write_text(json.dumps(packaged, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        verify_archive(candidate, staged)
        core.publish(candidate, staged, target, manifest, lambda: verify_archive(target, manifest))
        staged_checksum = temp / checksum.name
        staged_checksum.write_text(core.digest(target) + '  ' + target.name + '\n', encoding='utf-8')
        os.replace(staged_checksum, checksum)
    print('發行 ZIP：' + str(target))
    print('ZIP SHA-256：' + core.digest(target))
    print('上傳附件：' + ', '.join(path.name for path in (target, checksum, manifest)))
    return target


def build(args):
    sdk, build_tools = sdk_tools()
    keystore = Path(args.keystore).expanduser().resolve()
    password = Path(args.password_file).expanduser().resolve()
    key_password = Path(args.key_password_file or args.password_file).expanduser().resolve()
    if not all(p.is_file() for p in (keystore, password, key_password)):
        raise ValueError('缺少既有發行金鑰或密碼檔；請設定 --keystore 與 --password-file，不會另建簽章身分')
    os.environ['ANDROID_HOME'] = str(sdk)
    core.build()
    native.build()
    wrapper = ANDROID / ('gradlew.bat' if os.name == 'nt' else 'gradlew')
    run([wrapper, '-p', ANDROID, ':app:lintRelease', ':app:assembleRelease', '--console=plain'])
    name, code = version()
    output = ROOT / 'dist/android'
    output.mkdir(parents=True, exist_ok=True)
    target = output / ('YourDesk-' + name.replace(' ', '-') + '-android-arm64.apk')
    manifest = target.with_suffix('.json')
    with tempfile.TemporaryDirectory(prefix='.release-', dir=output) as temp:
        temp = Path(temp)
        candidate = temp / target.name
        unsigned = ANDROID / 'app/build/outputs/apk/release/app-release-unsigned.apk'
        run([build_tools / ('zipalign.exe' if os.name == 'nt' else 'zipalign'), '-P', '16', '-f', '4', unsigned, candidate])
        # 密碼由 apksigner 自行讀檔，從不放在命令列參數、日誌或 Git。
        # 同一檔案重複傳入會被 apksigner 視為逐行讀取；同密碼時使用內建沿用規則。
        key_pass_args = [] if key_password == password else ['--key-pass', 'file:' + str(key_password)]
        run([build_tools / ('apksigner.bat' if os.name == 'nt' else 'apksigner'), 'sign',
             '--ks', keystore, '--ks-key-alias', args.alias, '--ks-pass', 'file:' + str(password),
             *key_pass_args, '--v1-signing-enabled', 'false',
             '--v2-signing-enabled', 'true', '--v3-signing-enabled', 'true', '--v4-signing-enabled', 'false', candidate])
        signer = verify_apk(candidate, build_tools)
        record = {'versionName': name, 'versionCode': code, 'applicationId': 'com.yourdesk.android',
                  'abi': 'arm64-v8a', 'sha256': core.digest(candidate), 'signerSha256': signer,
                  'core': json.loads(core.MANIFEST.read_text()),
                  'cameraNative': json.loads(native.RECORD.read_text()),
                  'builtAtUtc': datetime.now(timezone.utc).isoformat(),
                  'gitRevision': run(['git', 'rev-parse', 'HEAD'], capture=True).stdout.strip(),
                  'workingTreeModified': bool(run(['git', 'status', '--porcelain'], capture=True).stdout.strip())}
        staged = temp / manifest.name
        staged.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
        core.publish(candidate, staged, target, manifest, lambda: verify_apk(target, build_tools))
    target.with_suffix('.sha256').write_text(core.digest(target) + '  ' + target.name + '\n')
    print('可安裝的簽章 APK：' + str(target))
    print('SHA-256：' + core.digest(target))
    print('簽章憑證 SHA-256：' + signer)
    pack(target, build_tools)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['build', 'verify', 'pack', 'verify-zip'])
    parser.add_argument('--apk', type=Path)
    parser.add_argument('--zip', dest='archive', type=Path)
    parser.add_argument('--record', type=Path, help='pack 使用的既有 APK 建置紀錄；預設為 APK 旁的 .json')
    parser.add_argument('--keystore', default=os.environ.get('YOURDESK_ANDROID_KEYSTORE', str(ANDROID / 'cert/yourdesk-release.jks')))
    parser.add_argument('--password-file', default=os.environ.get('YOURDESK_ANDROID_PASSWORD_FILE', str(ANDROID / 'cert/keystore-password.txt')))
    parser.add_argument('--key-password-file', default=os.environ.get('YOURDESK_ANDROID_KEY_PASSWORD_FILE'))
    parser.add_argument('--alias', default=os.environ.get('YOURDESK_ANDROID_KEY_ALIAS', 'yourdesk-release'))
    args = parser.parse_args()
    try:
        if args.action == 'build': build(args)
        elif args.action == 'verify-zip':
            if not args.archive: parser.error('verify-zip 需要 --zip')
            archive = args.archive.resolve()
            verify_archive(archive, archive.with_suffix('.zip.json'))
            if archive.with_suffix('.zip.sha256').read_text(encoding='utf-8') != core.digest(archive) + '  ' + archive.name + '\n':
                raise ValueError('發行 ZIP 外部 SHA-256 清單不符')
            print('發行 ZIP、APK 位元組與內外校驗紀錄一致')
        else:
            if not args.apk: parser.error(args.action + ' 需要 --apk')
            _, selected = sdk_tools()
            if args.action == 'pack': pack(args.apk.resolve(), selected, args.record)
            else:
                signer = verify_apk(args.apk.resolve(), selected)
                print('APK 版本、非偵錯、16 KB ELF／ZIP、v2/v3 簽章通過；憑證 SHA-256：' + signer)
    except (ValueError, OSError, KeyError, subprocess.CalledProcessError, zipfile.BadZipFile) as error:
        parser.exit(1, str(error) + '\n')


if __name__ == '__main__': main()
