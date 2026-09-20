import hashlib
import json
from pathlib import Path
import struct
import tempfile
import unittest
from unittest import mock
import zipfile

import android_core as core


def elf(alignment=16384, offset=0, address=0, relro_end=None):
    data = bytearray(64 + 56 * (2 if relro_end is not None else 1))
    data[:6] = b"\x7fELF\x02\x01"
    struct.pack_into("<HHI", data, 16, 3, 183, 1)
    struct.pack_into("<Q", data, 32, 64)
    struct.pack_into("<HH", data, 54, 56, 2 if relro_end is not None else 1)
    struct.pack_into("<IIQQQQQQ", data, 64, 1, 5, offset, address, 0, 0, 0, alignment)
    if relro_end is not None:
        struct.pack_into("<IIQQQQQQ", data, 120, 0x6474e552, 4, 0, 0, 0, 0, relro_end, 1)
    return bytes(data)


class AndroidArtifactTests(unittest.TestCase):
    def test_native_elf_alignment_and_relro(self):
        self.assertIsNone(core.elf_error(elf()))
        self.assertIsNone(core.elf_error(elf(relro_end=16384)))
        for data in (b"short", elf(4096), elf(offset=4096), elf(relro_end=4096), elf(24576)):
            self.assertIsNotNone(core.elf_error(data))
        malformed = bytearray(elf())
        struct.pack_into("<Q", malformed, 64 + 32, len(malformed) + 1)
        self.assertIsNotNone(core.elf_error(malformed))
        self.assertIsNotNone(core.elf_error(elf(), machine=62))

    def test_source_and_binary_freshness(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            source = root / "core"
            source.mkdir()
            (source / "go.mod").write_text("module example\n")
            (source / "api.go").write_text("package core\n")
            aar = root / "androidcore.aar"
            manifest = root / "androidcore.manifest.json"
            with zipfile.ZipFile(aar, "w") as archive:
                archive.writestr("jni/arm64-v8a/libgojni.so", elf())
            record = {"format": 1, "sourceSha256": core.source_digest(source), "aarSha256": core.digest(aar)}
            manifest.write_text(json.dumps(record))
            core.verify(aar, manifest, source)
            (source / "api.go.bak").write_text("old")
            (source / "._api.go").write_text("exFAT metadata")
            (source / "api_test.go").write_text("package core\n")
            core.verify(aar, manifest, source)
            (source / "api.go").write_text("package core\n// changed\n")
            with self.assertRaisesRegex(ValueError, "過期"):
                core.verify(aar, manifest, source)
            (source / "api.go").write_text("package core\n")
            with aar.open("ab") as stream:
                stream.write(b"changed")
            with self.assertRaisesRegex(ValueError, "過期"):
                core.verify(aar, manifest, source)

    def test_archive_checks_all_libraries(self):
        with tempfile.TemporaryDirectory() as folder:
            aar = Path(folder) / "androidcore.aar"
            with zipfile.ZipFile(aar, "w") as archive:
                archive.writestr("jni/arm64-v8a/libgojni.so", elf())
                archive.writestr("jni/arm64-v8a/libother.so", elf(4096))
            with self.assertRaisesRegex(ValueError, "libother"):
                core.verify_native(aar)

    def test_apk_zip_alignment(self):
        with tempfile.TemporaryDirectory() as folder:
            apk = Path(folder) / "app.apk"
            with zipfile.ZipFile(apk, "w") as archive:
                archive.writestr("lib/arm64-v8a/libgojni.so", elf())
            with self.assertRaisesRegex(ValueError, "APK ZIP"):
                core.verify_native(apk, apk=True)

    def test_apk_correct_local_extra_padding(self):
        with tempfile.TemporaryDirectory() as folder:
            apk = Path(folder) / "app.apk"
            name = "lib/arm64-v8a/libgojni.so"
            info = zipfile.ZipInfo(name)
            padding = (-30 - len(name.encode())) % core.PAGE_SIZE
            info.extra = struct.pack("<HH", 0xD935, padding - 4) + bytes(padding - 4)
            with zipfile.ZipFile(apk, "w") as archive:
                archive.writestr(info, elf())
            core.verify_native(apk, apk=True)

    def test_fingerprint_covers_native_and_embedded_resources(self):
        with tempfile.TemporaryDirectory() as folder:
            source = Path(folder)
            (source / "api.go").write_text("package core\n")
            previous = core.source_digest(source)
            for name in ("native.c", "native.h", "asm_arm64.s", "assets/config.json"):
                path = source / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("content")
                current = core.source_digest(source)
                self.assertNotEqual(previous, current, name)
                previous = current

    def test_publish_rolls_back_manifest_replace_failure(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            aar, manifest = root / "core.aar", root / "core.json"
            candidate, staged = root / "new.aar", root / "new.json"
            for path, value in ((aar, "old-aar"), (manifest, "old-manifest"),
                                (candidate, "new-aar"), (staged, "new-manifest")):
                path.write_text(value)
            replace = core.os.replace
            def fail_manifest(source, destination):
                if Path(source) == staged:
                    raise OSError("injected replacement failure")
                replace(source, destination)
            with mock.patch.object(core.os, "replace", side_effect=fail_manifest):
                with self.assertRaisesRegex(OSError, "injected"):
                    core.publish(candidate, staged, aar, manifest, lambda: None)
            self.assertEqual(aar.read_text(), "old-aar")
            self.assertEqual(manifest.read_text(), "old-manifest")
            self.assertEqual(aar.with_name("core.aar.bak").read_text(), "old-aar")
            self.assertEqual(manifest.with_name("core.json.bak").read_text(), "old-manifest")
            self.assertFalse((root / ".core.aar.lock").exists())

    def test_publish_rolls_back_validation_failure(self):
        for existing in (True, False):
            with self.subTest(existing=existing), tempfile.TemporaryDirectory() as folder:
                root = Path(folder)
                aar, manifest = root / "core.aar", root / "core.json"
                candidate, staged = root / "new.aar", root / "new.json"
                if existing:
                    aar.write_text("old-aar")
                    manifest.write_text("old-manifest")
                candidate.write_text("new-aar")
                staged.write_text("new-manifest")
                def reject():
                    raise ValueError("validation failed")
                with self.assertRaisesRegex(ValueError, "validation"):
                    core.publish(candidate, staged, aar, manifest, reject)
                if existing:
                    self.assertEqual(aar.read_text(), "old-aar")
                    self.assertEqual(manifest.read_text(), "old-manifest")
                else:
                    self.assertFalse(aar.exists())
                    self.assertFalse(manifest.exists())

    def test_publish_preexisting_backups_and_lock_are_preserved(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            aar, manifest = root / "core.aar", root / "core.json"
            candidate, staged = root / "new.aar", root / "new.json"
            aar.write_text("old-aar")
            backup = root / "core.json.bak"
            backup.write_text("previous recovery")
            with self.assertRaisesRegex(ValueError, "已有備份"):
                core.publish(candidate, staged, aar, manifest, lambda: None)
            self.assertEqual(aar.read_text(), "old-aar")
            self.assertEqual(backup.read_text(), "previous recovery")
            self.assertFalse((root / "core.aar.bak").exists())
            lock = root / ".core.aar.lock"
            lock.write_text("active")
            with self.assertRaisesRegex(ValueError, "發布"):
                core.publish(candidate, staged, aar, manifest, lambda: None)
            self.assertEqual(lock.read_text(), "active")

    def test_successful_publish_removes_own_backups(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            aar, manifest = root / "core.aar", root / "core.json"
            candidate, staged = root / "new.aar", root / "new.json"
            aar.write_text("old-aar")
            manifest.write_text("old-manifest")
            candidate.write_text("new-aar")
            staged.write_text("new-manifest")
            def validate():
                self.assertEqual(aar.read_text(), "new-aar")
                self.assertEqual(manifest.read_text(), "new-manifest")
            core.publish(candidate, staged, aar, manifest, validate)
            self.assertEqual(aar.read_text(), "new-aar")
            self.assertFalse((root / "core.aar.bak").exists())
            self.assertFalse((root / "core.json.bak").exists())
            self.assertFalse((root / ".core.aar.lock").exists())


if __name__ == "__main__":
    unittest.main()
