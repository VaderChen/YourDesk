"""固定版本、SHA-256 驗證的 TurboJPEG 靜態建置；產物僅存本機快取。"""
import hashlib
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tarfile
import urllib.request

VERSION = '3.2.0'
SHA256 = '6f30092cef9fb839779646608f4ee14ae3cbac989c47fa05e841b0841f09878e'
ROOT = Path(__file__).resolve().parent.parent
CACHE = ROOT / '.local-run' / 'turbojpeg'


def prepare(target, env):
    """回傳含靜態函式庫路徑的建置環境，以及原始碼授權目錄。"""
    if not shutil.which('cmake'):
        raise RuntimeError('TurboJPEG 建置需要 CMake')
    system, arch = target.split('/')
    if arch == 'amd64' and not shutil.which('nasm'):
        raise RuntimeError('x64 TurboJPEG SIMD 建置需要 NASM，請先安裝後重試')
    CACHE.mkdir(parents=True, exist_ok=True)
    archive = CACHE / f'libjpeg-turbo-{VERSION}.tar.gz'
    if not archive.exists() or hashlib.sha256(archive.read_bytes()).hexdigest() != SHA256:
        url = f'https://github.com/libjpeg-turbo/libjpeg-turbo/releases/download/{VERSION}/{archive.name}'
        with urllib.request.urlopen(url, timeout=60) as response:
            data = response.read(32 * 1024 * 1024 + 1)
        if len(data) > 32 * 1024 * 1024 or hashlib.sha256(data).hexdigest() != SHA256:
            raise RuntimeError('TurboJPEG 來源 SHA-256 不符')
        archive.write_bytes(data)
    source = CACHE / f'libjpeg-turbo-{VERSION}'
    if not source.exists():
        with tarfile.open(archive) as tar:
            members = tar.getmembers()
            for member in members:
                destination = (CACHE / member.name).resolve()
                if not (member.isfile() or member.isdir()) or CACHE.resolve() not in destination.parents:
                    raise RuntimeError('TurboJPEG 封存含不安全路徑或連結')
            tar.extractall(CACHE, members=members)
    compiler = env.get('CC', 'cc')
    if len(shlex.split(compiler)) != 1:
        raise RuntimeError('TurboJPEG CC 必須是單一編譯器路徑')
    signature = hashlib.sha256((VERSION + target + compiler + env.get('CGO_CFLAGS', '')).encode()).hexdigest()[:12]
    build = CACHE / f'{system}-{arch}-{signature}'
    library = build / 'libturbojpeg.a'
    if not library.exists():
        args = ['cmake', '-S', str(source), '-B', str(build), '-DCMAKE_BUILD_TYPE=Release',
                '-DENABLE_SHARED=OFF', '-DENABLE_STATIC=ON', '-DWITH_TURBOJPEG=ON',
                '-DWITH_SIMD=ON', '-DCMAKE_POSITION_INDEPENDENT_CODE=ON',
                f'-DCMAKE_INSTALL_PREFIX={build / "install"}',
                f'-DCMAKE_C_COMPILER={compiler}']
        if system == 'windows':
            args += ['-DCMAKE_SYSTEM_NAME=Windows', f'-DCMAKE_SYSTEM_PROCESSOR={"ARM64" if arch == "arm64" else "AMD64"}']
        elif system == 'darwin':
            args += [f'-DCMAKE_OSX_ARCHITECTURES={arch}']
        subprocess.run(args, check=True, env=env)
        subprocess.run(['cmake', '--build', str(build), '--target', 'turbojpeg-static', '--parallel', '4'], check=True, env=env)
        if not library.exists():
            raise RuntimeError(f'TurboJPEG 靜態函式庫不存在：{library}')
    result = dict(env)
    result['CGO_CFLAGS'] = (result.get('CGO_CFLAGS', '') + f' -I{shlex.quote(str(source / "src"))}').strip()
    result['CGO_LDFLAGS'] = (result.get('CGO_LDFLAGS', '') + f' -L{shlex.quote(str(build))}').strip()
    return result, source


def copy_licenses(source, folder):
    dest = Path(folder) / 'ThirdPartyLicenses' / 'libjpeg-turbo'
    dest.mkdir(parents=True, exist_ok=True)
    for name in ('LICENSE.md', 'README.ijg'):
        shutil.copy2(source / name, dest / name)


if __name__ == '__main__':
    if len(sys.argv) < 4 or sys.argv[2] != 'go' or sys.argv[3] not in ('build', 'test'):
        raise SystemExit('用法：python3 scripts/turbojpeg.py darwin/arm64 go build|test [參數]')
    target = sys.argv[1]
    if target not in ('darwin/arm64', 'windows/amd64', 'windows/arm64', 'linux/amd64', 'linux/arm64'):
        raise SystemExit('不支援的 TurboJPEG 建置目標')
    system, arch = target.split('/')
    env, _ = prepare(target, dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED='1'))
    subprocess.run(sys.argv[2:4] + ['-tags', 'turbojpeg'] + sys.argv[4:], env=env, check=True)
