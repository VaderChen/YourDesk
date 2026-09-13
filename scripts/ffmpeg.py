"""Windows 隨附 FFmpeg 軟解函式庫；固定來源與雜湊，保留可替換的 LGPL 動態庫。"""
import hashlib
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tarfile
import urllib.request
import windows_runtime

ROOT = Path(__file__).resolve().parent.parent
CACHE = ROOT / '.local-run' / 'software-video'
SOURCES = (
    ('ffmpeg-8.1.1', 'tar.xz', 'https://ffmpeg.org/releases/', 'b6863adde98898f42602017462871b5f6333e65aec803fdd7a6308639c52edf3'),
    ('libaom-3.13.1', 'tar.gz', 'https://storage.googleapis.com/aom-releases/', '19e45a5a7192d690565229983dad900e76b513a02306c12053fb9a262cbeca7d'),
)


def sources():
    CACHE.mkdir(parents=True, exist_ok=True)
    paths = []
    for name, suffix, base, digest in SOURCES:
        archive = CACHE / f'{name}.{suffix}'
        if not archive.exists() or hashlib.sha256(archive.read_bytes()).hexdigest() != digest:
            with urllib.request.urlopen(base + archive.name, timeout=60) as response:
                data = response.read(64 * 1024 * 1024 + 1)
            if hashlib.sha256(data).hexdigest() != digest:
                raise RuntimeError(f'{name} SHA-256 不符')
            archive.write_bytes(data)
        dest = CACHE / name
        if not dest.exists():
            dest.mkdir()
            with tarfile.open(archive) as tar:
                for member in tar.getmembers():
                    parts = Path(member.name).parts[1:]
                    if not parts:
                        continue
                    member.name = str(Path(*parts))
                    resolved = (dest / member.name).resolve()
                    if dest.resolve() not in resolved.parents or not (member.isfile() or member.isdir()):
                        raise RuntimeError('來源封存含不安全路徑或連結')
                    tar.extract(member, dest)
        paths.append(dest)
    return paths


