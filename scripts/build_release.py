#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""从干净 Git 提交构建二进制包和可离线重建的对应源码包。"""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile


def run(argv, cwd, env=None, capture=False):
    return subprocess.check_output(argv, cwd=cwd, env=env, text=True) if capture else subprocess.run(argv, cwd=cwd, env=env, check=True)


def snapshot(repo, target):
    data = subprocess.check_output(['git', 'archive', '--format=tar', 'HEAD'], cwd=repo)
    target.mkdir(parents=True)
    with tarfile.open(fileobj=io.BytesIO(data)) as archive:
        archive.extractall(target, filter='data')


def json_stream(raw):
    decoder, offset = json.JSONDecoder(), 0
    while offset < len(raw):
        while offset < len(raw) and raw[offset].isspace():
            offset += 1
        if offset == len(raw):
            return
        value, offset = decoder.raw_decode(raw, offset)
        yield value


def archive_tree(source, dest):
    with tarfile.open(dest, 'w:gz') as archive:
        for path in sorted([source, *source.rglob('*')]):
            info = archive.gettarinfo(str(path), arcname=str(Path(source.name)/path.relative_to(source)))
            info.uid = info.gid = 0
            info.uname = info.gname = ''
            if info.isfile():
                with path.open('rb') as stream:
                    archive.addfile(info, stream)
            else:
                archive.addfile(info)


REBUILD = '''#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-only
"""用随包 vendor 源码离线构建；需要已安装 Go 1.26 或更新版本。"""
import argparse, os, subprocess
from pathlib import Path
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--goos',required=True,choices=['darwin','linux'])
p.add_argument('--goarch',required=True,choices=['arm64','amd64'])
p.add_argument('--output',type=Path,required=True)
a=p.parse_args();root=Path(__file__).resolve().parent;out=a.output.resolve();out.mkdir(parents=True,exist_ok=True)
env=os.environ.copy();env.update(GOOS=a.goos,GOARCH=a.goarch,CGO_ENABLED='0',GOPROXY='off',GOSUMDB='off',GOTOOLCHAIN='local',GOFLAGS='')
subprocess.run(['go','build','-mod=vendor','-trimpath','-buildvcs=false','-o',str(out/'feishu-mcp-gateway'),'./cmd/gateway'],cwd=root/'project',env=env,check=True)
subprocess.run(['go','build','-mod=vendor','-trimpath','-buildvcs=false','-tags=authsidecar','-ldflags=-X main.sourceCommit=SOURCE_COMMIT','-o',str(out/'lark-cli-gateway'),'.'],cwd=root/'project/backend',env=env,check=True)
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('lark_source', type=Path)
    parser.add_argument('--tag', default='v0.2.0-dev')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    project = Path(__file__).resolve().parents[1]
    source = args.lark_source.resolve()
    if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9.-]+)?', args.tag):
        raise SystemExit('tag 需要合法版本号')
    for repo in [project, source]:
        if run(['git', 'status', '--porcelain'], repo, capture=True).strip():
            raise SystemExit('构建来源必须为干净 Git 工作区')
    commit = run(['git', 'rev-parse', 'HEAD'], project, capture=True).strip()
    cli_commit = run(['git', 'rev-parse', 'HEAD'], source, capture=True).strip()
    expected = json.loads((project/'internal/gateway/catalog.json').read_text())['source_commit']
    if cli_commit != expected:
        raise SystemExit('lark-cli 提交与目录版本不一致')
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    stage = output/'work'
    stage.mkdir()
    sources = stage/f'Feishu-MCP-Gateway-{args.tag}-source'
    sources.mkdir()
    snapshot(project, sources/'project')
    snapshot(source, sources/'lark-cli')
    backend = sources/'project/backend'
    with (backend/'go.mod').open('a') as stream:
        stream.write('\nreplace github.com/larksuite/cli => ../../lark-cli\n')
    env = os.environ.copy()
    env.update(CGO_ENABLED='0', GOTOOLCHAIN='local', GOFLAGS='')
    run(['go', 'mod', 'tidy'], backend, env)
    # 收集两种目标实际引用的模块及版本；运行清单不公开本机目录。
    modules = {}
    for target_os, target_arch in [('darwin', 'arm64'), ('linux', 'amd64')]:
        target_env = dict(env, GOOS=target_os, GOARCH=target_arch)
        for directory, flags, package in [(sources/'project', [], './cmd/gateway'), (backend, ['-tags=authsidecar'], '.')]:
            for item in json_stream(run(['go', 'list', '-mod=readonly', '-deps', '-json', *flags, package], directory, target_env, True)):
                mod = item.get('Module')
                if not mod or mod.get('Main'):
                    continue
                resolved = mod.get('Replace', mod)
                key = (mod['Path'], mod.get('Version', ''))
                modules[key] = (mod, Path(resolved.get('Dir') or mod['Dir']))
    licenses = sources/'THIRD_PARTY_LICENSES'
    licenses.mkdir()
    manifest = []
    for (module, version), (mod, directory) in sorted(modules.items()):
        folder = licenses/(module.replace('/', '__')+'@'+version)
        folder.mkdir()
        copied = []
        for path in sorted(directory.rglob('*')):
            if not path.is_file() or path.suffix == '.go' or not re.fullmatch(r'(?:licen[sc]e|copying|copyright|notice)(?:[._-].*)?', path.name, re.I):
                continue
            relative = path.relative_to(directory)
            if any(part in ('.git', 'local', 'node_modules', 'vendor') for part in relative.parts):
                continue
            destination = folder/relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(path, destination)
            copied.append(str(relative))
        if not copied:
            raise SystemExit('缺少模块许可证: '+module)
        manifest.append({'module': module, 'version': cli_commit if module == 'github.com/larksuite/cli' else version, 'directory': folder.name, 'license_files': copied})
    goroot = Path(run(['go', 'env', 'GOROOT'], project, capture=True).strip())
    shutil.copy2(goroot/'LICENSE', licenses/'GO-TOOLCHAIN-LICENSE')
    (licenses/'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2)+'\n')
    for directory in [sources/'project', backend]:
        run(['go', 'mod', 'vendor'], directory, env)
    (sources/'rebuild.py').write_text(REBUILD.replace('SOURCE_COMMIT', cli_commit))
    (sources/'SOURCE.md').write_text(f'''# Feishu-MCP-Gateway 对应源码

