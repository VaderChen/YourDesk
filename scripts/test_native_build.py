"""原生影音建置：標準組態、相對建置指令與 CGo 參數回歸測試。"""
import hashlib
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile
import unittest
from unittest import mock

import ffmpeg
import turbojpeg


class NativeBuildTests(unittest.TestCase):
    def prepare_fixture(self, root, configure_error=False):
        cache = root / 'cache'
        source, aom = cache / 'ffmpeg-source', cache / 'aom-source'
        source.mkdir(parents=True)
        cmake = aom / 'build/cmake'
        cmake.mkdir(parents=True)
        (cmake / 'aom_optimization.cmake').write_text('')
        (cmake / 'aom_configure.cmake').write_text('')
        for name in ('COPYING.LGPLv2.1', 'LICENSE.md'):
            (source / name).write_text('license fixture')
        for name in ('LICENSE', 'PATENTS'):
            (aom / name).write_text('license fixture')

        def run(args, **kwargs):
            if '--install' in args:
                pc = kwargs['cwd'] / 'install/lib/pkgconfig/aom.pc'
                pc.parent.mkdir(parents=True, exist_ok=True)
                pc.write_text('prefix=/build/install\nName: aom\nVersion: 3.13.1\n')
            if args[0] == '../../ffmpeg-source/configure':
                if configure_error:
                    raise subprocess.CalledProcessError(1, args)
                work = kwargs['cwd']
                (work / 'config.h').write_text('#define CONFIG_GPL 0\n#define CONFIG_NONFREE 0\n')
                (work / 'config_components.h').write_text(
                    '#define CONFIG_LIBAOM_AV1_ENCODER 1\n#define CONFIG_AV1_VIDEOTOOLBOX_HWACCEL 1\n')
                prefix = work.parent / 'install'
                for name in ('libavcodec/avcodec.h', 'libavutil/avutil.h', 'libswscale/swscale.h'):
                    path = prefix / 'include' / name
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_text('header fixture')
                (prefix / 'lib').mkdir(exist_ok=True)
                for name in ('libavcodec.62', 'libavutil.60', 'libswscale.9', 'libavcodec', 'libavutil', 'libswscale'):
                    (prefix / 'lib' / (name + '.dylib')).write_bytes(b'library fixture')
            return subprocess.CompletedProcess(args, 0)

        tools = {name: '/tools/' + name for name in ('cmake', 'make', 'pkg-config')}
        with mock.patch.object(ffmpeg, 'CACHE', cache), \
                mock.patch.object(ffmpeg, 'sources', return_value=(source, aom)), \
                mock.patch.object(ffmpeg, 'required_build_tools', return_value=tools), \
                mock.patch.object(ffmpeg.macos_build, 'verify_binary'), \
                mock.patch.object(ffmpeg.subprocess, 'run', side_effect=run) as commands:
            result = ffmpeg.prepare('darwin/arm64', {})
        return commands.call_args_list, result

    def test_standard_configure_uses_relative_directories_without_overrides(self):
        with tempfile.TemporaryDirectory() as temp:
            commands, (_, prefix) = self.prepare_fixture(Path(temp))
            cmake = commands[0].args[0]
            self.assertEqual(cmake[cmake.index('-S') + 1], '../aom-source')
            self.assertEqual(cmake[cmake.index('-B') + 1], 'aom')
            self.assertIn('-DCMAKE_INSTALL_PREFIX=install', cmake)
            self.assertIn('-DCMAKE_OSX_DEPLOYMENT_TARGET=13.0', cmake)
            self.assertEqual(commands[1].args[0][1:3], ['--build', 'aom'])
            self.assertEqual(commands[2].args[0][1:], ['--install', 'aom', '--prefix', 'install'])
            configure = commands[3]
            self.assertEqual(configure.args[0][0], '../../ffmpeg-source/configure')
            self.assertIn('--prefix=../install', configure.args[0])
            self.assertIn('--pkg-config-flags=--static', configure.args[0])
            self.assertIn('--extra-cflags=-mmacosx-version-min=13.0', configure.args[0])
            self.assertIn('--extra-ldflags=-mmacosx-version-min=13.0', configure.args[0])
            self.assertFalse(any('--define-variable' in arg for arg in configure.args[0]))
            self.assertEqual(configure.kwargs['env']['PKG_CONFIG_LIBDIR'], '../install/lib/pkgconfig')
            self.assertEqual(configure.kwargs['env']['PKG_CONFIG_PATH'], '')
            self.assertFalse((prefix.parent / 'ffmpeg/src').is_symlink())
            self.assertTrue((prefix.parent / 'complete').exists())
            self.assertTrue((prefix.parent / 'metadata/manifest.json').is_file())
            self.assertFalse((prefix.parent / 'ffmpeg').exists())
            self.assertFalse((prefix.parent / 'aom').exists())
            self.assertIn('prefix=${pcfiledir}/../..', (prefix / 'lib/pkgconfig/aom.pc').read_text())
            for command in commands[3:]:
                self.assertEqual(command.kwargs['cwd'], prefix.parent / 'ffmpeg')

    def test_ffmpeg_cgo_quotes_whole_arguments(self):
        # CGo 參數引用是一般正確性要求，與 FFmpeg 的來源路徑限制分開驗證。
        with tempfile.TemporaryDirectory(prefix='cgo flags-') as temp:
            _, (env, prefix) = self.prepare_fixture(Path(temp))
            self.assertTrue(env['CGO_CFLAGS'].endswith("'-I" + str(prefix / 'include') + "'"))
            self.assertTrue(env['CGO_LDFLAGS'].endswith("'-L" + str(prefix / 'lib') + "'"))
            self.assertIn('-mmacosx-version-min=13.0', shlex.split(env['CGO_CFLAGS']))

    def test_second_build_reuses_binaries_after_intermediates_are_removed(self):
        with tempfile.TemporaryDirectory() as temp:
            _, (_, prefix) = self.prepare_fixture(Path(temp))
            library = prefix / 'lib/libavcodec.62.dylib'
            before = (library.read_bytes(), library.stat().st_mtime_ns)
            with mock.patch.object(ffmpeg, 'CACHE', Path(temp) / 'cache'), \
                    mock.patch.object(ffmpeg, 'sources') as sources, \
                    mock.patch.object(ffmpeg, 'required_build_tools') as tools, \
                    mock.patch.object(ffmpeg.macos_build, 'verify_binary'), \
                    mock.patch.object(ffmpeg.subprocess, 'run') as commands:
                _, reused = ffmpeg.prepare('darwin/arm64', {})
            self.assertEqual(reused, prefix)
            self.assertEqual((library.read_bytes(), library.stat().st_mtime_ns), before)
            sources.assert_not_called()
            tools.assert_not_called()
            commands.assert_not_called()

    def test_configure_failure_points_to_log_and_leaves_cache_incomplete(self):
        with tempfile.TemporaryDirectory() as temp:
            with self.assertRaisesRegex(RuntimeError, 'configure 失敗.*ffbuild/config.log'):
                self.prepare_fixture(Path(temp), configure_error=True)
            self.assertEqual(list(Path(temp).rglob('complete')), [])

    def test_pkg_config_relocation_is_idempotent(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / 'aom.pc'
            path.write_text('prefix=/old/path\nincludedir=${prefix}/include\n')
            ffmpeg.relocatable_pkg_config(path)
            before = (path.read_text(), path.stat().st_mtime_ns)
            ffmpeg.relocatable_pkg_config(path)
            self.assertEqual((path.read_text(), path.stat().st_mtime_ns), before)
            path.write_text('invalid pkg-config file')
            with self.assertRaisesRegex(RuntimeError, 'prefix'):
                ffmpeg.relocatable_pkg_config(path)

    @unittest.skipUnless(shutil.which('pkg-config') and shutil.which('cc'), 'requires pkg-config and cc')
    def test_pkg_config_under_unicode_path_with_ffmpeg_c_locale(self):
        with tempfile.TemporaryDirectory(prefix='程式-') as temp:
            build = Path(temp)
            work = build / 'ffmpeg'
            work.mkdir()
            pc = build / 'install/lib/pkgconfig/aom.pc'
            pc.parent.mkdir(parents=True)
            header = build / 'install/include/aom/example.h'
            header.parent.mkdir(parents=True)
            header.write_text('#define EXAMPLE 1\n')
            pc.write_text(f'prefix={build / "install"}\nName: aom\nDescription: fixture\nVersion: 3.13.1\n'
                          'Cflags: -I${prefix}/include\nLibs: -L${prefix}/lib -laom\n')
            ffmpeg.relocatable_pkg_config(pc)
            env = dict(os.environ, LC_ALL='C', PKG_CONFIG_LIBDIR='../install/lib/pkgconfig', PKG_CONFIG_PATH='')
            flags = subprocess.run([shutil.which('pkg-config'), '--cflags', 'aom'], cwd=work,
                                   env=env, capture_output=True, text=True, check=True).stdout
            self.assertNotIn(str(build), flags)
            self.assertNotIn('\\', flags)
            subprocess.run([shutil.which('cc'), *shlex.split(flags), '-x', 'c', '-fsyntax-only', '-'],
                           cwd=work, env=env, input='#include <aom/example.h>\nint test = EXAMPLE;\n',
                           text=True, capture_output=True, check=True)

    def test_turbojpeg_build_paths_and_whole_argument_cgo_quotes(self):
        with tempfile.TemporaryDirectory(prefix='cgo flags-') as temp:
            cache = Path(temp)
            data = b'verified cached archive fixture'
            (cache / f'libjpeg-turbo-{turbojpeg.VERSION}.tar.gz').write_bytes(data)
            source = cache / f'libjpeg-turbo-{turbojpeg.VERSION}'
            source.mkdir()

            def run(args, **kwargs):
                if '--build' in args:
                    (kwargs['cwd'] / 'libturbojpeg.a').write_bytes(b'library fixture')
                return subprocess.CompletedProcess(args, 0)

            with mock.patch.object(turbojpeg, 'CACHE', cache), \
                    mock.patch.object(turbojpeg, 'SHA256', hashlib.sha256(data).hexdigest()), \
                    mock.patch.object(turbojpeg.shutil, 'which', return_value='/tools/cmake'), \
                    mock.patch.object(turbojpeg.subprocess, 'run', side_effect=run) as commands:
                env, actual_source = turbojpeg.prepare('darwin/arm64', {'CGO_CFLAGS': '-O2', 'CGO_LDFLAGS': '-lm'})
            configure = commands.call_args_list[0]
            args, build = configure.args[0], configure.kwargs['cwd']
            self.assertEqual(args[args.index('-S') + 1], '../' + source.name)
            self.assertEqual(args[args.index('-B') + 1], '.')
            self.assertIn('-DCMAKE_INSTALL_PREFIX=install', args)
            self.assertIn('-DCMAKE_OSX_DEPLOYMENT_TARGET=13.0', args)
            self.assertEqual(commands.call_args_list[1].args[0][1:3], ['--build', '.'])
            self.assertEqual(actual_source, source)
            self.assertEqual(env['CGO_CFLAGS'], "-O2 -mmacosx-version-min=13.0 '-I" + str(source / 'src') + "'")
            self.assertEqual(env['CGO_LDFLAGS'], "-lm -mmacosx-version-min=13.0 '-L" + str(build) + "'")


if __name__ == '__main__':
    unittest.main()