def prepare(target, env):
    # darwin 僅供同一套 CPU 解碼器的本機 Smoke，不加入 Mac 產品。
    system, arch = target.split('/')
    if system not in ('windows', 'darwin') or arch not in ('amd64', 'arm64'):
        raise RuntimeError('不支援的 FFmpeg 建置目標')
    ffmpeg, aom = sources()
    # NASM 3 分開一般與格式說明；另外取得最佳化選項，保留原有能力檢查。
    nasm_check = aom / 'build/cmake/aom_optimization.cmake'
    original = nasm_check.read_text().replace('${CMAKE_ASM_NASM_COMPILER} -h\n', '${CMAKE_ASM_NASM_COMPILER} -hf\n')
    marker = '  if(NOT "${nasm_helptext}" MATCHES "-Ox")'
    addition = '  execute_process(COMMAND ${CMAKE_ASM_NASM_COMPILER} -h -O OUTPUT_VARIABLE nasm_opts)\n  string(APPEND nasm_helptext "${nasm_opts}")\n'
    if addition not in original:
        original = original.replace(marker, addition + marker)
    nasm_check.write_text(original)
    # Windows 採 libaom 內建 Win32 執行緒，不額外依賴 winpthreads DLL。
    thread_config = aom / 'build/cmake/aom_configure.cmake'
    original = thread_config.read_text()
    marker = 'set(HAVE_PTHREAD_H ${CMAKE_USE_PTHREADS_INIT})'
    patched = marker + '\nif(WIN32)\n  set(HAVE_PTHREAD_H 0)\nendif()'
    if patched not in original:
        thread_config.write_text(original.replace(marker, patched))
    cc, cxx = env.get('CC', 'cc'), env.get('CXX', 'c++')
    if len(shlex.split(cc)) != 1 or len(shlex.split(cxx)) != 1:
        raise RuntimeError('CC／CXX 必須是單一編譯器路徑')
    build_key = repr((SOURCES, target, cc, cxx, ('shared-v5-av1-videotoolbox' if system=='darwin' else 'shared-v4-av1-encoder'), tuple((k, env.get(k, '')) for k in ('CFLAGS', 'CXXFLAGS', 'LDFLAGS'))))
    signature = hashlib.sha256(build_key.encode()).hexdigest()[:12]
    build = CACHE / f'{system}-{arch}-{signature}'
    prefix = build / 'install'
    complete = build / 'complete'
    if not complete.exists():
        build.mkdir(parents=True, exist_ok=True)
        args = ['cmake', '-S', str(aom), '-B', str(build / 'aom'), '-DCMAKE_BUILD_TYPE=Release',
                '-DBUILD_SHARED_LIBS=OFF', '-DENABLE_DOCS=OFF', '-DENABLE_TESTS=OFF', '-DENABLE_EXAMPLES=OFF',
                '-DENABLE_TOOLS=OFF', '-DCONFIG_AV1_ENCODER=1', '-DCMAKE_POSITION_INDEPENDENT_CODE=ON',
                f'-DCMAKE_INSTALL_PREFIX={prefix}', '-DCMAKE_INSTALL_LIBDIR=lib',
                f'-DCMAKE_C_COMPILER={cc}', f'-DCMAKE_CXX_COMPILER={cxx}']
        if system == 'windows':
            args += ['-DCMAKE_SYSTEM_NAME=Windows', f'-DCMAKE_SYSTEM_PROCESSOR={"aarch64" if arch == "arm64" else "AMD64"}']
        else:
            args += [f'-DCMAKE_OSX_ARCHITECTURES={"arm64" if arch == "arm64" else "x86_64"}']
        subprocess.run(args, env=env, check=True)
        subprocess.run(['cmake', '--build', str(build / 'aom'), '--target', 'aom', 'aom_pc', '--parallel', '6'], env=env, check=True)
        subprocess.run(['cmake', '--install', str(build / 'aom')], env=env, check=True)
        work = build / 'ffmpeg'
        work.mkdir(exist_ok=True)
        buildenv = dict(env, PKG_CONFIG_LIBDIR=str(prefix / 'lib/pkgconfig'), PKG_CONFIG_PATH='')
        args = [str(ffmpeg / 'configure'), f'--prefix={prefix}', f'--cc={cc}', f'--cxx={cxx}',
                '--disable-everything', '--disable-autodetect', '--disable-programs', '--disable-doc',
                '--disable-avdevice', '--disable-avformat', '--disable-avfilter', '--disable-swresample',
                '--enable-avcodec', '--enable-avutil', '--enable-swscale', '--enable-shared', '--disable-static',
                '--enable-libaom', '--enable-decoder=h264,hevc,libaom_av1', '--enable-encoder=libaom_av1',
                '--enable-parser=h264,hevc,av1', '--enable-pic', '--disable-debug',
                '--pkg-config-flags=--static', f'--pkg-config={shutil.which("pkg-config")}']
        if system == 'darwin':
            args += ['--enable-videotoolbox', '--enable-hwaccel=av1_videotoolbox', '--install-name-dir=@rpath']
        if system == 'windows':
            toolprefix = str(Path(cc).parent / ('aarch64-w64-mingw32-' if arch == 'arm64' else 'x86_64-w64-mingw32-'))
            args += ['--enable-cross-compile', '--target-os=mingw32', '--disable-pthreads', '--enable-w32threads', f'--arch={"aarch64" if arch == "arm64" else "x86_64"}',
                     f'--cross-prefix={toolprefix}', f'--ar={toolprefix}ar', f'--ranlib={toolprefix}ranlib', f'--nm={toolprefix}nm',
                     f'--windres={toolprefix}windres', '--extra-ldflags=-static-libgcc']
        subprocess.run(args, cwd=work, env=buildenv, check=True)
        if system == 'windows':
            # MinGW GCC 的 POSIX 時鐘／sleep 包裝也會引入 winpthreads，
            # 即使 HAVE_PTHREADS=0。改用 FFmpeg 既有的 Windows 時間及 Sleep 路徑。
            config_path = work / 'config.h'
            config_text = config_path.read_text()
            for feature in ('CLOCK_GETTIME', 'NANOSLEEP', 'USLEEP'):
                config_text = config_text.replace(f'#define HAVE_{feature} 1', f'#define HAVE_{feature} 0')
            config_path.write_text(config_text)
        # 前次中斷可能僅留下 DLL，重新連結以產出完整 import library。
        if system == 'windows':
            for dll in work.glob('lib*/*.dll'):
                dll.unlink()
        subprocess.run(['make', '-j6'], cwd=work, env=buildenv, check=True)
        subprocess.run(['make', 'install'], cwd=work, env=buildenv, check=True)

    configuration = (build / 'ffmpeg/config.h').read_text()
    components = (build / 'ffmpeg/config_components.h').read_text()
    if '#define CONFIG_LIBAOM_AV1_ENCODER 1' not in components:
        raise RuntimeError('FFmpeg 快取未包含 AV1 軟體編碼器')
    if system == 'darwin' and '#define CONFIG_AV1_VIDEOTOOLBOX_HWACCEL 1' not in components:
        raise RuntimeError('FFmpeg 快取未包含 VideoToolbox AV1 硬解')
    if '#define CONFIG_GPL 0' not in configuration or '#define CONFIG_NONFREE 0' not in configuration:
        raise RuntimeError('拒絕非 LGPL 的 FFmpeg 組態')
    if system == 'windows':
        if '#define HAVE_PTHREADS 0' not in configuration or '#define HAVE_W32THREADS 1' not in configuration:
            raise RuntimeError('Windows FFmpeg 必須採用 Win32 執行緒')
        windows_runtime.validate(prefix / 'bin')
    complete.write_text('完成\n')
    result = dict(env)
    result['CGO_CFLAGS'] = (result.get('CGO_CFLAGS', '') + f' -I{shlex.quote(str(prefix / "include"))}').strip()
    result['CGO_LDFLAGS'] = (result.get('CGO_LDFLAGS', '') + f' -L{shlex.quote(str(prefix / "lib"))}').strip()
    if system == 'darwin':
        result['CGO_LDFLAGS'] += ' -Wl,-rpath,@loader_path -Wl,-rpath,@loader_path/../Frameworks'
    return result, prefix


