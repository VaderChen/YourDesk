import hashlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock
import zipfile
from contextlib import redirect_stdout

import android_release as release


class AndroidReleasePackageTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        android = self.root / 'android'
        android.mkdir()
        (android / 'RELEASE-README.md').write_text('繁體中文 / English / 日本語 / 한국어\n', encoding='utf-8')
        for name in ('LICENSE.md', 'LICENSE.en.md', 'LICENSE.ja.md', 'LICENSE.ko.md'):
            (self.root / name).write_text(name + '\n', encoding='utf-8')
        self.apk = self.root / 'YourDesk-1.26.1001-build-1524-android-arm64.apk'
        self.payload = b'APK bytes including original signature\0' * 100
        self.apk.write_bytes(self.payload)
        self.record = self.apk.with_suffix('.json')
        self.record.write_text(json.dumps({'versionName': '1.26.1001 build 1524', 'versionCode': 29847324,
            'applicationId': 'com.yourdesk.android', 'abi': 'arm64-v8a',
            'sha256': hashlib.sha256(self.payload).hexdigest(), 'signerSha256': 'signer'}))
        for patch in (mock.patch.object(release, 'ROOT', self.root),
                      mock.patch.object(release, 'ANDROID', android),
                      mock.patch.object(release, 'version', return_value=('1.26.1001 build 1524', 29847324)),
                      mock.patch.object(release, 'verify_apk', return_value='signer')):
            patch.start()
            self.addCleanup(patch.stop)

    def pack(self):
        with redirect_stdout(io.StringIO()):
            return release.pack(self.apk, self.root)

    def test_round_trip_preserves_apk_and_excludes_private_siblings(self):
        (self.root / 'signing-key.jks').write_bytes(b'private')
        (self.root / 'config.bak').write_bytes(b'backup')
        target = self.pack()
        release.verify_archive(target, target.with_suffix('.zip.json'))
        with zipfile.ZipFile(target) as archive:
            self.assertEqual(archive.read(self.apk.name), self.payload)
            self.assertEqual(archive.read('BUILD.json'), self.record.read_bytes())
            self.assertEqual(set(archive.namelist()), {self.apk.name, 'README.md', 'BUILD.json', 'SHA256SUMS',
                'LICENSE.md', 'LICENSE.en.md', 'LICENSE.ja.md', 'LICENSE.ko.md'})
            self.assertTrue(all(item.compress_type == zipfile.ZIP_DEFLATED for item in archive.infolist()))
        self.assertEqual(target.with_suffix('.zip.sha256').read_text(),
                         release.core.digest(target) + '  ' + target.name + '\n')

    def test_same_inputs_produce_identical_package(self):
        target = self.pack()
        before = target.read_bytes()
        self.assertEqual(self.pack().read_bytes(), before)

    def test_stale_build_record_cannot_replace_existing_release(self):
        target = self.pack()
        before = target.read_bytes()
        self.apk.write_bytes(self.payload + b'changed')
        with self.assertRaisesRegex(ValueError, 'APK 與建置紀錄不符'):
            self.pack()
        self.assertEqual(target.read_bytes(), before)

    def test_corrupted_archive_is_rejected(self):
        target = self.pack()
        target.write_bytes(target.read_bytes() + b'corruption')
        with self.assertRaisesRegex(ValueError, 'ZIP 與封裝紀錄不符'):
            release.verify_archive(target, target.with_suffix('.zip.json'))


if __name__ == '__main__':
    unittest.main()
