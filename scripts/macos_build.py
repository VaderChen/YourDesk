"""共用 macOS deployment target，並在發布前核對實際 Mach-O。"""
from pathlib import Path
import plistlib
import re
import shlex
import struct

# go.mod requires Go 1.27, whose runtime requires macOS 13 or newer.
# Lowering only the C compiler flag does not restore macOS 12 compatibility.
MINIMUM_VERSION = '13.0'
RUNTIME_LIBRARIES = ('libavcodec.62.dylib', 'libavutil.60.dylib', 'libswscale.9.dylib')


def version(value):
    parts = str(value).split('.')
    if not 1 <= len(parts) <= 3 or any(not part.isdigit() for part in parts):
        raise RuntimeError(f'無效的 macOS 版本：{value}')
    return tuple(int(part) for part in parts) + (0,) * (3 - len(parts))


def check_flags(name, value):
    # clang 的 -target 優先於 -mmacosx-version-min；靜態庫只看最終
    # Mach-O 不能發現所有不一致，因此在 CGo 和 CMake 輸入處一起檢查。
    flags = shlex.split(value)
    minimums = []
    for index, item in enumerate(flags):
        if item.startswith(('-mmacosx-version-min=', '-mmacos-version-min=')):
            minimums.append(item.split('=', 1)[1])
        target = None
        if item in ('-target', '--target'):
            if index + 1 >= len(flags):
                raise RuntimeError(f'{name} 缺少 target')
            target = flags[index + 1]
        elif item.startswith(('-target=', '--target=')):
            target = item.split('=', 1)[1]
        if target:
            match = re.search(r'-apple-(macosx?|darwin)([0-9.]+)', target)
            if match:
                if match[1] == 'darwin':
                    raise RuntimeError(f'{name} 請用 macos{MINIMUM_VERSION}，不可用 Darwin 核心版本覆蓋最低版本')
                minimums.append(match[2])
    linker = [part for item in flags for part in item.split(',') if part not in ('-Wl', '-Xlinker')]
    for index, item in enumerate(linker):
        if item == '-macosx_version_min' and index + 1 < len(linker):
            minimums.append(linker[index + 1])
        if item == '-platform_version' and linker[index + 1:index + 2] == ['macos'] and index + 2 < len(linker):
            minimums.append(linker[index + 2])
    if any(version(minimum) != version(MINIMUM_VERSION) for minimum in minimums):
        raise RuntimeError(f'{name} 的最低 macOS 版本與 {MINIMUM_VERSION} 不一致')


def environment(target, env):
    result = dict(env)
    if not target.startswith('darwin/'):
        return result
    configured = result.get('MACOSX_DEPLOYMENT_TARGET') or MINIMUM_VERSION
    if version(configured) != version(MINIMUM_VERSION):
        raise RuntimeError(f'MACOSX_DEPLOYMENT_TARGET 必須為 {MINIMUM_VERSION}，以符合產品最低版本')
    result['MACOSX_DEPLOYMENT_TARGET'] = MINIMUM_VERSION
    flag = f'-mmacosx-version-min={MINIMUM_VERSION}'
    for name in ('CFLAGS', 'CXXFLAGS', 'CPPFLAGS', 'LDFLAGS', 'CGO_CPPFLAGS', 'CGO_CFLAGS', 'CGO_CXXFLAGS', 'CGO_LDFLAGS'):
        check_flags(name, result.get(name, ''))
    for name, default in (('CGO_CFLAGS', '-O2 -g'), ('CGO_CXXFLAGS', '-O2 -g'), ('CGO_LDFLAGS', '')):
        value = result.get(name, default)
        flags = shlex.split(value)
        if flag not in flags:
            value = (value + ' ' + flag).strip()
        result[name] = value
    return result


def verify_binary(path, maximum=MINIMUM_VERSION):
    """僅讀取 Apple Silicon 產物的 header/load commands，不需執行產物。"""
    path = Path(path)
    with path.open('rb') as stream:
        header = stream.read(32)
        if len(header) != 32:
            raise RuntimeError(f'不完整的 Mach-O：{path}')
        magic, cpu, _, kind, count, size, _, _ = struct.unpack('<8I', header)
        if magic != 0xFEEDFACF or cpu != 0x0100000C or kind not in (2, 6, 8):
            raise RuntimeError(f'不是 arm64 macOS 執行檔／動態庫：{path}')
        if size > 1024 * 1024 or count > size // 8:
            raise RuntimeError(f'無效的 Mach-O load commands：{path}')
        commands = stream.read(size)
    if len(commands) != size:
        raise RuntimeError(f'不完整的 Mach-O load commands：{path}')
    offset, minimums = 0, []
    for _ in range(count):
        if offset + 8 > size:
            raise RuntimeError(f'不完整的 Mach-O command：{path}')
        command, length = struct.unpack_from('<2I', commands, offset)
        if length < 8 or offset + length > size:
            raise RuntimeError(f'無效的 Mach-O command 長度：{path}')
        if command == 0x32:  # LC_BUILD_VERSION
            if length < 24:
                raise RuntimeError(f'不完整的 LC_BUILD_VERSION：{path}')
            platform, minimum = struct.unpack_from('<2I', commands, offset + 8)
            if platform != 1:
                raise RuntimeError(f'Mach-O 不是 macOS 平台：{path}')
            minimums.append(minimum)
        elif command == 0x24:  # LC_VERSION_MIN_MACOSX
            if length < 16:
                raise RuntimeError(f'不完整的 LC_VERSION_MIN_MACOSX：{path}')
            minimums.append(struct.unpack_from('<I', commands, offset + 8)[0])
        offset += length
    if offset != size or not minimums:
        raise RuntimeError(f'Mach-O 缺少有效的最低 macOS 版本：{path}')
    for number in minimums:
        actual = (number >> 16, (number >> 8) & 255, number & 255)
        if actual > version(maximum):
            shown = '.'.join(str(part) for part in actual)
            raise RuntimeError(f'{path.name} 最低 macOS {shown} 高於允許的 {maximum}；請重新建置，不可發布')


def verify_application(app):
    app = Path(app)
    with (app / 'Contents/Info.plist').open('rb') as stream:
        info = plistlib.load(stream)
    if version(info.get('LSMinimumSystemVersion', '')) != version(MINIMUM_VERSION):
        raise RuntimeError('App 宣告的最低 macOS 版本不符')
    for name in ('YourDesk', 'yourdesk-client', 'yourdesk-remote'):
        verify_binary(app / 'Contents/MacOS' / name)
    for name in RUNTIME_LIBRARIES:
        verify_binary(app / 'Contents/Frameworks' / name)
    # Siri extension 也不得意外要求高於主程式的系統版本。
    extension = app / 'Contents/Extensions/YourDeskSiri.appex/Contents/MacOS/YourDeskSiri'
    if extension.exists():
        verify_binary(extension)
