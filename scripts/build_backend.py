#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""从指定的已有 lark-cli 源码构建受控后端；不修改来源工作区。"""
import argparse
import json
import os
import subprocess
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('--output', type=Path, default=Path('bin/lark-cli-gateway'))
    parser.add_argument('--test', action='store_true', help='运行 wrapper 测试和 vet')
    parser.add_argument('--goos')
    parser.add_argument('--goarch')
    args = parser.parse_args()
    project = Path(__file__).resolve().parents[1]
    source = args.source.resolve()
    commit = subprocess.check_output(['git', '-C', str(source), 'rev-parse', 'HEAD'], text=True).strip()
    if subprocess.check_output(['git', '-C', str(source), 'status', '--porcelain'], text=True).strip():
        raise SystemExit('来源工作区有未提交修改，不能声称构建固定源码版本')
    expected = json.loads((project/'internal/gateway/catalog.json').read_text())['source_commit']
    if commit != expected:
        raise SystemExit('来源提交与当前 Gateway 目录版本不一致')
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
