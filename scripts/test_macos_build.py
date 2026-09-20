"""最低 macOS 版本必須同時作用於編譯設定、原生快取與真正產物。"""
import os
from pathlib import Path
import plistlib
import shlex
import struct
import tempfile
import unittest
from unittest import mock

import macos_build
import release


def binary(minimum=13, platform=1, cpu=0x0100000C, legacy=False):
    if legacy:
        command = struct.pack('<4I', 0x24, 16, minimum << 16, 27 << 16)
    else:
        command = struct.pack('<6I', 0x32, 24, platform, minimum << 16, 27 << 16, 0)
    return struct.pack('<8I', 0xFEEDFACF, cpu, 0, 2, 1, len(command), 0, 0) + command


class MacOSBuildTests(unittest.TestCase):
    def test_environment_is_idempotent_and_preserves_optimization(self):
        original = {'CGO_LDFLAGS': "'-Lsome folder'"}
        result = macos_build.environment('darwin/arm64', original)
        self.assertEqual(original, {'CGO_LDFLAGS': "'-Lsome folder'"})
        self.assertEqual(result['MACOSX_DEPLOYMENT_TARGET'], '13.0')
        self.assertIn('-O2', shlex.split(result['CGO_CFLAGS']))
        self.assertIn('-O2', shlex.split(result['CGO_CXXFLAGS']))
        self.assertIn('-Lsome folder', shlex.split(result['CGO_LDFLAGS']))
        self.assertEqual(result, macos_build.environment('darwin/arm64', result))
        self.assertEqual(original, macos_build.environment('windows/amd64', original))

    def test_reject_conflicting_targets(self):
        for env in ({'MACOSX_DEPLOYMENT_TARGET': '27.0'},
                    {'MACOSX_DEPLOYMENT_TARGET': 'invalid'},
                    {'CGO_CFLAGS': '-O2 -mmacosx-version-min=12.0'},
                    {'CGO_LDFLAGS': '-mmacosx-version-min=27.0'}):
            with self.subTest(env=env), self.assertRaises(RuntimeError):
                macos_build.environment('darwin/arm64', env)

    def test_new_sdk_does_not_raise_minimum(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / 'binary'
            for legacy in (False, True):
                path.write_bytes(binary(13, legacy=legacy))
                macos_build.verify_binary(path)
                path.write_bytes(binary(27, legacy=legacy))
                with self.assertRaisesRegex(RuntimeError, '高於允許'):
                    macos_build.verify_binary(path)

    def test_native_and_cgo_flags_cannot_override_minimum(self):
        names = ('CFLAGS', 'CXXFLAGS', 'CPPFLAGS', 'LDFLAGS', 'CGO_CPPFLAGS', 'CGO_CFLAGS', 'CGO_CXXFLAGS', 'CGO_LDFLAGS')
        flags = ('-mmacosx-version-min=27.0', '-mmacos-version-min=12.0',
                 '-target arm64-apple-macos27.0', '--target=arm64-apple-macosx27.0',
                 '-target arm64-apple-darwin27', '-Wl,-platform_version,macos,27.0,27.0',
                 '-Wl,-macosx_version_min,27.0', '-Xlinker -platform_version -Xlinker macos -Xlinker 27.0 -Xlinker 27.0')
        for name in names:
            for flag in flags:
                with self.subTest(name=name, flag=flag), self.assertRaises(RuntimeError):
                    macos_build.environment('darwin/arm64', {name: flag})
            macos_build.environment('darwin/arm64', {name: '-target arm64-apple-macos13.0 -O2'})

    def test_release_signing_ignores_appledouble(self):
        with tempfile.TemporaryDirectory() as temp:
            folder = Path(temp)
            for name in macos_build.RUNTIME_LIBRARIES:
                (folder / name).write_bytes(binary())
                (folder / ('._' + name)).write_bytes(b'AppleDouble')
            with mock.patch.object(release, 'run') as run, \
                    mock.patch.object(release, 'environment', return_value={}), \
                    mock.patch.object(macos_build, 'verify_binary'):
                release.compile_program('desktop', folder, 'darwin/arm64', '1.26.0920 build 2130')
            signing = run.call_args.args[0]
            self.assertEqual(signing[1:], [folder / name for name in macos_build.RUNTIME_LIBRARIES] + [folder / 'YourDesk'])

    def test_reject_invalid_platform_architecture_and_commands(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / 'binary'
            for data in (b'not macho', binary(platform=2), binary(cpu=0x01000007),
                         binary()[:-1], binary()[:32], binary()[:32] + struct.pack('<2I', 0x32, 0) + bytes(16)):
                with self.subTest(data=data):
                    path.write_bytes(data)
                    with self.assertRaises(RuntimeError):
                        macos_build.verify_binary(path)

    def test_application_checks_runtime_and_optional_extension(self):
        with tempfile.TemporaryDirectory() as temp:
            app = Path(temp) / 'YourDesk.app'
            paths = [app / 'Contents/MacOS' / name for name in ('YourDesk', 'yourdesk-client', 'yourdesk-remote')]
            paths += [app / 'Contents/Frameworks' / name for name in macos_build.RUNTIME_LIBRARIES]
            for path in paths:
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(binary())
            (app / 'Contents/Info.plist').write_bytes(plistlib.dumps({'LSMinimumSystemVersion': '13.0'}))
            extension = app / 'Contents/Extensions/YourDeskSiri.appex/Contents/MacOS/YourDeskSiri'
            extension.parent.mkdir(parents=True)
            extension.write_bytes(binary(13))
            macos_build.verify_application(app)
            paths[-1].write_bytes(binary(27))
            with self.assertRaisesRegex(RuntimeError, '高於允許'):
                macos_build.verify_application(app)
            paths[-1].write_bytes(binary())
            extension.write_bytes(binary(27))
            with self.assertRaisesRegex(RuntimeError, '高於允許'):
                macos_build.verify_application(app)

    def test_release_environment_applies_target_after_user_flags(self):
        with mock.patch.dict(os.environ, {'YOURDESK_CGO_CFLAGS_DARWIN_ARM64': '-O3'}, clear=True), \
                mock.patch.object(release, 'capture', return_value='darwin'):
            env = release.environment('darwin/arm64', True)
            self.assertEqual(env['CGO_CFLAGS'], '-O3 -mmacosx-version-min=13.0')
            self.assertEqual(env['MACOSX_DEPLOYMENT_TARGET'], '13.0')


if __name__ == '__main__':
    unittest.main()
