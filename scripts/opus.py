"""固定版本及 SHA-256 的 libopus 靜態建置；不依賴使用者安裝編解碼器。"""
from contextlib import contextmanager
import hashlib
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tarfile
import urllib.request

import ffmpeg
import ffmpeg_artifacts
import macos_build

VERSION = '1.6.1'
# https://opus-codec.org/release/stable/2026/01/14/libopus-1_6_1.html
SHA256 = '6ffcb593207be92584df15b32466ed64bbec99109f007c82205f0194572411a1'
ROOT = Path(__file__).resolve().parent.parent
CACHE = ROOT / '.local-run' / 'opus'


@contextmanager
def cache_lock():
    with (CACHE / '.build.lock').open('a+b') as stream:
        if os.name == 'nt':
            import msvcrt
            if stream.tell() == 0:
                stream.write(b'0')
                stream.flush()
            stream.seek(0)
            msvcrt.locking(stream.fileno(), msvcrt.LK_LOCK, 1)
        else:
            import fcntl
            fcntl.flock(stream.fileno(), fcntl.LOCK_EX)
        try:
            yield
        finally:
            if os.name == 'nt':
                stream.seek(0)
                msvcrt.locking(stream.fileno(), msvcrt.LK_UNLCK, 1)
            else:
                fcntl.flock(stream.fileno(), fcntl.LOCK_UN)


