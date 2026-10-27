#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""verify_release.py 的自测：用固定字节代替 Go 构建生成发布目录，确认篡改包内二进制、重建结果或文件清单时核对失败。"""
import hashlib
import json
import subprocess
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path

VERIFY = Path(__file__).resolve().parent/'verify_release.py'
TAG = 'v0.0.0-selftest'
TARGETS = ['darwin-arm64', 'linux-amd64']
BINARIES = ['feishu-mcp-gateway', 'lark-cli-gateway']
REBUILT = b'rebuilt fixture bytes\n'


def digest(data):
    return hashlib.sha256(data).hexdigest()


def archive(tree, output):
    with tarfile.open(output, 'w:gz') as stream:
        stream.add(tree, arcname=tree.name)


def make_release(root, packaged=REBUILT, rebuilt=REBUILT, recorded=REBUILT, extra=None):
    """生成与 build_release.py 同结构的发布目录；各参数分别控制包内二进制、源码包重建结果和 PROVENANCE 记录。"""
    release = root/'release'
    release.mkdir()
    source = root/f'Feishu-MCP-Gateway-{TAG}-source'
    source.mkdir()
    (source/'rebuild.py').write_text(
        'import argparse\nfrom pathlib import Path\np = argparse.ArgumentParser()\n'
        "p.add_argument('--goos')\np.add_argument('--goarch')\np.add_argument('--output', type=Path)\n"
        'a = p.parse_args()\na.output.mkdir(parents=True)\n'
        f'for name in {BINARIES!r}:\n    (a.output/name).write_bytes({rebuilt!r})\n')
    archive(source, release/f'{source.name}.tar.gz')
    provenance = {'version': TAG, 'commit': 'selftest', 'lark_cli_commit': 'selftest', 'go_version': 'selftest', 'targets': {}}
    for target in TARGETS:
        bundle = root/f'Feishu-MCP-Gateway-{TAG}-{target}'
        bundle.mkdir()
        for name in BINARIES:
            (bundle/name).write_bytes(packaged)
        provenance['targets'][target] = {name: digest(recorded) for name in BINARIES}
        archive(bundle, release/f'{bundle.name}.tar.gz')
    (release/'PROVENANCE.json').write_text(json.dumps(provenance))
    if extra:
        (release/extra).write_bytes(b'extra\n')
    files = sorted(p for p in release.iterdir() if p.name != extra)
    (release/'SHA256SUMS').write_text(''.join(digest(p.read_bytes())+'  '+p.name+'\n' for p in files))
    return release


def verify(release):
    result = subprocess.run([sys.executable, str(VERIFY), str(release)], capture_output=True, text=True)
    return result.returncode, json.loads(result.stdout)


class VerifyReleaseTest(unittest.TestCase):
    def check(self, passed, **fixture):
        with tempfile.TemporaryDirectory(prefix='verify-release-test-') as directory:
            code, report = verify(make_release(Path(directory), **fixture))
        self.assertEqual(code == 0, passed, report)
        self.assertEqual(report['passed'], passed, report)
        return report

    def test_consistent_release_passes(self):
        report = self.check(True)
        self.assertEqual(len(report['binaries']), len(TARGETS)*len(BINARIES))

    def test_packaged_binary_differs(self):
        report = self.check(False, packaged=b'different packaged bytes\n')
        self.assertTrue(all(item['archive_sha256'] != item['provenance_sha256'] for item in report['binaries']))

    def test_rebuild_differs_from_record(self):
        self.check(False, rebuilt=b'different rebuild bytes\n')

    def test_record_differs_from_package_and_rebuild(self):
        self.check(False, recorded=b'different recorded bytes\n')

    def test_file_missing_from_sums(self):
        report = self.check(False, extra='unlisted.tar.gz')
        self.assertEqual(report['files_missing_from_sums'], ['unlisted.tar.gz'])


if __name__ == '__main__':
    unittest.main()
