"""Windows 隨附 FFmpeg 軟解函式庫；固定來源與雜湊，保留可替換的 LGPL 動態庫。"""
import hashlib
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
import windows_runtime
import ffmpeg_artifacts as artifacts
import macos_build

ROOT = Path(__file__).resolve().parent.parent
CACHE = ROOT / '.local-run' / 'software-video'
SOURCES = (
    ('ffmpeg-8.1.1', 'tar.xz', 'https://ffmpeg.org/releases/', 'b6863adde98898f42602017462871b5f6333e65aec803fdd7a6308639c52edf3'),
    ('libaom-3.13.1', 'tar.gz', 'https://storage.googleapis.com/aom-releases/', '19e45a5a7192d690565229983dad900e76b513a02306c12053fb9a262cbeca7d'),
)
NATIVE_PATH_RECIPE = 'relative-native-paths-v1'


def native_path_flags(roots):
    """Map compiler source/debug paths to stable, relative build names."""
    mappings = {}
    for source, replacement in ((Path.home(), 'home'), (Path(tempfile.gettempdir()), 'tmp'), *roots):
        for path in (Path(source).absolute(), Path(source).resolve()):
            if path != Path(path.anchor):
                mappings[str(path)] = replacement
    flags = ['-gno-record-gcc-switches']
    for source, replacement in sorted(mappings.items(), key=lambda item: len(item[0])):
        flags.extend('-f' + kind + '-prefix-map=' + source + '=' + replacement for kind in ('file', 'debug'))
    return flags


def native_environment(env, roots, for_configure=False):
    result = dict(env)
    arguments = native_path_flags(roots)
    if for_configure:
        # FFmpeg configure expands CFLAGS without shell re-parsing. Quotes are
        # literal there; its out-of-tree source paths cannot contain whitespace.
        if any(re.search(r'''[\s'"`$\\]''', argument) for argument in arguments):
            raise RuntimeError('FFmpeg 來源／建置目錄不支援空白或 shell 特殊字元；請使用一般相對專案路徑')
        flags = ' '.join(arguments)
    else:
        flags = shlex.join(arguments)
    for name in ('CFLAGS', 'CXXFLAGS'):
        result[name] = (result.get(name, '') + ' ' + flags).strip()
    return result


def configure_tool(command, env):
    """Keep tool locations in PATH, not FFmpeg's embedded configuration."""
    path = Path(command)
    if path.parent != Path('.'):
        env['PATH'] = str(path.absolute().parent) + os.pathsep + env.get('PATH', os.defpath)
        return path.name
    return command


def configure_tools(commands, env):
    original_path = env.get('PATH', os.defpath)
    expected = {}
    for name, command in commands.items():
        resolved = shutil.which(command, path=original_path)
        if not resolved:
            raise RuntimeError('FFmpeg 找不到指定工具：' + name)
        expected[name] = Path(resolved).resolve()
    selected = {name: configure_tool(command, env) for name, command in commands.items()}
    for name, command in selected.items():
        resolved = shutil.which(command, path=env.get('PATH', os.defpath))
        if not resolved or Path(resolved).resolve() != expected[name]:
            raise RuntimeError('FFmpeg PATH 工具名稱衝突：' + name + '；請使用一致的工具鏈目錄')
    return selected


def go_quote(argument):
    # cmd/go quoted.Split does not concatenate shell-escaped quote fragments.
    quote = "'" if "'" not in argument else '"'
    if quote in argument:
        raise RuntimeError('CGo 參數同時含單引號與雙引號，無法安全編碼建置路徑')
    return quote + argument + quote


def go_join(arguments):
    return ' '.join(go_quote(argument) for argument in arguments)


def validate_native_paths(paths):
    personal = re.compile(rb'(?:/(?:Users|home|Volumes)/|[A-Za-z]:[\\/](?:Users|Documents and Settings)[\\/])[^\x00-\x20/\\]+', re.I)
    for path in paths:
        if personal.search(path.read_bytes()):
            raise RuntimeError('原生影音產物的建置路徑驗證失敗：' + path.name)


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