def prepare(target, env):
    env = macos_build.environment(target, env)
    if not shutil.which('cmake'):
        raise RuntimeError('Opus 建置需要 CMake')
    system, arch = target.split('/')
    compiler = env.get('CC', 'cc')
    if len(shlex.split(compiler)) != 1:
        raise RuntimeError('Opus CC 必須是單一編譯器路徑')
    recipe = (VERSION, SHA256, target, compiler, ffmpeg_artifacts.digest(__file__),
              ffmpeg.NATIVE_PATH_RECIPE,
              tuple((key, env.get(key, '')) for key in (
                  'CFLAGS', 'CXXFLAGS', 'CPPFLAGS', 'LDFLAGS', 'SDKROOT', 'MACOSX_DEPLOYMENT_TARGET')))
    signature = hashlib.sha256(repr(recipe).encode()).hexdigest()[:12]
    source = CACHE / f'opus-{VERSION}'
    build = CACHE / f'{system}-{arch}-{signature}'
    library = build / 'libopus.a'
    stamp = build / 'complete.sha256'
    # 同時啟動 Client／顯示區域建置時，共用下載及靜態庫不得互相覆寫。
    CACHE.mkdir(parents=True, exist_ok=True)
    with cache_lock():
        archive = CACHE / f'opus-{VERSION}.tar.gz'
        if not archive.is_file() or ffmpeg_artifacts.digest(archive) != SHA256:
            with urllib.request.urlopen(f'https://downloads.xiph.org/releases/opus/{archive.name}', timeout=60) as response:
                data = response.read(64 * 1024 * 1024 + 1)
            if len(data) > 64 * 1024 * 1024 or hashlib.sha256(data).hexdigest() != SHA256:
                raise RuntimeError('Opus 來源 SHA-256 不符')
            archive.write_bytes(data)
        if not source.is_dir():
            with tarfile.open(archive) as tar:
                members = tar.getmembers()
                for member in members:
                    destination = (CACHE / member.name).resolve()
                    if not (member.isfile() or member.isdir()) or source.resolve() not in (destination, *destination.parents):
                        raise RuntimeError('Opus 封存含不安全路徑或連結')
                tar.extractall(CACHE, members=members)
        current = library.is_file() and stamp.is_file() and stamp.read_text() == ffmpeg_artifacts.digest(library)
        if not current:
            build.mkdir(parents=True, exist_ok=True)
            native_env = ffmpeg.native_environment(env, ((ROOT, '.'), (source, 'opus'), (build, 'build')))
            args = ['cmake', '-S', os.path.relpath(source, build), '-B', '.', '-DCMAKE_BUILD_TYPE=Release',
                    '-DBUILD_SHARED_LIBS=OFF', '-DOPUS_BUILD_SHARED_LIBRARY=OFF',
                    '-DOPUS_BUILD_TESTING=OFF', '-DOPUS_BUILD_PROGRAMS=OFF',
                    '-DOPUS_DRED=OFF', '-DOPUS_OSCE=OFF', '-DOPUS_DEEP_PLC=OFF',
                    '-DCMAKE_POSITION_INDEPENDENT_CODE=ON', '-DCMAKE_INSTALL_PREFIX=install',
                    f'-DCMAKE_C_COMPILER={compiler}']
            if system == 'windows':
                args += ['-DCMAKE_SYSTEM_NAME=Windows', f'-DCMAKE_SYSTEM_PROCESSOR={"aarch64" if arch == "arm64" else "AMD64"}']
            elif system == 'darwin':
                args += [f'-DCMAKE_OSX_ARCHITECTURES={arch}',
                         f'-DCMAKE_OSX_DEPLOYMENT_TARGET={macos_build.MINIMUM_VERSION}']
            if arch == 'arm64':
                # ARM64 桌面目標皆具 NEON；避免落入僅支援 MSVC 的 Windows ARM CPU 探測。
                args += ['-DOPUS_MAY_HAVE_NEON=OFF', '-DOPUS_PRESUME_NEON=ON']
                # libopus 1.6.1 的標頭宣告仍依賴 MAY_HAVE，與是否產生 RTCD 分開。
                native_env['CFLAGS'] += ' -DOPUS_ARM_MAY_HAVE_NEON=1 -DOPUS_ARM_MAY_HAVE_NEON_INTR=1'
            subprocess.run(args, cwd=build, check=True, env=native_env)
            subprocess.run(['cmake', '--build', '.', '--target', 'opus', '--parallel', '4'], cwd=build, check=True, env=native_env)
            ffmpeg.validate_native_paths([library])
            stamp.write_text(ffmpeg_artifacts.digest(library))
        ffmpeg.validate_native_paths([library])
    result = dict(env)
    if '-trimpath' not in shlex.split(result.get('GOFLAGS', '')):
        result['GOFLAGS'] = (result.get('GOFLAGS', '') + ' -trimpath').strip()
    path_flags = ffmpeg.go_join(ffmpeg.native_path_flags(((ROOT, '.'), (source, 'opus'), (build, 'build'))))
    result['CGO_CPPFLAGS'] = (result.get('CGO_CPPFLAGS', '') + ' ' + path_flags).strip()
    result['CGO_CFLAGS'] = (result.get('CGO_CFLAGS', '') + ' ' + ffmpeg.go_quote('-I' + str(source / 'include'))).strip()
    result['CGO_LDFLAGS'] = (result.get('CGO_LDFLAGS', '') + ' ' + ffmpeg.go_quote('-L' + str(build))).strip()
    return result, source


def copy_licenses(source, folder):
    destination = Path(folder) / 'ThirdPartyLicenses' / 'Opus'
    destination.mkdir(parents=True, exist_ok=True)
    for name in ('COPYING', 'AUTHORS'):
        shutil.copy2(source / name, destination / name)
    (destination / 'README.txt').write_text(
        f'YourDesk 靜態連結 libopus {VERSION}（BSD）。\n'
        f'來源：https://downloads.xiph.org/releases/opus/opus-{VERSION}.tar.gz\n'
        f'SHA-256：{SHA256}\n', encoding='utf-8')


if __name__ == '__main__':
    if len(sys.argv) < 4 or sys.argv[2:4] not in (['go', 'test'], ['go', 'build']):
        raise SystemExit('用法：python3 scripts/opus.py 平台/架構 go test|build [參數]')
    import release
    target = sys.argv[1]
    env, source = prepare(target, release.environment(target, True))
    subprocess.run(sys.argv[2:4] + ['-tags', 'opus'] + sys.argv[4:], env=env, check=True)
    if sys.argv[3] == 'build' and '-o' in sys.argv[4:]:
        copy_licenses(source, Path(sys.argv[sys.argv.index('-o') + 1]).resolve().parent)
