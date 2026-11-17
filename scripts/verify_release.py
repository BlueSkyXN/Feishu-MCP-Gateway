#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""核对发布文件、二进制的 PROVENANCE 与离线重建结果，以及平台客户端脚本与对应源码的一致性。"""
import argparse
import hashlib
import json
import os
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path

BINARIES = ['feishu-mcp-gateway', 'lark-cli-gateway']
CLIENT_SCRIPTS = ['request_client.py', 'transfer_file.py']


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
    provenance = json.loads((release/'PROVENANCE.json').read_text())
    tag = provenance['version']
    targets = provenance['targets']
    # 与 build_release.py 写入 SHA256SUMS、发布工作流收集附件的范围一致；目录中的其他文件不发布。
    published = sorted([p.name for p in release.glob('*.tar.gz')] + (['PROVENANCE.json'] if (release/'PROVENANCE.json').is_file() else []))
    source_name = f'Feishu-MCP-Gateway-{tag}-source'
    expected_files = sorted([f'{source_name}.tar.gz', 'PROVENANCE.json', *(f'Feishu-MCP-Gateway-{tag}-{t}.tar.gz' for t in targets)])
    mismatched = sorted(name for name, digest in sums.items() if (release/name).is_file() and sha256(release/name) != digest)
    report = {'version': tag, 'commit': provenance['commit'], 'lark_cli_commit': provenance['lark_cli_commit'], 'go_version': provenance['go_version'],
              'checksums_verified': len(sums) - len(mismatched), 'checksum_mismatches': mismatched,
              # 发布目录、SHA256SUMS 与 PROVENANCE 的目标列表必须描述同一组文件。
              'files_missing_from_sums': sorted(set(published) - set(sums)), 'files_missing_from_release': sorted(set(sums) - set(published)),
              'files_not_in_provenance': sorted(set(published) - set(expected_files)), 'provenance_files_missing': sorted(set(expected_files) - set(published)),
              'binaries': [], 'client_scripts': []}
    with tempfile.TemporaryDirectory(prefix='release-verify-') as directory:
        root = Path(directory)
        with tarfile.open(release/f'{source_name}.tar.gz') as archive:
            archive.extractall(root/'source', filter='data')
        tree = root/'source'/source_name
        # rebuild.py 自身关闭模块代理与工具链下载；这里另给空的模块缓存与构建缓存。
        env = {'PATH': os.environ['PATH'], 'HOME': str(root/'home'), 'GOPATH': str(root/'gopath'), 'GOMODCACHE': str(root/'modcache'), 'GOCACHE': str(root/'gocache')}
        for target, recorded in sorted(targets.items()):
            goos, goarch = target.split('-')
            bundle = f'Feishu-MCP-Gateway-{tag}-{target}'
            packaged = root/'packages'/target
            if (release/f'{bundle}.tar.gz').is_file():
                with tarfile.open(release/f'{bundle}.tar.gz') as archive:
                    archive.extractall(packaged, filter='data')
            for name in CLIENT_SCRIPTS:
                source_path = tree/'project/scripts'/name
                # 旧版本源码没有请求客户端；按对应源码实际提供的入口核对平台包。
                if not source_path.is_file():
                    continue
                archive_path = packaged/bundle/'scripts'/name
                item = {'target': target, 'script': 'scripts/'+name,
                        'source_sha256': sha256(source_path),
                        'archive_sha256': sha256(archive_path) if archive_path.is_file() else None}
                item['identical'] = item['archive_sha256'] == item['source_sha256']
                report['client_scripts'].append(item)
            rebuilt = root/'rebuild'/target
            subprocess.run([sys.executable, 'rebuild.py', '--goos', goos, '--goarch', goarch, '--output', str(rebuilt)], cwd=tree, env=env, check=True)
            for name in sorted(set(BINARIES) | set(recorded)):
                archive_path, rebuild_path = packaged/bundle/name, rebuilt/name
                item = {'target': target, 'binary': name, 'provenance_sha256': recorded.get(name),
                        'archive_sha256': sha256(archive_path) if archive_path.is_file() else None,
                        'rebuild_sha256': sha256(rebuild_path) if rebuild_path.is_file() else None}
                item['identical'] = item['provenance_sha256'] is not None and item['provenance_sha256'] == item['archive_sha256'] == item['rebuild_sha256']
                report['binaries'].append(item)
    report['passed'] = (not mismatched and not report['files_missing_from_sums'] and not report['files_missing_from_release']
                        and not report['files_not_in_provenance'] and not report['provenance_files_missing']
                        and bool(report['binaries']) and all(item['identical'] for item in report['binaries'])
                        and all(item['identical'] for item in report['client_scripts']))
    text = json.dumps(report, ensure_ascii=False, indent=2)+'\n'
    if args.output:
        args.output.write_text(text)
    print(text, end='')
    if not report['passed']:
        raise SystemExit('发布包核对失败：请检查上方 JSON 中的文件清单、校验和、二进制重建结果及客户端脚本一致性。')


if __name__ == '__main__':
    main()