def required_build_tools(arch, env):
    # 必須檢查傳給子程序的 PATH，不能誤用啟動 Python 時的環境。
    names = ['cmake', 'pkg-config', 'make']
    if arch == 'amd64':
        names.append('nasm')
    tools = {name: shutil.which(name, path=env.get('PATH', os.defpath)) for name in names}
    missing = [name for name in names if not tools[name]]
    if missing:
        hint = 'macOS 請執行 brew install cmake pkgconf'
        if arch == 'amd64':
            hint += ' nasm'
        if 'make' in missing:
            hint += '；缺少 make 時請先安裝 Xcode Command Line Tools（xcode-select --install）'
        raise RuntimeError('FFmpeg／libaom 建置缺少工具：' + ', '.join(missing) + '。' + hint + '，並確認工具所在目錄已加入 PATH。')
    return tools


def relocatable_pkg_config(path):
    # pcfiledir 是 pkg-config 的標準變數；不把專案的中文／搬移前路徑寫入 -I/-L。
    original = path.read_text()
    lines = original.splitlines()
    if sum(line.startswith('prefix=') for line in lines) != 1:
        raise RuntimeError(f'libaom 套件資訊缺少唯一的 prefix：{path}')
    updated = '\n'.join('prefix=${pcfiledir}/../..' if line.startswith('prefix=') else line for line in lines) + '\n'
    if updated != original:
        path.write_text(updated)


def prepare(target, env):
    # macOS 與 Windows 共用經固定來源驗證的 FFmpeg／libaom。
    system, arch = target.split('/')
    if system not in ('windows', 'darwin') or arch not in ('amd64', 'arm64'):
        raise RuntimeError('不支援的 FFmpeg 建置目標')
    env = macos_build.environment(target, env)
    cc, cxx = env.get('CC', 'cc'), env.get('CXX', 'c++')
    if len(shlex.split(cc)) != 1 or len(shlex.split(cxx)) != 1:
        raise RuntimeError('CC／CXX 必須是單一編譯器路徑')
    cc, cxx = shlex.split(cc)[0], shlex.split(cxx)[0]
    flag_names = ('CFLAGS', 'CXXFLAGS', 'CPPFLAGS', 'LDFLAGS', 'SDKROOT') + (('MACOSX_DEPLOYMENT_TARGET',) if system == 'darwin' else ())
    build_key = repr((SOURCES, target, cc, cxx, NATIVE_PATH_RECIPE, ('shared-v5-av1-videotoolbox' if system=='darwin' else 'shared-v4-av1-encoder'), tuple((k, env.get(k, '')) for k in flag_names)))
    signature = hashlib.sha256(build_key.encode()).hexdigest()[:12]
    build = CACHE / f'{system}-{arch}-{signature}'
    with artifacts.locked(CACHE, build):
        return prepare_locked(target, env, cc, cxx, build)


