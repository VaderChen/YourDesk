"""檢查 Windows PE 相依閉包，避免產出缺 DLL 的安裝包；不需要啟動 Windows。"""
from pathlib import Path
import struct

FFMPEG_DLLS = ('avcodec-62.dll', 'avutil-60.dll', 'swscale-9.dll')
SYSTEM_DLLS = set('kernel32.dll kernelbase.dll ntdll.dll user32.dll gdi32.dll advapi32.dll ole32.dll oleaut32.dll shell32.dll shlwapi.dll version.dll bcrypt.dll crypt32.dll secur32.dll ws2_32.dll iphlpapi.dll winmm.dll winhttp.dll wininet.dll userenv.dll ucrtbase.dll msvcrt.dll setupapi.dll comdlg32.dll comctl32.dll dwmapi.dll dxgi.dll d3d11.dll d3d12.dll mf.dll mfplat.dll mfreadwrite.dll propsys.dll rpcrt4.dll wtsapi32.dll imm32.dll opengl32.dll usp10.dll'.split())


def imports(path):
    data = Path(path).read_bytes()
    def u16(offset): return struct.unpack_from('<H', data, offset)[0]
    def u32(offset): return struct.unpack_from('<I', data, offset)[0]
    if data[:2] != b'MZ': raise ValueError(f'不是 PE：{path}')
    pe = u32(60)
    if data[pe:pe+4] != b'PE\0\0': raise ValueError(f'無效 PE：{path}')
    machine, sections, size = u16(pe+4), u16(pe+6), u16(pe+20)
    optional = pe+24
    magic = u16(optional)
    if magic not in (0x10b, 0x20b): raise ValueError(f'未知 PE 格式：{path}')
    table = optional+size
    def offset(rva):
        for i in range(sections):
            section = table+i*40
            length, base, rawsize, raw = struct.unpack_from('<IIII', data, section+8)
            if base <= rva < base+max(length, rawsize):
                result = raw+rva-base
                if result >= len(data): break
                return result
        raise ValueError(f'無效 PE RVA：{path}')
    import_rva = u32(optional+(112 if magic == 0x20b else 96)+8)
    found = []
    if import_rva:
        at = offset(import_rva)
        for i in range(4096):
            name = u32(at+i*20+12)
            if not name: break
            begin = offset(name)
            end = data.index(b'\0', begin, min(len(data), begin+512))
            found.append(data[begin:end].decode('ascii').lower())
        else: raise ValueError(f'PE import 數量異常：{path}')
    return machine, found


def validate(folder, roots=FFMPEG_DLLS):
    folder = Path(folder)
    files = {p.name.lower(): p for p in folder.iterdir() if p.is_file()}
    pending = [name.lower() for name in roots]
    seen = set()
    architecture = None
    while pending:
        name = pending.pop()
        if name in seen: continue
        seen.add(name)
        if name not in files: raise RuntimeError(f'Windows 發行包缺少相依檔案：{name}')
        machine, dependencies = imports(files[name])
        if architecture is not None and machine != architecture:
            raise RuntimeError(f'Windows EXE／DLL 架構不一致：{name}')
        architecture = machine
        for dependency in dependencies:
            if dependency in SYSTEM_DLLS or dependency.startswith(('api-ms-win-', 'ext-ms-win-')):
                continue
            pending.append(dependency)
    return seen