def copy_runtime(prefix, folder):
    folder = Path(folder)
    if (prefix / 'bin/avcodec-62.dll').exists():
        windows_runtime.validate(prefix / 'bin')
        dlls = list((prefix / 'bin').glob('*.dll'))
    else:
        dlls = [prefix / 'lib' / name for name in ('libavcodec.62.dylib','libavutil.60.dylib','libswscale.9.dylib')]
        if not all(p.is_file() for p in dlls):
            raise RuntimeError('缺少 macOS FFmpeg 動態庫')
    if len(dlls) != 3:
        raise RuntimeError(f'FFmpeg 動態庫數量不符：{dlls}')
    for dll in dlls:
        shutil.copy2(dll, folder / dll.name)
    dest = folder / 'ThirdPartyLicenses' / 'FFmpeg'
    dest.mkdir(parents=True, exist_ok=True)
    ffmpeg, aom = sources()
    for name in ('COPYING.LGPLv2.1', 'LICENSE.md'):
        shutil.copy2(ffmpeg / name, dest / name)
    for name in ('LICENSE', 'PATENTS'):
        shutil.copy2(aom / name, dest / ('libaom-' + name))
    # 隨附精確來源及重建腳本，動態函式庫可由使用者替換。
    for name, suffix, _, _ in SOURCES:
        shutil.copy2(CACHE / f'{name}.{suffix}', dest)
    shutil.copy2(__file__, dest / 'ffmpeg.py')
    shutil.copy2(Path(__file__).with_name('windows_runtime.py'), dest / 'windows_runtime.py')
    (dest / 'README.txt').write_text('YourDesk 使用 FFmpeg 8.1.1（LGPL 2.1 或更新）及 libaom 3.13.1（BSD）。\n'
        'FFmpeg 未啟用 GPL 或 nonfree 元件。可替換 Windows 程式旁的 DLL 或 macOS App/Contents/Frameworks 內的 dylib；不限制為除錯修改函式庫所需的逆向工程。\n'
        '完整來源封存與重建腳本隨附。於 YourDesk 原始碼執行 scripts/ffmpeg.py 可重建。\n', encoding='utf-8')


if __name__ == '__main__':
    if len(sys.argv) < 4 or sys.argv[2:4] not in (['go', 'test'], ['go', 'build']):
        raise SystemExit('用法：python3 scripts/ffmpeg.py 平台/架構 go test|build [參數]')
    import release
    import turbojpeg
    target = sys.argv[1]
    env, prefix = prepare(target, release.environment(target, True))
    env, jpeg_source = turbojpeg.prepare(target, env)
    if sys.argv[3] == 'test' and target.startswith('darwin/'):
        env['CGO_LDFLAGS'] += ' -Wl,-rpath,' + shlex.quote(str(prefix / 'lib'))
    subprocess.run(sys.argv[2:4] + ['-tags', 'turbojpeg,ffmpeg'] + sys.argv[4:], env=env, check=True)
    if sys.argv[3] == 'build' and '-o' in sys.argv[4:]:
        output = Path(sys.argv[sys.argv.index('-o') + 1]).resolve().parent
        copy_runtime(prefix, output)
        turbojpeg.copy_licenses(jpeg_source, output)