def prepare_locked(target, env, cc, cxx, build):
    system, arch = target.split('/')
    prefix = build / 'install'
    complete = build / 'complete'
    inputs = artifacts.build_inputs(Path(__file__), target, env, SOURCES)
    source_state = artifacts.source_state(CACHE, SOURCES)
    metadata = build / 'metadata'
    ready = artifacts.current(build, inputs, source_state)
    # 一次性接管舊版已完成產物，不因改用產物清單而重編現有 FFmpeg。
    legacy = (complete.is_file() and not (metadata / 'manifest.json').exists()
              and (build / 'ffmpeg/config.h').is_file() and (build / 'ffmpeg/config_components.h').is_file())
    if not ready and legacy:
        validate_build(system, prefix, build / 'ffmpeg')
        retain_metadata(build, CACHE / SOURCES[0][0], CACHE / SOURCES[1][0])
        artifacts.save(build, inputs, source_state)
        ready = True
    # 首次建置先檢查工具，再下載／解壓來源；有效快取不新增建置工具需求。
    if ready:
        validate_build(system, prefix, metadata)
        artifacts.clean_intermediates(CACHE, build)
        print(f'重用 FFmpeg 二進位產物：{build.name}', flush=True)
        return build_environment(env, prefix, system), prefix
    tools = required_build_tools(arch, env)
    ffmpeg, aom = sources()
    native_env = native_environment(env, ((ROOT, '.'), (ffmpeg, 'ffmpeg'), (aom, 'libaom'), (build, 'build')))
    buildenv = native_environment(env, ((ROOT, '.'), (ffmpeg, 'ffmpeg'), (aom, 'libaom'), (build, 'build')), for_configure=True)
    buildenv.update(PKG_CONFIG_LIBDIR='../install/lib/pkgconfig', PKG_CONFIG_PATH='')
    configure_commands = {'cc': cc, 'cxx': cxx, 'pkg-config': tools['pkg-config']}
    if system == 'windows':
        toolprefix = 'aarch64-w64-mingw32-' if arch == 'arm64' else 'x86_64-w64-mingw32-'
        configure_commands.update((name, str(Path(cc).parent / (toolprefix + name))) for name in ('ar', 'ranlib', 'nm', 'windres'))
    selected_tools = configure_tools(configure_commands, buildenv)
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
    if not ready:
        build.mkdir(parents=True, exist_ok=True)
        args = [tools['cmake'], '-S', os.path.relpath(aom, build), '-B', 'aom', '-DCMAKE_BUILD_TYPE=Release',
                '-DBUILD_SHARED_LIBS=OFF', '-DENABLE_DOCS=OFF', '-DENABLE_TESTS=OFF', '-DENABLE_EXAMPLES=OFF',
                '-DENABLE_TOOLS=OFF', '-DCONFIG_AV1_ENCODER=1', '-DCMAKE_POSITION_INDEPENDENT_CODE=ON',
                '-DCMAKE_INSTALL_PREFIX=install', '-DCMAKE_INSTALL_LIBDIR=lib',
                f'-DCMAKE_C_COMPILER={cc}', f'-DCMAKE_CXX_COMPILER={cxx}']
        if system == 'windows':
            args += ['-DCMAKE_SYSTEM_NAME=Windows', f'-DCMAKE_SYSTEM_PROCESSOR={"aarch64" if arch == "arm64" else "AMD64"}']
        else:
            args += [f'-DCMAKE_OSX_ARCHITECTURES={"arm64" if arch == "arm64" else "x86_64"}',
                     f'-DCMAKE_OSX_DEPLOYMENT_TARGET={macos_build.MINIMUM_VERSION}']
        subprocess.run(args, cwd=build, env=native_env, check=True)
        subprocess.run([tools['cmake'], '--build', 'aom', '--target', 'aom', 'aom_pc', '--parallel', '6'], cwd=build, env=native_env, check=True)
        subprocess.run([tools['cmake'], '--install', 'aom', '--prefix', 'install'], cwd=build, env=native_env, check=True)
        relocatable_pkg_config(prefix / 'lib/pkgconfig/aom.pc')
        work = build / 'ffmpeg'
        work.mkdir(exist_ok=True)
        args = [os.path.relpath(ffmpeg / 'configure', work), '--prefix=../install', f'--cc={selected_tools["cc"]}', f'--cxx={selected_tools["cxx"]}',
                '--disable-everything', '--disable-autodetect', '--disable-programs', '--disable-doc',
                '--disable-avdevice', '--disable-avformat', '--disable-avfilter', '--disable-swresample',
                '--enable-avcodec', '--enable-avutil', '--enable-swscale', '--enable-shared', '--disable-static',
                '--enable-libaom', '--enable-decoder=h264,hevc,libaom_av1', '--enable-encoder=libaom_av1',
                '--enable-parser=h264,hevc,av1', '--enable-pic', '--disable-debug',
                '--pkg-config-flags=--static', f'--pkg-config={selected_tools["pkg-config"]}']
        if system == 'darwin':
            minimum = f'-mmacosx-version-min={macos_build.MINIMUM_VERSION}'
            args += ['--enable-videotoolbox', '--enable-hwaccel=av1_videotoolbox', '--install-name-dir=@rpath',
                     f'--extra-cflags={minimum}', f'--extra-ldflags={minimum}']
        if system == 'windows':
            args += ['--enable-cross-compile', '--target-os=mingw32', '--disable-pthreads', '--enable-w32threads', f'--arch={"aarch64" if arch == "arm64" else "x86_64"}',
                     f'--cross-prefix={toolprefix}', *(f'--{name}={selected_tools[name]}' for name in ('ar', 'ranlib', 'nm', 'windres')),
                     '--extra-ldflags=-static-libgcc']
        try:
            subprocess.run(args, cwd=work, env=buildenv, check=True)
        except subprocess.CalledProcessError as error:
            raise RuntimeError(f'FFmpeg configure 失敗（結束碼：{error.returncode}）；詳細原因請查看 {work / "ffbuild/config.log"}') from error
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
        subprocess.run([tools['make'], '-j6'], cwd=work, env=buildenv, check=True)
        subprocess.run([tools['make'], 'install'], cwd=work, env=buildenv, check=True)

    validate_build(system, prefix, build / 'ffmpeg')
    retain_metadata(build, ffmpeg, aom)
    artifacts.save(build, inputs, artifacts.source_state(CACHE, SOURCES))
    complete.write_text('完成\n')
    artifacts.clean_intermediates(CACHE, build)
    return build_environment(env, prefix, system), prefix


