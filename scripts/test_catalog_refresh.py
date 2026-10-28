#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""用本地 Git fixture 检查来源固定与升级入口；不执行 Go 构建或访问网络。"""
import contextlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import build_backend
import refresh_catalog
import lark_guidance


class CatalogRefreshTest(unittest.TestCase):
    def setUp(self):
        isolated_git = mock.patch.dict(os.environ, {"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_SYSTEM": os.devnull, "GIT_CONFIG_COUNT": "0", "GIT_CONFIG_PARAMETERS": ""})
        isolated_git.start()
        self.addCleanup(isolated_git.stop)
        temporary = tempfile.TemporaryDirectory(prefix='catalog-refresh-test-')
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.project = self.root/'project'
        (self.project/'internal/gateway').mkdir(parents=True)
        (self.project/'backend').mkdir()
        (self.project/'backend/go.mod').write_text('module example.invalid/backend\n\ngo 1.26.0\n')
        self.source = self.root/'source'
        self.source.mkdir()
        subprocess.run(['git', 'init', '-q', str(self.source)], check=True)
        (self.source/'fixture.txt').write_text('fixture\n')
        subprocess.run(['git', '-C', str(self.source), 'add', 'fixture.txt'], check=True)
        subprocess.run(['git', '-C', str(self.source), '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-qm', 'fixture'], check=True)
        self.commit = subprocess.check_output(['git', '-C', str(self.source), 'rev-parse', 'HEAD'], text=True).strip()
        self.catalog = self.project/'internal/gateway/catalog.json'
        self.catalog.write_text(json.dumps({'source_commit': '0'*40}))

    def build(self, *args):
        runner = mock.Mock()
        runner.check_output.side_effect = subprocess.check_output
        with mock.patch.object(build_backend, '__file__', str(self.project/'scripts/build_backend.py')), mock.patch.object(build_backend, 'subprocess', runner), mock.patch.object(sys, 'argv', ['build_backend.py', str(self.source), '--output', str(self.root/'backend'), *args]):
            with contextlib.redirect_stdout(io.StringIO()):
                build_backend.main()
        return runner

    def test_default_rejects_different_source(self):
        with self.assertRaisesRegex(SystemExit, '来源提交与固定提交不一致'):
            self.build()

    def test_default_accepts_catalog_pin(self):
        self.catalog.write_text(json.dumps({'source_commit': self.commit}))
        self.assert_build_pin(self.build())

    def test_explicit_upgrade_pin_accepts_new_source(self):
        self.assert_build_pin(self.build('--source-commit', self.commit))
        self.assertEqual(json.loads(self.catalog.read_text())['source_commit'], '0'*40)

    def assert_build_pin(self, runner):
        commands = [call.args[0] for call in runner.run.call_args_list]
        self.assertIn('-ldflags=-X main.sourceCommit='+self.commit, commands[-1])

    def test_explicit_pin_does_not_allow_mismatch(self):
        with self.assertRaisesRegex(SystemExit, '来源提交与固定提交不一致'):
            self.build('--source-commit', '1'*40)

    def test_explicit_pin_does_not_allow_dirty_source(self):
        (self.source/'fixture.txt').write_text('modified\n')
        with self.assertRaisesRegex(SystemExit, '来源工作区有未提交修改'):
            self.build('--source-commit', self.commit)

    def test_pin_requires_complete_commit(self):
        with self.assertRaisesRegex(SystemExit, '完整的 40 位'):
            self.build('--source-commit', self.commit[:12])

    def test_refresh_forwards_explicit_pin(self):
        runner = mock.Mock()
        runner.run.return_value.stdout = b'{"shortcuts":[]}\n'
        with mock.patch.object(refresh_catalog, '__file__', str(self.project/'scripts/refresh_catalog.py')), mock.patch.object(refresh_catalog, 'subprocess', runner), mock.patch.object(sys, 'argv', ['refresh_catalog.py', str(self.source), '--source-commit', self.commit]):
            with contextlib.redirect_stdout(io.StringIO()):
                refresh_catalog.main()
        build = runner.run.call_args_list[0].args[0]
        self.assertEqual(build[-2:], ['--source-commit', self.commit])
        self.assertTrue(runner.run.call_args_list[-1].args[0][1].endswith('/scripts/import_catalog.py'))


class ArtifactGuidanceTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        catalog = Path(__file__).resolve().parents[1]/'internal/gateway/catalog.json'
        cls.index = lark_guidance.Index(json.loads(catalog.read_text())['operations'], [])

    def test_ndjson_without_path_becomes_artifact_request(self):
        call = lark_guidance.parse_call(self.index, ['base', '+record-list', '--base-token', 'app_fixture', '--table-id', 'tbl_fixture', '--format', 'ndjson'], 'base', True)
        self.assertEqual(call[1]['output'], 'records.ndjson')

    def test_explicit_path_is_preserved(self):
        call = lark_guidance.parse_call(self.index, ['base', '+record-list', '--format=ndjson', '--output', 'exports/result.ndjson'], 'base', True)
        self.assertEqual(call[1]['output'], 'exports/result.ndjson')

    def test_json_stays_inline(self):
        call = lark_guidance.parse_call(self.index, ['base', '+record-list', '--format', 'json'], 'base', True)
        self.assertNotIn('output', call[1])


if __name__ == '__main__':
    unittest.main()
