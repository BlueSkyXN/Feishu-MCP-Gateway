#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""从指定的已有 lark-cli 源码构建受控后端；不修改来源工作区。"""
import argparse
import json
import os
import re
import subprocess
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('--output', type=Path, default=Path('bin/lark-cli-gateway'))
    parser.add_argument('--test', action='store_true', help='运行 wrapper 测试和 vet')
    parser.add_argument('--goos')
    parser.add_argument('--goarch')
    parser.add_argument('--source-commit', help='目录升级时显式指定新的完整来源提交；默认使用现有目录的固定提交')
    args = parser.parse_args()
    project = Path(__file__).resolve().parents[1]
    source = args.source.resolve()
    commit = subprocess.check_output(['git', '-C', str(source), 'rev-parse', 'HEAD'], text=True).strip()
    if subprocess.check_output(['git', '-C', str(source), 'status', '--porcelain'], text=True).strip():
        raise SystemExit('来源工作区有未提交修改，不能声称构建固定源码版本')
    expected = args.source_commit or json.loads((project/'internal/gateway/catalog.json').read_text())['source_commit']
    if not re.fullmatch(r'[0-9a-f]{40}', expected):
        raise SystemExit('source-commit 需要完整的 40 位 Git 提交摘要')
    if commit != expected:
        raise SystemExit('来源提交与固定提交不一致；升级目录时用 --source-commit 显式指定目标提交')
    local = project/'local/backend-build'
    local.mkdir(parents=True, exist_ok=True)
    mod = local/'backend.mod'
    mod.write_text((project/'backend/go.mod').read_text()+'\nreplace github.com/larksuite/cli => '+json.dumps(str(source))+'\n')
    env = os.environ.copy()
    if args.goos:
        env['GOOS'] = args.goos
        env['CGO_ENABLED'] = '0'
    if args.goarch:
        env['GOARCH'] = args.goarch
    subprocess.run(['go', 'mod', 'tidy', '-modfile='+str(mod)], cwd=project/'backend', env=env, check=True)
    if args.test:
        subprocess.run(['go', 'test', '-race', '-tags=authsidecar', '-modfile='+str(mod), './...'], cwd=project/'backend', env=env, check=True)
        subprocess.run(['go', 'vet', '-tags=authsidecar', '-modfile='+str(mod), './...'], cwd=project/'backend', env=env, check=True)
    output = args.output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(['go', 'build', '-tags=authsidecar', '-modfile='+str(mod), '-trimpath', '-ldflags=-X main.sourceCommit='+commit, '-o', str(output), '.'], cwd=project/'backend', env=env, check=True)
    print(json.dumps({'output':str(output),'source_commit':commit}))


if __name__ == '__main__':
    main()