def validate_build(system, prefix, configuration_folder):
    configuration = (configuration_folder / 'config.h').read_text()
    components = (configuration_folder / 'config_components.h').read_text()
    configured = re.search(r'^#define FFMPEG_CONFIGURATION "(.*)"$', configuration, re.M)
    if configured is None or re.search(r'--[a-z-]+=(?:[\\\'"])*(?:/|[A-Za-z]:[\\/])', configured[1]):
        raise RuntimeError('FFmpeg 組態必須使用相對路徑及 PATH 工具名稱')
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
    required = [prefix / 'include' / name for name in (
        'libavcodec/avcodec.h', 'libavutil/avutil.h', 'libswscale/swscale.h')]
    if system == 'darwin':
        required += [prefix / 'lib' / name for name in (
            'libavcodec.62.dylib', 'libavutil.60.dylib', 'libswscale.9.dylib',
            'libavcodec.dylib', 'libavutil.dylib', 'libswscale.dylib')]
    else:
        required += [prefix / 'lib' / f'lib{name}.dll.a' for name in ('avcodec', 'avutil', 'swscale')]
    if any(not path.is_file() or path.stat().st_size == 0 for path in required):
        raise RuntimeError('FFmpeg 已安裝產物不完整，缺少動態庫、連結庫或標頭')
    if system == 'darwin':
        for name in macos_build.RUNTIME_LIBRARIES:
            macos_build.verify_binary(prefix / 'lib' / name)
    libraries = list((prefix / 'bin').glob('*.dll')) if system == 'windows' else [prefix / 'lib' / name for name in macos_build.RUNTIME_LIBRARIES]
    validate_native_paths(libraries)


def retain_metadata(build, ffmpeg, aom):
    metadata = build / 'metadata'
    licenses = metadata / 'licenses'
    licenses.mkdir(parents=True, exist_ok=True)
    for name in ('config.h', 'config_components.h'):
        shutil.copy2(build / 'ffmpeg' / name, metadata / name)
    for name in ('COPYING.LGPLv2.1', 'LICENSE.md'):
        shutil.copy2(ffmpeg / name, licenses / name)
    for name in ('LICENSE', 'PATENTS'):
        shutil.copy2(aom / name, licenses / ('libaom-' + name))