项目提交：{commit}。lark-cli 提交：{cli_commit}。

project/ 是 Gateway 的发布提交，backend/go.mod 增加相对路径 replacement 并解析锁定依赖。
lark-cli/ 是固定提交的未修改源码；两个 vendor/ 目录提供构建所需依赖。
THIRD_PARTY_LICENSES/ 保留实际链接模块的许可与版本。Go 工具链由本机提供。

在此目录执行：

    python3 rebuild.py --goos darwin --goarch arm64 --output ./build/darwin-arm64
    python3 rebuild.py --goos linux --goarch amd64 --output ./build/linux-amd64

构建关闭网络模块解析和自动工具链下载。源码、第三方源码和许可证均随包提供。
''')
    metadata = {'project': 'Feishu-MCP-Gateway', 'version': args.tag, 'commit': commit, 'lark_cli_commit': cli_commit, 'license': 'GPL-3.0-only', 'go_version': run(['go', 'version'], project, capture=True).strip(), 'modules': manifest, 'targets': {}}
    for target_os, target_arch in [('darwin', 'arm64'), ('linux', 'amd64')]:
        target = target_os+'-'+target_arch
        bundle = stage/f'Feishu-MCP-Gateway-{args.tag}-{target}'
        bundle.mkdir()
        # 实际发布的二进制直接使用对应源码中的离线入口构建。
        run(['python3', 'rebuild.py', '--goos', target_os, '--goarch', target_arch, '--output', str(bundle)], sources)
        for name in ['README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.md']:
            shutil.copy2(sources/'project'/name, bundle/name)
        for name in ['examples', 'skills', 'docs']:
            shutil.copytree(sources/'project'/name, bundle/name)
        (bundle/'scripts').mkdir()
        shutil.copy2(sources/'project/scripts/transfer_file.py', bundle/'scripts/transfer_file.py')
        shutil.copytree(licenses, bundle/'THIRD_PARTY_LICENSES')
        metadata['targets'][target] = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in [bundle/'feishu-mcp-gateway', bundle/'lark-cli-gateway']}
        archive_tree(bundle, output/(bundle.name+'.tar.gz'))
    archive_tree(sources, output/(sources.name+'.tar.gz'))
    (output/'PROVENANCE.json').write_text(json.dumps(metadata, ensure_ascii=False, indent=2)+'\n')
    files = sorted([*output.glob('*.tar.gz'), output/'PROVENANCE.json'])
    (output/'SHA256SUMS').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in files))
    print(json.dumps({'tag': args.tag, 'commit': commit, 'modules_with_licenses': len(manifest), 'artifacts': [p.name for p in files]}, ensure_ascii=False))


if __name__ == '__main__':
    main()
