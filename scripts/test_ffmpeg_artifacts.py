"""已建置產物保留與中繼檔清理的離線回歸測試。"""
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import ffmpeg_artifacts as artifacts


class ArtifactTests(unittest.TestCase):
    def fixture(self, root):
        build = root / 'darwin-arm64-0123456789ab'
        for name in ('install/lib', 'metadata', 'aom', 'ffmpeg'):
            (build / name).mkdir(parents=True)
        (build / 'install/lib/library.dylib').write_bytes(b'keep binary')
        (build / 'metadata/config.h').write_text('keep configuration')
        (build / 'aom/object.o').write_bytes(b'discard intermediate')
        (build / 'ffmpeg/config.log').write_text('discard build log')
        (build / 'complete').write_text('complete')
        (root / 'source.tar.xz').write_bytes(b'keep source archive')
        inputs, sources = {'recipe': 'one'}, {'source': 'source hash'}
        artifacts.save(build, inputs, sources)
        return build, inputs, sources

    def test_cleanup_preserves_install_manifest_and_sources(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            build, inputs, sources = self.fixture(root)
            binary = build / 'install/lib/library.dylib'
            before = (binary.read_bytes(), binary.stat().st_mtime_ns)
            artifacts.clean_intermediates(root, build)
            artifacts.clean_intermediates(root, build)
            self.assertFalse((build / 'aom').exists())
            self.assertFalse((build / 'ffmpeg').exists())
            self.assertEqual((binary.read_bytes(), binary.stat().st_mtime_ns), before)
            self.assertTrue((root / 'source.tar.xz').is_file())
            self.assertTrue(artifacts.current(build, inputs, sources))

    def test_changed_recipe_sources_or_missing_artifact_requires_build(self):
        with tempfile.TemporaryDirectory() as temp:
            build, inputs, sources = self.fixture(Path(temp))
            self.assertFalse(artifacts.current(build, {'recipe': 'two'}, sources))
            self.assertFalse(artifacts.current(build, inputs, {'source': 'changed hash'}))
            binary = build / 'install/lib/library.dylib'
            binary.write_bytes(b'changed binary')
            self.assertFalse(artifacts.current(build, inputs, sources))
            binary.unlink()
            self.assertFalse(artifacts.current(build, inputs, sources))

    def test_unpacked_sources_may_be_absent_without_rebuilding_binary(self):
        with tempfile.TemporaryDirectory() as temp:
            build, inputs, _ = self.fixture(Path(temp))
            self.assertTrue(artifacts.current(build, inputs, {}))

    def test_old_helper_fingerprint_does_not_force_rebuild(self):
        with tempfile.TemporaryDirectory() as temp:
            build, _, sources = self.fixture(Path(temp))
            artifacts.save(build, {'recipe': ['native recipe', 'old cleanup helper']}, sources)
            self.assertTrue(artifacts.current(build, {'recipe': ['native recipe']}, sources))
            self.assertFalse(artifacts.current(build, {'recipe': ['changed native recipe']}, sources))

    def test_cleanup_ignores_only_disappearing_appledouble_files(self):
        for name, error, ignored in (
                ('._object.o', FileNotFoundError('already removed'), True),
                ('object.o', FileNotFoundError('unexpected missing file'), False),
                ('._object.o', PermissionError('denied'), False),
                ('._object.o', OSError('I/O error'), False)):
            with self.subTest(name=name, error=type(error).__name__), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                build, _, _ = self.fixture(root)

                def remove(path, onerror):
                    onerror(None, str(path / name), (type(error), error, None))

                with mock.patch.object(artifacts.shutil, 'rmtree', side_effect=remove):
                    if ignored:
                        artifacts.clean_intermediates(root, build)
                    else:
                        with self.assertRaises(type(error)):
                            artifacts.clean_intermediates(root, build)

    def test_invalid_manifest_is_not_a_cache_hit(self):
        with tempfile.TemporaryDirectory() as temp:
            build, inputs, sources = self.fixture(Path(temp))
            manifest = build / 'metadata/manifest.json'
            manifest.write_text('broken')
            self.assertFalse(artifacts.current(build, inputs, sources))
            manifest.write_text(json.dumps({'inputs': inputs, 'sources': sources, 'files': {'../../outside': 'hash'}}))
            self.assertFalse(artifacts.current(build, inputs, sources))

    def test_cleanup_rejects_wrong_directory_and_symlink(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            build, _, _ = self.fixture(root)
            with self.assertRaises(RuntimeError):
                artifacts.clean_intermediates(root, root)
            outside = root / 'outside'
            outside.mkdir()
            (outside / 'keep').write_text('keep')
            (build / 'ffmpeg/config.log').unlink()
            (build / 'ffmpeg').rmdir()
            (build / 'ffmpeg').symlink_to(outside, target_is_directory=True)
            with self.assertRaises(RuntimeError):
                artifacts.clean_intermediates(root, build)
            self.assertEqual((outside / 'keep').read_text(), 'keep')
            self.assertTrue((build / 'aom/object.o').is_file())

    def test_source_content_changes_are_detected_but_timestamp_alone_is_not(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source = root / 'source'
            source.mkdir()
            path = source / 'file.c'
            path.write_text('one')
            definitions = [('source', 'tar.xz', 'url', 'hash')]
            before = artifacts.source_state(root, definitions)
            path.touch()
            self.assertEqual(before, artifacts.source_state(root, definitions))
            path.write_text('two')
            self.assertNotEqual(before, artifacts.source_state(root, definitions))


if __name__ == '__main__':
    unittest.main()