def build_environment(env, prefix, system):
    result = dict(env)
    result['GOFLAGS'] = (result.get('GOFLAGS', '') + ' -trimpath').strip()
    path_flags = native_path_flags(((ROOT, '.'), (prefix, 'native')))
    result['CGO_CPPFLAGS'] = (result.get('CGO_CPPFLAGS', '') + ' ' + go_join(path_flags)).strip()
    # Go 的 quoted.Split 只接受整個參數加引號，不接受 -I'含空白路徑'。
    # CGo 會切換到各套件工作目錄，因此此處才從專案位置推導完整搜尋路徑。
    result['CGO_CFLAGS'] = (result.get('CGO_CFLAGS', '') + ' ' + go_quote('-I' + str(prefix / 'include'))).strip()
    result['CGO_LDFLAGS'] = (result.get('CGO_LDFLAGS', '') + ' ' + go_quote('-L' + str(prefix / 'lib'))).strip()
    if system == 'darwin':
        # CGo 預設拒絕以 @ 開頭的連結器參數；只允許套件內這兩個 macOS 路徑。
        allowed = r'-Wl,-rpath,@loader_path(?:/\.\./Frameworks)?'
        existing = result.get('CGO_LDFLAGS_ALLOW', '')
        result['CGO_LDFLAGS_ALLOW'] = f'(?:{existing})|(?:{allowed})' if existing else allowed
    return result


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
    validate_native_paths(dlls)
    for dll in dlls:
        shutil.copy2(dll, folder / dll.name)
    dest = folder / 'ThirdPartyLicenses' / 'FFmpeg'
    dest.mkdir(parents=True, exist_ok=True)
    for license_file in (prefix.parent / 'metadata/licenses').iterdir():
        shutil.copy2(license_file, dest / license_file.name)
    # 隨附精確來源及重建腳本，動態函式庫可由使用者替換。
    for name, suffix, _, _ in SOURCES:
        shutil.copy2(CACHE / f'{name}.{suffix}', dest)
    shutil.copy2(__file__, dest / 'ffmpeg.py')
    shutil.copy2(Path(__file__).with_name('windows_runtime.py'), dest / 'windows_runtime.py')
    shutil.copy2(Path(__file__).with_name('ffmpeg_artifacts.py'), dest / 'ffmpeg_artifacts.py')
    shutil.copy2(Path(__file__).with_name('macos_build.py'), dest / 'macos_build.py')
    (dest / 'README.txt').write_text('YourDesk 使用 FFmpeg 8.1.1（LGPL 2.1 或更新）及 libaom 3.13.1（BSD）。\n'
        'FFmpeg 未啟用 GPL 或 nonfree 元件。可替換 Windows 程式旁的 DLL 或 macOS App/Contents/Frameworks 內的 dylib；不限制為除錯修改函式庫所需的逆向工程。\n'
        '完整來源封存與重建腳本隨附。於 YourDesk 原始碼執行 scripts/ffmpeg.py 可重建。\n', encoding='utf-8')


if __name__ == '__main__':
    if len(sys.argv) < 4 or sys.argv[2:4] not in (['go', 'test'], ['go', 'build']):
        raise SystemExit('用法：python3 scripts/ffmpeg.py 平台/架構 go test|build [參數]')
    import release
    import turbojpeg
    import opus
    target = sys.argv[1]
    try:
        env, prefix = prepare(target, release.environment(target, True))
        env, jpeg_source = turbojpeg.prepare(target, env)
        env, opus_source = opus.prepare(target, env)
    except RuntimeError as error:
        raise SystemExit(str(error))
    except subprocess.CalledProcessError as error:
        print(f'原生影音依賴建置失敗（結束碼：{error.returncode}），請查看上方訊息。', file=sys.stderr)
        raise SystemExit(error.returncode)
    test_flags = []
    if sys.argv[3] == 'test' and target.startswith('darwin/'):
        # macOS 啟動工具鏈時可能清除 DYLD_*；在啟動測試執行檔的最後一步才設定。
        # 快取絕對路徑只存在測試程序環境，不寫入正式程式的 rpath。
        libraries = os.pathsep.join(filter(None, (str(prefix / 'lib'), env.get('DYLD_LIBRARY_PATH', ''))))
        test_flags = ['-exec', '/usr/bin/env ' + shlex.quote('DYLD_LIBRARY_PATH=' + libraries)]
    try:
        subprocess.run(sys.argv[2:4] + ['-tags', 'turbojpeg,ffmpeg,opus'] + test_flags + sys.argv[4:], env=env, check=True)
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode)
    if sys.argv[3] == 'build' and '-o' in sys.argv[4:]:
        executable = Path(sys.argv[sys.argv.index('-o') + 1]).resolve()
        if target.startswith('darwin/'):
            try:
                macos_build.verify_binary(executable)
            except RuntimeError as error:
                raise SystemExit(str(error))
        output = executable.parent
        copy_runtime(prefix, output)
        turbojpeg.copy_licenses(jpeg_source, output)
        opus.copy_licenses(opus_source, output)
