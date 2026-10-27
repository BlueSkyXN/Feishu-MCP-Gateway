#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""核对 build_release.py 的输出：SHA256SUMS 与文件一致；对应源码包在关闭网络、空模块与构建缓存时重建，二进制与 PROVENANCE.json 记录一致。"""
import argparse
import hashlib
import json
import os
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('release', type=Path, help='build_release.py 的 --output 目录')
    parser.add_argument('--output', type=Path, help='另存核对结果 JSON')
    args = parser.parse_args()
    release = args.release.resolve()
    sums = {}
    for line in (release/'SHA256SUMS').read_text().splitlines():
        digest, name = line.split('  ', 1)
        sums[name] = digest
    mismatched = sorted(name for name, digest in sums.items() if sha256(release/name) != digest)
    provenance = json.loads((release/'PROVENANCE.json').read_text())
    tag = provenance['version']
    report = {'version': tag, 'commit': provenance['commit'], 'lark_cli_commit': provenance['lark_cli_commit'], 'go_version': provenance['go_version'],
              'checksums_verified': len(sums) - len(mismatched), 'checksum_mismatches': mismatched, 'offline_rebuild': []}
    with tempfile.TemporaryDirectory(prefix='release-verify-') as directory:
        root = Path(directory)
        with tarfile.open(release/f'Feishu-MCP-Gateway-{tag}-source.tar.gz') as archive:
            archive.extractall(root, filter='data')
        tree = root/f'Feishu-MCP-Gateway-{tag}-source'
        # rebuild.py 自身关闭模块代理与工具链下载；这里另给空的模块缓存与构建缓存。
        env = {'PATH': os.environ['PATH'], 'HOME': str(root/'home'), 'GOPATH': str(root/'gopath'), 'GOMODCACHE': str(root/'modcache'), 'GOCACHE': str(root/'gocache')}
        for target, expected in sorted(provenance['targets'].items()):
            goos, goarch = target.split('-')
            output = root/'build'/target
            subprocess.run([sys.executable, 'rebuild.py', '--goos', goos, '--goarch', goarch, '--output', str(output)], cwd=tree, env=env, check=True)
            for name, digest in sorted(expected.items()):
                actual = sha256(output/name)
                report['offline_rebuild'].append({'target': target, 'binary': name, 'sha256': actual, 'identical_to_release': actual == digest})
    report['passed'] = not mismatched and all(item['identical_to_release'] for item in report['offline_rebuild'])
    text = json.dumps(report, ensure_ascii=False, indent=2)+'\n'
    if args.output:
        args.output.write_text(text)
    print(text, end='')
    if not report['passed']:
        raise SystemExit('发布包核对失败：校验和不一致，或源码包重建结果与发布二进制不同')


if __name__ == '__main__':
    main()
