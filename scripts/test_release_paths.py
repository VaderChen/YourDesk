"""Filesystem metadata filters; no real build, signing or release packaging."""
from contextlib import ExitStack
import json
from pathlib import Path
import shlex
import shutil
import tempfile
import unittest
from unittest import mock

import release


class ReleasePathTests(unittest.TestCase):
    VERSION = '1.26.0921 build 0000'

    def setUp(self):
        self.guards = ExitStack()
        self.addCleanup(self.guards.close)
        for name in ('run', 'capture', 'build', 'reset_dist', 'signing_identity',
                     'notarize_app', 'notarize', 'staple'):
            self.guards.enter_context(mock.patch.object(
                release, name, side_effect=AssertionError('Unexpected release operation: ' + name)))

    def write(self, folder, name, data=b'fixture payload'):
        path = folder / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
        return path

    def add_metadata(self, folder):
        for name in ('.DS_Store', '._root', 'nested/.DS_Store', 'nested/._license',
                     '._metadata/ordinary.txt', 'nested/.DS_Store-dir/._child',
                     'directory/.DS_Store/ordinary.txt'):
            self.write(folder, name, b'filesystem metadata fixture')

    def assert_clean_names(self, names):
        for name in names:
            self.assertFalse(release.is_filesystem_metadata(name), name)

    def fake_zip(self, entries, source=None):
        def create(filename, *_args, **_kwargs):
            archive = Path(filename)
            # Packaging methods may move this placeholder, never a real ZIP.
            archive.write_bytes(b'zip writer fixture')
            folder = source or next(path for path in archive.parent.iterdir() if path.is_dir())
            # Simulate metadata generated after copytree/manifest, so the ZIP
            # writer's own filter is tested independently of those filters.
            self.add_metadata(folder)
            output = mock.MagicMock()
            output.__enter__.return_value = output
            output.write.side_effect = lambda item, name: entries.__setitem__(
                str(name), Path(item).read_bytes())
            return output
        return create

    def test_metadata_names_are_rejected_at_any_depth(self):
        for name in ('._item', '.DS_Store', 'a/._dir/child', 'a/.DS_Store/child'):
            self.assertTrue(release.is_filesystem_metadata(name), name)
        for name in ('LICENSE', '.license', 'a/not._metadata', 'a/.DS_Store-dir/child'):
            self.assertFalse(release.is_filesystem_metadata(name), name)

    def test_nsis_recursive_license_exclusions_static(self):
        # Static option regression only; this does not execute makensis or
        # prove the behavior of an actual compiled installer.
        source = Path(__file__).with_name('windows-installer.nsi').read_text()
        commands = [shlex.split(line) for line in source.splitlines()
                    if line.lstrip().startswith('File ') and 'ThirdPartyLicenses' in line]
        self.assertEqual(commands, [[
            'File', '/r', '/x', '._*', '/x', '.DS_Store', '${PAYLOAD_DIR}/ThirdPartyLicenses']])

    def test_manifest_omits_metadata_and_preserves_regular_hidden_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            self.write(folder, 'program')
            self.write(folder, 'nested/LICENSE')
            self.write(folder, '.license')
            self.add_metadata(folder)
            release.manifest(folder)
            names = {line.split('  ', 1)[1] for line in (folder / 'SHA256SUMS').read_text().splitlines()}
            self.assertEqual(names, {'program', 'nested/LICENSE', '.license'})
            self.assert_clean_names(names)

    def test_copytree_excludes_metadata_directories_and_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source, destination = root / 'source', root / 'destination'
            self.write(source, 'nested/LICENSE')
            self.write(source, '.license')
            self.add_metadata(source)
            shutil.copytree(source, destination, ignore=release.ignore_filesystem_metadata)
            names = {str(path.relative_to(destination)) for path in destination.rglob('*') if path.is_file()}
            self.assertEqual(names, {'nested/LICENSE', '.license'})

    def test_project_license_glob_does_not_copy_appledouble(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            project, output = root / 'project', root / 'output'
            output.mkdir()
            for name in release.PROJECT_LICENSE_FILES:
                self.write(project, name)
            self.write(project, 'docs/third-party/example-LICENSE.txt')
            self.write(project, 'docs/third-party/._example-LICENSE.txt', b'metadata')
            with mock.patch.object(release, 'ROOT', project):
                release.copy_project_licenses(output)
            self.assertEqual({path.name for path in (output / 'ThirdPartyLicenses').iterdir()},
                             {'example-LICENSE.txt'})

    def test_winpe_zip_writer_omits_metadata_added_after_manifest(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary) / 'winpe'
            for name in ('yourdesk-winpe.exe', 'start-yourdesk.cmd', 'README.md', *release.PROJECT_LICENSE_FILES):
                self.write(folder, name)
            self.write(folder, 'ThirdPartyLicenses/nested/LICENSE')
            self.add_metadata(folder / 'ThirdPartyLicenses')
            entries = {}
            with mock.patch.object(release.zipfile, 'ZipFile', side_effect=self.fake_zip(entries)):
                archive = release.winpe_zip(folder, 'fixture-winpe')
            self.assertTrue(archive.is_file())
            self.assertIn('fixture-winpe/yourdesk-winpe.exe', entries)
            self.assertIn('fixture-winpe/ThirdPartyLicenses/nested/LICENSE', entries)
            self.assertIn('fixture-winpe/SHA256SUMS', entries)
            self.assert_clean_names(entries)

    def test_windows_zip_writer_omits_metadata_added_after_manifest(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary) / 'windows'
            for name in ('YourDesk.exe', 'yourdesk-client.exe', 'yourdesk-remote.exe',
                         *release.windows_runtime.FFMPEG_DLLS, *release.PROJECT_LICENSE_FILES,
                         *(f'release_{language}.note' for language in release.RELEASE_NOTE_LANGUAGES)):
                self.write(folder, name)
            self.write(folder, 'ThirdPartyLicenses/nested/LICENSE')
            self.add_metadata(folder / 'ThirdPartyLicenses')
            entries = {}
            with mock.patch.object(release.windows_runtime, 'validate'), \
                 mock.patch.object(release, 'write_instructions',
                                   side_effect=lambda output, *_: self.write(output, 'README.txt')), \
                 mock.patch.object(release.zipfile, 'ZipFile', side_effect=self.fake_zip(entries)):
                archive = release.windows_portable_zip(folder, 'fixture-windows', self.VERSION)
            self.assertTrue(archive.is_file())
            self.assertIn('YourDesk/YourDesk.exe', entries)
            self.assertIn('YourDesk/ThirdPartyLicenses/nested/LICENSE', entries)
            self.assert_clean_names(entries)

    def test_linux_zip_writer_omits_nested_metadata(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            folder = root / 'linux-arm64'
            self.write(folder, 'yourdesk-client')
            self.write(folder, 'ThirdPartyLicenses/nested/LICENSE')
            self.add_metadata(folder)
            self.write(root, 'release.json', json.dumps({
                'version': self.VERSION, 'targets': ['linux/arm64']}).encode())
            for language in release.RELEASE_NOTE_LANGUAGES:
                self.write(root, f'release_{language}.note')
            entries = {}
            with mock.patch.object(release, 'copy_release_notes'), \
                 mock.patch.object(release, 'write_instructions',
                                   side_effect=lambda output, *_: self.write(output, 'README.txt')), \
                 mock.patch.object(release.zipfile, 'ZipFile', side_effect=self.fake_zip(entries, folder)):
                release.pack(root)
            self.assertIn('linux-arm64/yourdesk-client', entries)
            self.assertIn('linux-arm64/ThirdPartyLicenses/nested/LICENSE', entries)
            self.assert_clean_names(entries)

    def test_mac_bundle_license_resources_omit_metadata(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            for name in ('YourDesk', 'yourdesk-client', 'yourdesk-remote', *release.macos_build.RUNTIME_LIBRARIES):
                self.write(folder, name)
            icon = self.write(folder, 'fixture.icns')
            for name in ('libjpeg-turbo', 'FFmpeg'):
                self.write(folder, f'ThirdPartyLicenses/{name}/nested/LICENSE')
                self.add_metadata(folder / 'ThirdPartyLicenses' / name)
            with mock.patch.dict(release.os.environ, {'YOURDESK_MAC_ICON_PATH': str(icon)}), \
                 mock.patch.object(release, 'copy_project_licenses'), \
                 mock.patch.object(release, 'copy_model_licenses'), \
                 mock.patch.object(release, 'signing_identity', return_value='-'), \
                 mock.patch.object(release, 'run'), \
                 mock.patch.object(release, 'notarize_app'), \
                 mock.patch.object(release.macos_build, 'verify_application'), \
                 mock.patch('siri.build'):
                release.mac_bundle(folder, self.VERSION)
            resources = folder / 'YourDesk.app/Contents/Resources/ThirdPartyLicenses'
            names = {str(path.relative_to(resources)) for path in resources.rglob('*') if path.is_file()}
            self.assertEqual(names, {'libjpeg-turbo/nested/LICENSE', 'FFmpeg/nested/LICENSE'})

    def test_mac_dmg_staging_copy_omits_metadata_before_signing(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            folder = root / 'macos-arm64'
            app = folder / 'YourDesk.app'
            self.write(app, 'Contents/Resources/nested/LICENSE')
            self.add_metadata(app)
            self.write(root, 'release.json', json.dumps({
                'version': self.VERSION, 'targets': ['darwin/arm64']}).encode())
            for language in release.RELEASE_NOTE_LANGUAGES:
                self.write(root, f'release_{language}.note')
            checked = []

            def inspect_staged_app(staged):
                names = {str(path.relative_to(staged)) for path in staged.rglob('*') if path.is_file()}
                self.assert_clean_names(names)
                self.assertIn('Contents/Resources/nested/LICENSE', names)
                checked.append(staged)

            with mock.patch.object(release, 'copy_release_notes'), \
                 mock.patch.object(release, 'write_instructions',
                                   side_effect=lambda output, *_: self.write(output, 'README.txt')), \
                 mock.patch.object(release, 'copy_project_licenses'), \
                 mock.patch.object(release.macos_build, 'verify_application'), \
                 mock.patch.object(release, 'signing_identity', return_value='-'), \
                 mock.patch.object(release, 'run'), \
                 mock.patch.object(release, 'notarize_app', side_effect=inspect_staged_app), \
                 mock.patch.object(release, 'notarize'), \
                 mock.patch.object(release, 'staple'):
                release.pack(root)
            self.assertEqual(len(checked), 1)


if __name__ == '__main__':
    unittest.main()
