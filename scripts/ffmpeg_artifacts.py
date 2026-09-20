"""保存原生影音產物的內容指紋；只清理可重建的工作目錄。"""
import hashlib
import json
import os
from contextlib import contextmanager
from pathlib import Path
import re
import shutil


def digest(path):
    value = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            value.update(block)
    return value.hexdigest()


def tree_digest(root):
    value = hashlib.sha256()
    for path in sorted(root.rglob('*')):
        if path.name.startswith('._') or path.name == '.DS_Store':
            continue
        if path.is_symlink():
            data = 'link:' + os.readlink(path)
        elif path.is_file():
            data = digest(path)
        else:
            continue
        value.update((path.relative_to(root).as_posix() + '\0' + data + '\n').encode())
    return value.hexdigest()


def source_state(cache, sources):
    # 未解壓的來源可從固定雜湊封存重建；已存在的來源需偵測本機修改。
    return {name: tree_digest(cache / name) for name, *_ in sources if (cache / name).is_dir()}


def build_inputs(script, target, env, sources):
    toolchain = {}
    for name, default in (('CC', 'cc'), ('CXX', 'c++')):
        command = env.get(name, default)
        tool = shutil.which(command, path=env.get('PATH', os.defpath))
        stamp = None
        if tool:
            info = Path(tool).stat()
            stamp = [str(Path(tool).resolve()), info.st_size, info.st_mtime_ns]
        toolchain[name] = [command, stamp]
    return {
        'schema': 1, 'target': target, 'sources': [list(source) for source in sources],
        # 清單／清理實作不改變原生編譯結果；只有 FFmpeg 編譯腳本屬於 recipe。
        'recipe': [digest(script)], 'toolchain': toolchain,
        'flags': {key: env.get(key, '') for key in (
            'CFLAGS', 'CXXFLAGS', 'CPPFLAGS', 'LDFLAGS', 'SDKROOT', 'MACOSX_DEPLOYMENT_TARGET')},
    }


def current(build, inputs, sources):
    try:
        record = json.loads((build / 'metadata/manifest.json').read_text())
        if (not isinstance(record, dict) or not isinstance(record.get('sources'), dict)
                or not isinstance(record.get('files'), dict)):
            return False
        previous = record['inputs']
        if (isinstance(previous, dict) and isinstance(previous.get('recipe'), list)
                and len(previous['recipe']) == 2 and isinstance(inputs.get('recipe'), list)
                and len(inputs['recipe']) == 1):
            # 舊清單第二項只代表此清理模組；移除它不需要重編已驗證的函式庫。
            previous = dict(previous, recipe=previous['recipe'][:1])
        if previous != inputs or not (build / 'complete').is_file():
            return False
        if any(record['sources'].get(name) != value for name, value in sources.items()):
            return False
        if not record['files']:
            return False
        for name, expected in record['files'].items():
            relative = Path(name)
            if relative.is_absolute() or '..' in relative.parts:
                return False
            path = build / relative
            if not path.is_file() or digest(path) != expected:
                return False
        return True
    except (OSError, ValueError, KeyError, TypeError):
        return False


@contextmanager
def locked(cache, build):
    validate_directory(cache, build)
    build.mkdir(parents=True, exist_ok=True)
    # 作業系統檔案鎖會在程序退出時釋放，避免重用者刪除另一個建置中的物件檔。
    with (build / '.build.lock').open('a+b') as stream:
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


def save(build, inputs, sources):
    files = {}
    for root in (build / 'install', build / 'metadata'):
        for path in sorted(root.rglob('*')):
            if path.name in ('manifest.json', 'manifest.json.tmp') or path.name.startswith('._'):
                continue
            if path.is_file():
                files[path.relative_to(build).as_posix()] = digest(path)
    record = {'inputs': inputs, 'sources': sources, 'files': files}
    temporary = build / 'metadata/manifest.json.tmp'
    temporary.write_text(json.dumps(record, sort_keys=True, ensure_ascii=False, indent=2) + '\n')
    temporary.replace(build / 'metadata/manifest.json')


def validate_directory(cache, build):
    if (build.parent.resolve() != cache.resolve() or build.is_symlink()
            or not re.fullmatch(r'(darwin|windows)-(arm64|amd64)-[0-9a-f]{12}', build.name)):
        raise RuntimeError(f'拒絕清理非預期的 FFmpeg 工作目錄：{build}')


def clean_intermediates(cache, build):
    # 僅接受一個受管理的平台建置目錄，拒絕符號連結；不碰 install／來源／封存。
    validate_directory(cache, build)
    targets = [build / name for name in ('aom', 'ffmpeg')]
    if any(path.is_symlink() for path in targets):
        raise RuntimeError('拒絕清理指向其他位置的 FFmpeg 工作目錄連結')

    def remove_error(function, path, exception):
        # macOS 可能隨本體自動移除 AppleDouble；只忽略這種已消失的副檔。
        if (isinstance(exception[1], FileNotFoundError) and Path(path).name.startswith('._')
                and build.is_dir()):
            return
        raise exception[1]

    for path in targets:
        if path.is_dir():
            shutil.rmtree(path, onerror=remove_error)
