#!/usr/bin/env python3
"""從固定的官方來源重新連結 CameraX JNI，保留完整 Maven 傳遞依賴與嚴格 16 KB 檢查。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import tempfile
import urllib.request
import xml.etree.ElementTree as ET
import zipfile

import android_core as core

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'android/third_party/camera-core'
VERSION = '1.6.2-arm64-16k1'
REPO = ROOT / 'android/libs/maven'
DEST = REPO / 'com/yourdesk/thirdparty/camera-core' / VERSION
AAR = DEST / ('camera-core-' + VERSION + '.aar')
POM = AAR.with_suffix('.pom')
RECORD = AAR.with_suffix('.json')


def fingerprint():
    digest = hashlib.sha256(Path(__file__).read_bytes())
    for path in sorted(SOURCE.iterdir()):
        if path.is_file() and not path.name.startswith('.') and not path.name.endswith('.bak'):
            digest.update(path.name.encode() + b'\0' + path.read_bytes())
    return digest.hexdigest()


def verify():
    record = json.loads(RECORD.read_text())
    if record['sourceSha256'] != fingerprint() or record['aarSha256'] != core.digest(AAR) or record['pomSha256'] != core.digest(POM):
        raise ValueError('CameraX 來源／產物已變更，請重建 android_native.py build')
    check_archive(AAR)


def check_archive(archive):
    core.verify_private_paths(archive)
    with zipfile.ZipFile(archive) as z:
        libraries = [name for name in z.namelist() if name.endswith('.so')]
        if not libraries: raise ValueError('CameraX 缺少原生庫')
        for name in libraries:
            if not name.startswith('jni/arm64-v8a/'): raise ValueError('CameraX ABI 不符')
            error = core.elf_error(z.read(name), 183)
            if error: raise ValueError(name + ': ' + error)


def download(info, target):
    data = urllib.request.urlopen(info['url'], timeout=60).read()
    if hashlib.sha256(data).hexdigest() != info['sha256']:
        raise ValueError('官方依賴 SHA-256 不符，停止建置')
    target.write_bytes(data)


def build():
    try:
        verify()
        print('CameraX JNI 來源／16 KB 驗證通過，沿用既有產物。')
        return
    except (OSError, ValueError, KeyError): pass
    sdk = Path(os.environ.get('ANDROID_HOME') or os.environ.get('ANDROID_SDK_ROOT', ''))
    ndk = os.environ.get('ANDROID_NDK_HOME')
    if not ndk:
        candidates = list((sdk / 'ndk').glob('*'))
        candidates = [p for p in candidates if (p / 'source.properties').exists()]
        if not candidates: raise ValueError('需要 Android NDK；請設定 ANDROID_NDK_HOME')
        ndk = str(max(candidates, key=lambda p: tuple(int(n) for n in p.name.split('.'))))
    host = {'Darwin': 'darwin-x86_64', 'Linux': 'linux-x86_64', 'Windows': 'windows-x86_64'}[platform.system()]
    compiler = Path(ndk) / 'toolchains/llvm/prebuilt' / host / 'bin' / ('clang++.exe' if os.name == 'nt' else 'clang++')
    provenance = json.loads((SOURCE / 'provenance.json').read_text())
    for name, info in provenance['files'].items():
        if core.digest(SOURCE / name) != info['sha256']: raise ValueError('CameraX 官方來源已變更：' + name)
    DEST.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.native-', dir=DEST) as temp:
        temp = Path(temp)
        upstream, pom = temp / 'upstream.aar', temp / 'upstream.pom'
        download(provenance['aar'], upstream)
        download(provenance['pom'], pom)
        library = temp / 'libsurface_util_jni.so'
        subprocess.run([str(compiler), '--target=aarch64-linux-android26', '-shared', '-fPIC', '-O2', '-DNDEBUG',
                        '-fvisibility=hidden', '-fstack-protector-strong', '-Wl,--build-id=sha1',
                        '-Wl,-soname,libsurface_util_jni.so', '-Wl,--version-script=' + str(SOURCE / 'jni.lds'),
                        '-Wl,-z,max-page-size=16384,-z,common-page-size=16384,-z,relro,-z,now',
                        '-Wl,--no-undefined', '-Wl,--undefined-version', '-Wl,-s', '-ffile-prefix-map=' + str(SOURCE) + '=camera-core',
                        '-static-libstdc++', str(SOURCE / 'surface_util_jni.cc'), '-landroid', '-o', str(library)], check=True)
        candidate = temp / AAR.name
        with zipfile.ZipFile(upstream) as source, zipfile.ZipFile(candidate, 'w', zipfile.ZIP_DEFLATED) as target:
            for item in source.infolist():
                if item.filename.startswith('jni/') and not item.filename.startswith('jni/arm64-v8a/'): continue
                data = library.read_bytes() if item.filename == 'jni/arm64-v8a/libsurface_util_jni.so' else source.read(item)
                target.writestr(item, data)
        check_archive(candidate)
        # 保留 upstream POM 的完整依賴；僅更換此衍生產物的座標。
        ns = 'http://maven.apache.org/POM/4.0.0'
        ET.register_namespace('', ns)
        tree = ET.fromstring(pom.read_bytes())
        tree.find('{' + ns + '}groupId').text = 'com.yourdesk.thirdparty'
        tree.find('{' + ns + '}version').text = VERSION
        patched_pom = ET.tostring(tree, encoding='utf-8', xml_declaration=True)
        # Maven metadata 隨 manifest 校驗，任何中斷／過期都會讓 Gradle preBuild 拒絕。
        previous_pom = POM.read_bytes() if POM.exists() else None
        candidate_pom = temp / POM.name
        candidate_pom.write_bytes(patched_pom)
        staged = temp / RECORD.name
        staged.write_text(json.dumps({'sourceSha256': fingerprint(), 'aarSha256': core.digest(candidate),
                                      'pomSha256': core.digest(candidate_pom), 'upstream': provenance}, indent=2) + '\n')
        os.replace(candidate_pom, POM)
        try:
            core.publish(candidate, staged, AAR, RECORD, verify)
        except BaseException:
            if previous_pom is None:
                POM.unlink(missing_ok=True)
            else:
                candidate_pom.write_bytes(previous_pom)
                os.replace(candidate_pom, POM)
            raise
    print('CameraX JNI 已從固定官方來源重新連結；全部 arm64 原生庫通過 16 KB 驗證。')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['build', 'verify'])
    args = parser.parse_args()
    try:
        if args.action == 'build': build()
        else: verify(); print('CameraX 來源／Maven metadata／16 KB 驗證通過。')
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError, zipfile.BadZipFile) as error:
        parser.exit(1, str(error) + '\n')


if __name__ == '__main__': main()
