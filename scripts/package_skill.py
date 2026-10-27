#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""打包可上传到 Wake 等宿主的 feishu-gateway Skill：SKILL.md 加离线指南 references/。

指南与网关 feishu_guide 返回的内容相同，来自 internal/gateway/guidance/。不访问网络，不修改仓库。
"""
import argparse
import json
import shutil
import zipfile
from pathlib import Path


def add_references(project, skill_dir):
    """把每个领域指南目录复制到 skill_dir/references/<上游 Skill 名>/。"""
    guidance = project / 'internal/gateway/guidance'
    references = skill_dir / 'references'
    references.mkdir(parents=True, exist_ok=False)
    for source in sorted(p for p in guidance.iterdir() if p.is_dir()):
        shutil.copytree(source, references / source.name)
    return sum(1 for _ in references.rglob('*.md'))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('output', type=Path, help='输出目录；在其中生成 feishu-gateway/')
    parser.add_argument('--zip', action='store_true', help='同时生成 feishu-gateway.zip，便于上传')
    args = parser.parse_args()
    project = Path(__file__).resolve().parents[1]
    target = args.output.resolve() / 'feishu-gateway'
    if target.exists():
        raise SystemExit('目标已存在，请换一个空目录: ' + str(target))
    target.mkdir(parents=True)
    shutil.copy2(project / 'skills/feishu-gateway/SKILL.md', target / 'SKILL.md')
    count = add_references(project, target)
    report = {'skill': str(target), 'reference_files': count}
    if args.zip:
        archive = target.parent / 'feishu-gateway.zip'
        with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as z:
            for path in sorted(target.rglob('*')):
                if path.is_file():
                    info = zipfile.ZipInfo(str(path.relative_to(target.parent)), (2026, 1, 1, 0, 0, 0))
                    info.external_attr = 0o644 << 16
                    info.compress_type = zipfile.ZIP_DEFLATED
                    z.writestr(info, path.read_bytes())
        report['zip'] = str(archive)
    print(json.dumps(report, ensure_ascii=False))


if __name__ == '__main__':
    main()
