#!/usr/bin/env python3
"""Build/verify the pinned Android core without replacing a known artifact on failure."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
CORE = ROOT / "android/core"
AAR = ROOT / "android/libs/androidcore.aar"
MANIFEST = AAR.with_suffix(".manifest.json")
PAGE_SIZE = 16384
PERSONAL_PATH = re.compile(
    rb"(?:/(?:Users|home|Volumes)/|[A-Za-z]:[\\/](?:Users|Documents and Settings)[\\/])"
    rb"[^\x00-\x20/\\\"'<>]+", re.IGNORECASE)


def private_build_environment(environment, root=ROOT, home=None, temporary=None):
    """Trim Go paths and C/C++ header/macro paths without recording local roots.

    The pinned gomobile forwards -trimpath and inherits Android CGO_CPPFLAGS.
    Go also maps package/work directories itself when -trimpath is enabled.
    These extra maps cover absolute header paths outside the package, including
    the SDK. CPPFLAGS is shared by both C and C++ without changing their defaults.
    """
    env = environment.copy()
    env["GOWORK"] = "off"
    env["GOFLAGS"] = "-mod=readonly -trimpath"
    roots = [(home or Path.home(), "toolchain/home"),
             (temporary or tempfile.gettempdir(), "build/tmp"), (root, ".")]
    roots.extend((env[key], "toolchain/" + name) for key, name in (
        ("ANDROID_HOME", "android-sdk"), ("ANDROID_NDK_HOME", "android-ndk"),
        ("GOTMPDIR", "go-tmp")) if env.get(key))
    mappings = {}
    for path, replacement in roots:
        for source in (Path(path).absolute(), Path(path).resolve()):
            # Never use an entire filesystem as a prefix-map source.
            if source != Path(source.anchor):
                mappings[str(source)] = replacement
    flags = []
    # Clang uses the last matching map: place specific roots after their parents.
    for source, replacement in sorted(mappings.items(), key=lambda item: len(item[0])):
        for kind in ("file", "debug"):
            flag = "-f" + kind + "-prefix-map=" + source + "=" + replacement
            # cmd/go uses quoted.Split, not a shell: quote each complete token.
            quote = "'" if "'" not in flag else '"'
            if quote in flag:
                raise ValueError("建置目錄同時含單引號與雙引號，無法安全編碼 CGO 路徑設定。")
            flags.append(quote + flag + quote)
    env["CGO_CPPFLAGS"] = " ".join(filter(None, [env.get("CGO_CPPFLAGS", ""), *flags]))
    return env


def verify_private_paths(archive):
    """Reject personal build roots in native code, metadata and Java archives.

    Paths such as /proc and /system are legitimate
    runtime/toolchain paths; this is not a blanket ban on absolute paths.
    Report only the member name, never the private path found inside it.
    """
    def inspect(z):
        for member in z.infolist():
            if member.is_dir():
                continue
            data = z.read(member)
            if PERSONAL_PATH.search(member.filename.encode()) or PERSONAL_PATH.search(data):
                raise ValueError("Android core 含個人建置路徑，拒絕發布。")
            if member.filename.endswith(".jar"):
                with zipfile.ZipFile(io.BytesIO(data)) as nested:
                    # AAR classes.jar may be compressed, hiding paths from an
                    # outer-byte scan. Java class members are not nested jars.
                    for entry in nested.infolist():
                        if PERSONAL_PATH.search(entry.filename.encode()) or PERSONAL_PATH.search(nested.read(entry)):
                            raise ValueError("Android core 的 Java archive 含個人建置路徑，拒絕發布。")
    with zipfile.ZipFile(archive) as archive_file:
        inspect(archive_file)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def source_digest(core=CORE):
    h = hashlib.sha256()
    for path in sorted(core.rglob("*")):
        if not path.is_file() or any(part.startswith(".") for part in path.relative_to(core).parts):
            continue
        # gomobile packages assets/ too; cgo, assembly and go:embed resources
        # affect the binary just as Go source does. Only scratch/test files
        # are excluded, not arbitrary non-Go resources.
        if path.name.endswith(("_test.go", ".bak", ".pyc")):
            continue
        h.update(path.relative_to(core).as_posix().encode() + b"\0")
        h.update(path.read_bytes())
    h.update(Path(__file__).read_bytes())
    return h.hexdigest()


def elf_error(data, machine=None):
    if len(data) < 64 or data[:6] != b"\x7fELF\x02\x01":
        return "不是 little-endian ELF64"
    elf_type, elf_machine, elf_version = struct.unpack_from("<HHI", data, 16)
    if elf_type != 3 or elf_version != 1 or (machine is not None and elf_machine != machine):
        return "ELF 類型／架構／版本不符"
    phoff = struct.unpack_from("<Q", data, 32)[0]
    entsize, count = struct.unpack_from("<HH", data, 54)
    if entsize < 56 or count == 0 or phoff + entsize * count > len(data):
        return "ELF program headers 無效"
    loads = 0
    for index in range(count):
        kind, _, offset, address, _, file_size, memory_size, alignment = struct.unpack_from("<IIQQQQQQ", data, phoff + index * entsize)
        if kind == 1:
            loads += 1
            if file_size > memory_size or offset + file_size > len(data):
                return "PT_LOAD 區段大小／檔案範圍無效"
            if alignment < PAGE_SIZE or alignment & (alignment - 1) or offset % alignment != address % alignment:
                return "PT_LOAD 不符合 16 KB 對齊"
        if kind == 0x6474e552 and (address + memory_size) % PAGE_SIZE:
            return "GNU_RELRO 尾端未對齊 16 KB"
    return None if loads else "缺少 PT_LOAD"


def verify_native(archive, apk=False):
    errors = []
    with zipfile.ZipFile(archive) as z:
        members = [i for i in z.infolist() if i.filename.endswith(".so")
                   and ("/arm64-v8a/" in i.filename or "/x86_64/" in i.filename)]
        if not members:
            raise ValueError("找不到 64-bit native library")
        for member in members:
            machine = 183 if "/arm64-v8a/" in member.filename else 62
            problem = elf_error(z.read(member), machine)
            if problem:
                errors.append(member.filename + ": " + problem)
            if apk and member.compress_type == zipfile.ZIP_STORED:
                with open(archive, "rb") as stream:
                    stream.seek(member.header_offset)
                    header = stream.read(30)
                name_len, extra_len = struct.unpack_from("<HH", header, 26)
                if (member.header_offset + 30 + name_len + extra_len) % PAGE_SIZE:
                    errors.append(member.filename + ": APK ZIP data 未對齊 16 KB")
            elif apk and member.compress_type != zipfile.ZIP_DEFLATED:
                errors.append(member.filename + ": APK native library 壓縮格式不支援")
        if not apk and "jni/arm64-v8a/libgojni.so" not in z.namelist():
            errors.append("缺少 jni/arm64-v8a/libgojni.so")
    if errors:
        raise ValueError("\n".join(errors))


def verify(aar=AAR, manifest=MANIFEST, core=CORE):
    if not aar.is_file() or not manifest.is_file():
        raise ValueError("Android core 缺少來源驗證記錄；請先執行 python3 scripts/android_core.py build")
    record = json.loads(manifest.read_text())
    if not isinstance(record, dict) or record.get("format") != 1 or record.get("sourceSha256") != source_digest(core) or record.get("aarSha256") != digest(aar):
        raise ValueError("Android core AAR 已過期或內容不符；請執行 python3 scripts/android_core.py build")
    verify_native(aar)
    verify_private_paths(aar)


def publish(candidate, staged_manifest, aar, manifest, validator):
    """Publish a verified pair; any failure restores originals and retains .bak.

    The short exclusive lock prevents two builds from interleaving replacements.
    A crash leaves the lock/backups visible for manual recovery, never overwritten.
    """
    lock = aar.with_name("." + aar.name + ".lock")
    try:
        descriptor = os.open(lock, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError as error:
        raise ValueError("Android core 正在發布或上次發布中斷；請先確認 " + lock.name) from error
    os.close(descriptor)
    originals = {path: path.exists() for path in (aar, manifest)}
    backups = {path: path.with_name(path.name + ".bak") for path in originals}
    replaced = []
    try:
        # Preflight all paths before making even the first backup.
        for backup in backups.values():
            if backup.exists():
                raise ValueError("已有備份 " + backup.name + "；請先確認上次建置狀態。")
        for path, existed in originals.items():
            if existed:
                shutil.copy2(path, backups[path])
        try:
            for source, destination in ((candidate, aar), (staged_manifest, manifest)):
                os.replace(source, destination)
                replaced.append(destination)
            validator()
        except BaseException:
            for destination in reversed(replaced):
                if originals[destination]:
                    # Restore via an adjacent temporary file so readers never
                    # observe a half-copied archive, and preserve recovery .bak.
                    descriptor, name = tempfile.mkstemp(prefix=".restore-", dir=destination.parent)
                    os.close(descriptor)
                    restore = Path(name)
                    try:
                        shutil.copy2(backups[destination], restore)
                        os.replace(restore, destination)
                    finally:
                        restore.unlink(missing_ok=True)
                else:
                    destination.unlink(missing_ok=True)
            raise
        for path, existed in originals.items():
            if existed:
                try:
                    backups[path].unlink()
                except OSError as error:
                    # Publication is already validated; a cleanup failure must
                    # not report that the new AAR failed or remove valid output.
                    print("AAR 驗證通過，但備份未能移除：" + backups[path].name + "（" + str(error) + "）")
    finally:
        try:
            lock.unlink()
        except OSError as error:
            print("發布鎖未能移除，重建前請確認：" + lock.name + "（" + str(error) + "）")


def build(force=False):
    if not force:
        try:
            verify(AAR, MANIFEST, CORE)
            print("Android core 未變更，保留既有 AAR。")
            return
        except (ValueError, OSError, json.JSONDecodeError, zipfile.BadZipFile):
            pass
    go = os.environ.get("YOURDESK_GO") or shutil.which("go")
    sdk = os.environ.get("ANDROID_HOME") or os.environ.get("ANDROID_SDK_ROOT")
    if not go or not sdk or not (Path(sdk) / "platforms").is_dir():
        raise ValueError("需要 Go、JDK 及 Android SDK/NDK；請設定 PATH（或 YOURDESK_GO）、JAVA_HOME、ANDROID_HOME。未更動既有 AAR。")
    go = str(Path(shutil.which(go) or go).resolve())
    sdk = str(Path(sdk).resolve())
    env = os.environ.copy()
    # Do not accidentally use a parent go.work or caller's module overrides.
    env["ANDROID_HOME"] = sdk
    for variable in ("JAVA_HOME", "ANDROID_NDK_HOME"):
        if env.get(variable):
            env[variable] = str(Path(env[variable]).resolve())
    env = private_build_environment(env)
    env["PATH"] = str(Path(go).resolve().parent) + os.pathsep + env.get("PATH", "")
    if env.get("JAVA_HOME"):
        env["PATH"] = str(Path(env["JAVA_HOME"]) / "bin") + os.pathsep + env["PATH"]
    if not shutil.which("javac", path=env["PATH"]):
        raise ValueError("找不到 javac；請設定有效的 JAVA_HOME。")
    subprocess.run(["javac", "-version"], env=env, check=True)
    original_digest = source_digest()
    AAR.parent.mkdir(parents=True, exist_ok=True)
    # Same-filesystem staging permits atomic replacement only after all checks pass.
    with tempfile.TemporaryDirectory(prefix=".core-build-", dir=AAR.parent) as directory:
        stage = Path(directory)
        extension = ".exe" if os.name == "nt" else ""
        for tool in ("gomobile", "gobind"):
            subprocess.run([go, "build", "-mod=readonly", "-o", str(stage / (tool + extension)),
                            "golang.org/x/mobile/cmd/" + tool], cwd=CORE, env=env, check=True)
        env["PATH"] = str(stage) + os.pathsep + env["PATH"]
        candidate = stage / "androidcore.aar"
        subprocess.run([str(stage / ("gomobile" + extension)), "bind", "-target=android/arm64", "-androidapi=26",
                        "-javapkg=com.yourdesk.androidcore", "-trimpath",
                        "-ldflags=-extldflags=-Wl,-z,max-page-size=16384,-z,common-page-size=16384",
                        "-o", str(candidate), "."], cwd=CORE, env=env, check=True)
        verify_native(candidate)
        verify_private_paths(candidate)
        if source_digest() != original_digest:
            raise ValueError("建置期間原始碼已改變，保留舊 AAR，請重試。")
        record = {"format": 1, "sourceSha256": original_digest, "aarSha256": digest(candidate),
                  "goVersion": subprocess.check_output([go, "version"], cwd=CORE, env=env, text=True).strip()}
        staged_manifest = stage / "androidcore.manifest.json"
        staged_manifest.write_text(json.dumps(record, indent=2) + "\n")
        publish(candidate, staged_manifest, AAR, MANIFEST, lambda: verify(AAR, MANIFEST, CORE))
    print("Android core AAR 建置、來源指紋、個人路徑與 16 KB ELF 驗證完成；未動 FFmpeg。")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("build", "verify", "verify-apk"))
    parser.add_argument("--force", action="store_true")
    parser.add_argument("--apk", type=Path)
    args = parser.parse_args()
    try:
        if args.action == "build":
            build(args.force)
        elif args.action == "verify":
            verify()
            print("Android core source/AAR/個人路徑/16 KB ELF 驗證通過。")
        else:
            if args.apk is None:
                parser.error("verify-apk 需要 --apk")
            verify_native(args.apk, apk=True)
            verify_private_paths(args.apk)
            print("APK 所有 64-bit native ELF／ZIP 對齊與建置路徑驗證通過；仍需 16 KB 裝置實測。")
    except (ValueError, OSError, subprocess.CalledProcessError, zipfile.BadZipFile) as error:
        parser.exit(1, str(error) + "\n")


if __name__ == "__main__":
    main()
