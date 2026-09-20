"""離線驗證建置工具預檢與 macOS Bash 3.2 的錯誤處理。"""
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest
from unittest import mock

import ffmpeg


class BuildPrerequisiteTests(unittest.TestCase):
    def test_missing_tools_stop_before_sources(self):
        with tempfile.TemporaryDirectory() as temp:
            with mock.patch.object(ffmpeg, 'CACHE', Path(temp)), \
                    mock.patch.object(ffmpeg, 'sources') as sources:
                with self.assertRaisesRegex(RuntimeError, 'cmake, pkg-config, make'):
                    ffmpeg.prepare('darwin/arm64', {'PATH': temp})
                sources.assert_not_called()

    def test_uses_child_environment_path_and_requires_nasm_only_for_x64(self):
        def tool(name, path):
            self.assertEqual(path, '/child/tools')
            return None if name == 'nasm' else '/child/tools/' + name

        with mock.patch.object(ffmpeg.shutil, 'which', side_effect=tool):
            tools = ffmpeg.required_build_tools('arm64', {'PATH': '/child/tools'})
            self.assertEqual(tools['pkg-config'], '/child/tools/pkg-config')
            with self.assertRaisesRegex(RuntimeError, 'nasm'):
                ffmpeg.required_build_tools('amd64', {'PATH': '/child/tools'})

    def test_missing_path_uses_subprocess_default(self):
        with mock.patch.object(ffmpeg.shutil, 'which', return_value='/tool') as which:
            ffmpeg.required_build_tools('arm64', {})
            for call in which.call_args_list:
                self.assertEqual(call.kwargs['path'], os.defpath)

    def test_complete_cache_does_not_require_build_tools(self):
        with mock.patch.object(ffmpeg.artifacts, 'current', return_value=True), \
                mock.patch.object(ffmpeg.artifacts, 'locked'), \
                mock.patch.object(ffmpeg.artifacts, 'source_state', return_value={}), \
                mock.patch.object(ffmpeg.artifacts, 'clean_intermediates'), \
                mock.patch.object(ffmpeg, 'validate_build'), \
                mock.patch.object(ffmpeg, 'required_build_tools') as check, \
                mock.patch.object(ffmpeg, 'sources') as sources:
            ffmpeg.prepare('darwin/arm64', {'PATH': '/missing'})
            check.assert_not_called()
            sources.assert_not_called()

    def test_launcher_failure_preserves_exit_code(self):
        root = Path(__file__).resolve().parent.parent
        for name in ('runUITest.command', 'remoteClientOnly.command'):
            text = (root / name).read_text(encoding='utf-8')
            finish = re.search(r'^finish\(\) \{.*?^\}', text, re.M | re.S).group()
            for locale in ('C', 'en_US.UTF-8', 'C.UTF-8'):
                with self.subTest(script=name, locale=locale):
                    script = 'set -Eeuo pipefail\nBUILD_STAGE=""\n' + finish + '\ntrap finish EXIT\nexit 7\n'
                    result = subprocess.run(['/bin/bash', '-c', script], input='',
                                            capture_output=True, text=True,
                                            env=dict(os.environ, LC_ALL=locale), timeout=5)
                    self.assertEqual(result.returncode, 7, result.stderr)
                    self.assertIn('結束碼：7', result.stderr)
                    self.assertNotIn('unbound variable', result.stderr)


if __name__ == '__main__':
    unittest.main()
