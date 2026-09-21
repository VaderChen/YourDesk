"""原生影音建置：標準組態、相對建置指令與 CGo 參數回歸測試。"""
import hashlib
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import tempfile
import unittest
from unittest import mock

import ffmpeg
import turbojpeg


def go_split(value):
    """Mirror cmd/go quoted.Split: whole-token quotes, no shell concatenation."""
    result = []
    while value:
        value = value.lstrip(' \t\r\n')
        if not value:
            break
        if value[0] in ('\'', '"'):
            end = value.find(value[0], 1)
            if end == -1:
                raise ValueError('unterminated Go quote')
            result.append(value[1:end])
            value = value[end + 1:]
        else:
            end = re.search(r'[ \t\r\n]', value)
            size = end.start() if end else len(value)
            result.append(value[:size])
            value = value[size:]
    return result


class NativeBuildTests(unittest.TestCase):
    def executable(self, path):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text('#!/bin/sh\nexit 0\n')
        path.chmod(0o755)
        return str(path)

    def prepare_fixture(self, root, configure_error=False, target='darwin/arm64', environment=None):
        cache = root / 'cache'
        source, aom = cache / 'ffmpeg-source', cache / 'aom-source'
        source.mkdir(parents=True, exist_ok=True)
        cmake = aom / 'build/cmake'
        cmake.mkdir(parents=True, exist_ok=True)
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
                (work / 'config.h').write_text('#define CONFIG_GPL 0\n#define CONFIG_NONFREE 0\n'
                                              '#define HAVE_PTHREADS 0\n#define HAVE_W32THREADS 1\n'
                                              '#define FFMPEG_CONFIGURATION "' + ' '.join(args[1:]) + '"\n')
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
                if target.startswith('windows/'):
                    (prefix / 'bin').mkdir(exist_ok=True)
                    for name in ('avcodec', 'avutil', 'swscale'):
                        (prefix / 'lib' / ('lib' + name + '.dll.a')).write_bytes(b'import library fixture')
                        (prefix / 'bin' / (name + '.dll')).write_bytes(b'library fixture')
            return subprocess.CompletedProcess(args, 0)

        tools = {name: self.executable(root / 'tools' / name) for name in ('cmake', 'make', 'pkg-config')}
        if target.startswith('windows/'):
            for key in ('CC', 'CXX'):
                self.executable(Path(environment[key]))
            for name in ('ar', 'ranlib', 'nm', 'windres'):
                self.executable(Path(environment['CC']).parent / ('aarch64-w64-mingw32-' + name))
        with mock.patch.object(ffmpeg, 'CACHE', cache), \
                mock.patch.object(ffmpeg, 'sources', return_value=(source, aom)), \
                mock.patch.object(ffmpeg, 'required_build_tools', return_value=tools), \
                mock.patch.object(ffmpeg.macos_build, 'verify_binary'), \
                mock.patch.object(ffmpeg.windows_runtime, 'validate'), \
                mock.patch.object(ffmpeg.subprocess, 'run', side_effect=run) as commands:
            result = ffmpeg.prepare(target, environment or {})
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
            self.assertIn('--pkg-config=pkg-config', configure.args[0])
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

    def test_windows_configuration_uses_tool_names_and_relative_install(self):
        with tempfile.TemporaryDirectory() as temp:
            toolchain = Path(temp) / 'compiler/bin'
            environment = {'CC': str(toolchain / 'aarch64-w64-mingw32-clang'),
                           'CXX': str(toolchain / 'aarch64-w64-mingw32-clang++')}
            commands, (_, prefix) = self.prepare_fixture(Path(temp), target='windows/arm64', environment=environment)
            configure = commands[3]
            args = configure.args[0]
            self.assertIn('--cc=aarch64-w64-mingw32-clang', args)
            self.assertIn('--cxx=aarch64-w64-mingw32-clang++', args)
            self.assertIn('--cross-prefix=aarch64-w64-mingw32-', args)
            self.assertIn('--ar=aarch64-w64-mingw32-ar', args)
            self.assertIn('--prefix=../install', args)
            self.assertFalse(any(temp in argument or argument.startswith('--pkg-config=/') for argument in args))
            self.assertIn(str(toolchain), configure.kwargs['env']['PATH'].split(os.pathsep))
            self.assertIn('-ffile-prefix-map=', configure.kwargs['env']['CFLAGS'])
            configuration = (prefix.parent / 'metadata/config.h').read_text()
            self.assertNotIn(temp, configuration)

    def test_native_recipe_and_flag_changes_preserve_previous_cache(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            with mock.patch.object(ffmpeg, 'NATIVE_PATH_RECIPE', 'previous-build-recipe'):
                _, (_, previous) = self.prepare_fixture(root)
            binary = previous / 'lib/libavcodec.62.dylib'
            before = (binary.read_bytes(), binary.stat().st_mtime_ns)
            _, (_, current) = self.prepare_fixture(root)
            self.assertNotEqual(previous, current)
            self.assertEqual((binary.read_bytes(), binary.stat().st_mtime_ns), before)
            _, (_, changed) = self.prepare_fixture(root, environment={'CPPFLAGS': '-DEXAMPLE=1'})
            self.assertNotEqual(current, changed)
            self.assertEqual((binary.read_bytes(), binary.stat().st_mtime_ns), before)

    def test_absolute_configuration_and_personal_binary_paths_are_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            config = root / 'config.h'
            (root / 'config_components.h').write_text('')
            for option in ('--prefix=/opt/build/install', '--cc=/opt/build/cc',
                           "--pkg-config='/opt/build/pkg-config'", '--cc=C:/build/cc'):
                config.write_text('#define FFMPEG_CONFIGURATION "' + option + '"\n')
                with self.assertRaisesRegex(RuntimeError, '相對路徑'):
                    ffmpeg.validate_build('darwin', root / 'install', root)
            binary = root / 'library.dylib'
            binary.write_bytes(b'object\0/Users/example-builder/source.c')
            with self.assertRaisesRegex(RuntimeError, '建置路徑驗證失敗'):
                ffmpeg.validate_native_paths([binary])
            binary.write_bytes(b'object\0ffmpeg/source.c\0/system/lib')
            ffmpeg.validate_native_paths([binary])

    def test_copy_runtime_checks_every_library_before_copying(self):
        for system, private in (('darwin', b'/Volumes/example-disk/build/codec.c'),
                                ('windows', b'C:\\Users\\example-builder\\build\\codec.c')):
            with self.subTest(system=system), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                prefix, output = root / 'install', root / 'output'
                output.mkdir()
                names = ('avcodec-62.dll', 'avutil-60.dll', 'swscale-9.dll') if system == 'windows' else ffmpeg.macos_build.RUNTIME_LIBRARIES
                folder = prefix / ('bin' if system == 'windows' else 'lib')
                folder.mkdir(parents=True)
                for index, name in enumerate(names):
                    (folder / name).write_bytes(b'library\0' + (private if index == 2 else b'relative/source.c'))
                with mock.patch.object(ffmpeg.windows_runtime, 'validate'):
                    with self.assertRaisesRegex(RuntimeError, '建置路徑驗證失敗'):
                        ffmpeg.copy_runtime(prefix, output)
                self.assertEqual(list(output.iterdir()), [])

    @unittest.skipUnless(shutil.which('cc') and shutil.which('c++'), 'requires C and C++ compilers')
    def test_real_c_and_cpp_objects_use_relative_macro_and_debug_paths(self):
        with tempfile.TemporaryDirectory(prefix='native 路徑-') as temp:
            root = Path(temp)
            source = root / 'source'
            source.mkdir()
            environment = ffmpeg.native_environment({}, ((root, 'build'), (source, 'source')))
            for compiler, extension, variable in (('cc', 'c', 'CFLAGS'), ('c++', 'cpp', 'CXXFLAGS')):
                path = source / ('fixture.' + extension)
                path.write_text('const char *source_name = __FILE__;\nint fixture(void) { return source_name[0]; }\n')
                output = root / ('fixture-' + extension + '.o')
                subprocess.run([shutil.which(compiler), '-g', *shlex.split(environment[variable]),
                                '-c', str(path), '-o', str(output)], cwd=root,
                               capture_output=True, check=True)
                data = output.read_bytes()
                self.assertNotIn(str(root).encode(), data)
                self.assertNotIn(str(root.resolve()).encode(), data)
                self.assertIn(('source/fixture.' + extension).encode(), data)

    def test_ffmpeg_cgo_quotes_whole_arguments(self):
        # CGo 參數引用是一般正確性要求，與 FFmpeg 的來源路徑限制分開驗證。
        with tempfile.TemporaryDirectory(prefix='cgo flags-') as temp:
            prefix = Path(temp) / 'install'
            env = ffmpeg.build_environment(ffmpeg.macos_build.environment('darwin/arm64', {}), prefix, 'darwin')
            self.assertTrue(env['CGO_CFLAGS'].endswith("'-I" + str(prefix / 'include') + "'"))
            self.assertTrue(env['CGO_LDFLAGS'].endswith("'-L" + str(prefix / 'lib') + "'"))
            self.assertIn('-mmacosx-version-min=13.0', shlex.split(env['CGO_CFLAGS']))
            self.assertIn('-trimpath', shlex.split(env['GOFLAGS']))
            self.assertIn('-ffile-prefix-map=', env['CGO_CPPFLAGS'])

    def test_cgo_single_and_double_quote_paths_use_go_token_rules(self):
        for name in ("builder's files", 'builder"s files'):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temp:
                prefix = Path(temp) / name
                env = ffmpeg.build_environment({}, prefix, 'darwin')
                self.assertEqual(go_split(env['CGO_CFLAGS']), ['-I' + str(prefix / 'include')])
                self.assertEqual(go_split(env['CGO_LDFLAGS']), ['-L' + str(prefix / 'lib')])
                self.assertEqual(go_split(env['CGO_CPPFLAGS']), ffmpeg.native_path_flags(((ffmpeg.ROOT, '.'), (prefix, 'native'))))
        with self.assertRaisesRegex(RuntimeError, '單引號與雙引號'):
            ffmpeg.build_environment({}, Path('both\'and"quotes'), 'darwin')

    def test_configure_tools_rejects_path_shadowing_of_requested_compiler(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            requested = self.executable(root / 'toolchain/clang')
            other = self.executable(root / 'system/clang')
            cxx = self.executable(root / 'system/clang++')
            pkg = self.executable(root / 'system/pkg-config')
            env = {'PATH': str(root / 'toolchain')}
            with self.assertRaisesRegex(RuntimeError, 'PATH 工具名稱衝突：cc'):
                ffmpeg.configure_tools({'cc': requested, 'cxx': cxx, 'pkg-config': pkg}, env)
            # The safe case still writes only names to configure, while every
            # name resolves to the exact originally requested executable.
            env = {'PATH': str(root / 'system')}
            selected = ffmpeg.configure_tools({'cc': other, 'cxx': cxx, 'pkg-config': pkg}, env)
            self.assertEqual(selected, {'cc': 'clang', 'cxx': 'clang++', 'pkg-config': 'pkg-config'})
            self.assertEqual(Path(shutil.which(selected['cc'], path=env['PATH'])).resolve(), Path(other).resolve())
            with self.assertRaisesRegex(RuntimeError, '找不到指定工具：ar'):
                ffmpeg.configure_tools({'ar': str(root / 'missing/ar')}, env)

    def test_configure_tools_bare_names_are_pinned_before_path_changes(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.executable(root / 'original/clang')
            self.executable(root / 'new/clang')
            pkg = self.executable(root / 'new/pkg-config')
            with self.assertRaisesRegex(RuntimeError, 'PATH 工具名稱衝突：cc'):
                ffmpeg.configure_tools({'cc': 'clang', 'pkg-config': pkg}, {'PATH': str(root / 'original')})
            alias = root / 'alias'
            alias.symlink_to(root / 'original', target_is_directory=True)
            pkg = self.executable(root / 'original/pkg-config')
            selected = ffmpeg.configure_tools({'cc': 'clang', 'pkg-config': pkg}, {'PATH': str(alias)})
            self.assertEqual(selected['cc'], 'clang')

    @unittest.skipUnless(shutil.which('cc'), 'requires C compiler')
    def test_ffmpeg_unquoted_flags_support_unicode_under_c_locale(self):
        with tempfile.TemporaryDirectory(prefix='原生路徑-') as temp:
            root = Path(temp)
            source, output = root / 'fixture.c', root / 'fixture.o'
            source.write_text('const char *source_name = __FILE__;\n')
            environment = ffmpeg.native_environment(dict(os.environ, LC_ALL='C'), ((root, 'source'),), for_configure=True)
            self.assertNotIn("'", environment['CFLAGS'])
            subprocess.run(['/bin/sh', '-c', '"$1" $CFLAGS -g -c "$2" -o "$3"',
                            'configure-test', shutil.which('cc'), str(source), str(output)],
                           cwd=root, env=environment, capture_output=True, check=True)
            data = output.read_bytes()
            self.assertNotIn(str(root).encode(), data)
            self.assertNotIn(str(root.resolve()).encode(), data)
            self.assertIn(b'source/fixture.c', data)

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
            self.assertIn('-ffile-prefix-map=', configure.kwargs['env']['CFLAGS'])
            self.assertIn('-fdebug-prefix-map=', configure.kwargs['env']['CXXFLAGS'])
            self.assertIn('-trimpath', shlex.split(env['GOFLAGS']))

    def test_turbojpeg_nasm_reproducible_windows_build(self):
        with tempfile.TemporaryDirectory() as temp:
            cache = Path(temp)
            data = b'cached archive fixture'
            (cache / f'libjpeg-turbo-{turbojpeg.VERSION}.tar.gz').write_bytes(data)
            (cache / f'libjpeg-turbo-{turbojpeg.VERSION}').mkdir()
            def run(args, **kwargs):
                if '--build' in args:
                    (kwargs['cwd'] / 'libturbojpeg.a').write_bytes(b'valid library')
                return subprocess.CompletedProcess(args, 0)
            with mock.patch.object(turbojpeg, 'CACHE', cache), \
                 mock.patch.object(turbojpeg, 'SHA256', hashlib.sha256(data).hexdigest()), \
                 mock.patch.object(turbojpeg.shutil, 'which', return_value='/tools/nasm'), \
                 mock.patch.object(turbojpeg.subprocess, 'run', side_effect=run) as commands:
                turbojpeg.prepare('windows/amd64', {'CC': 'x86_64-w64-mingw32-gcc'})
                self.assertIn('-DCMAKE_ASM_NASM_FLAGS=--reproducible', commands.call_args_list[0].args[0])
                count = commands.call_count
                turbojpeg.prepare('windows/amd64', {'CC': 'x86_64-w64-mingw32-gcc'})
                self.assertEqual(commands.call_count, count)
                turbojpeg.prepare('windows/arm64', {'CC': 'aarch64-w64-mingw32-clang'})
                self.assertNotIn('-DCMAKE_ASM_NASM_FLAGS=--reproducible', commands.call_args_list[-2].args[0])

    def test_turbojpeg_native_flags_select_new_cache_and_unchanged_build_is_reused(self):
        with tempfile.TemporaryDirectory() as temp:
            cache = Path(temp)
            data = b'cached archive fixture'
            (cache / f'libjpeg-turbo-{turbojpeg.VERSION}.tar.gz').write_bytes(data)
            (cache / f'libjpeg-turbo-{turbojpeg.VERSION}').mkdir()
            builds = []
            def run(args, **kwargs):
                if '--build' in args:
                    build = kwargs['cwd']
                    builds.append(build)
                    (build / 'libturbojpeg.a').write_bytes(b'valid relative library fixture')
                return subprocess.CompletedProcess(args, 0)
            with mock.patch.object(turbojpeg, 'CACHE', cache), \
                 mock.patch.object(turbojpeg, 'SHA256', hashlib.sha256(data).hexdigest()), \
                 mock.patch.object(turbojpeg.shutil, 'which', return_value='/tools/cmake'), \
                 mock.patch.object(turbojpeg.subprocess, 'run', side_effect=run):
                turbojpeg.prepare('darwin/arm64', {'CFLAGS': '-O2'})
                previous = builds[0] / 'libturbojpeg.a'
                before = (previous.read_bytes(), previous.stat().st_mtime_ns)
                turbojpeg.prepare('darwin/arm64', {'CFLAGS': '-O2'})
                self.assertEqual(len(builds), 1)
                turbojpeg.prepare('darwin/arm64', {'CFLAGS': '-O3'})
                self.assertEqual(len(builds), 2)
                self.assertNotEqual(builds[0], builds[1])
                self.assertEqual((previous.read_bytes(), previous.stat().st_mtime_ns), before)
                previous.write_bytes(b'library\0/home/example-builder/source.c')
                with self.assertRaisesRegex(RuntimeError, '建置路徑驗證失敗'):
                    turbojpeg.prepare('darwin/arm64', {'CFLAGS': '-O2'})
                self.assertEqual(len(builds), 2)

    def test_turbojpeg_cgo_quoted_paths_and_early_unencodable_path_rejection(self):
        for name in ("builder's files", 'builder"s files', 'both\'and"quotes'):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temp:
                cache = Path(temp) / name
                cache.mkdir()
                data = b'cached archive fixture'
                (cache / f'libjpeg-turbo-{turbojpeg.VERSION}.tar.gz').write_bytes(data)
                source = cache / f'libjpeg-turbo-{turbojpeg.VERSION}'
                source.mkdir()
                builds = []
                def run(args, **kwargs):
                    if '--build' in args:
                        builds.append(kwargs['cwd'])
                        (kwargs['cwd'] / 'libturbojpeg.a').write_bytes(b'library fixture')
                    return subprocess.CompletedProcess(args, 0)
                with mock.patch.object(turbojpeg, 'CACHE', cache), \
                     mock.patch.object(turbojpeg, 'SHA256', hashlib.sha256(data).hexdigest()), \
                     mock.patch.object(turbojpeg.shutil, 'which', return_value='/tools/cmake'), \
                     mock.patch.object(turbojpeg.subprocess, 'run', side_effect=run) as commands:
                    if name.startswith('both'):
                        with self.assertRaisesRegex(RuntimeError, '單引號與雙引號'):
                            turbojpeg.prepare('darwin/arm64', {})
                        commands.assert_not_called()
                    else:
                        env, _ = turbojpeg.prepare('darwin/arm64', {})
                        self.assertIn('-I' + str(source / 'src'), go_split(env['CGO_CFLAGS']))
                        self.assertIn('-L' + str(builds[0]), go_split(env['CGO_LDFLAGS']))
                        expected = ffmpeg.native_path_flags(((turbojpeg.ROOT, '.'), (source, 'libjpeg-turbo'), (builds[0], 'build')))
                        self.assertEqual(go_split(env['CGO_CPPFLAGS']), expected)


if __name__ == '__main__':
    unittest.main()
