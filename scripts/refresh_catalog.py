#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""从干净的 lark-cli 源码重新生成目录、覆盖台账和用法指南。

依次执行：构建受控后端、用临时 HOME 导出 shortcut 声明、运行 import_catalog.py。
不请求飞书，不读取开发者的 CLI 配置。完成后按提示核对差异并重新渲染 Skill。
"""
import argparse
import subprocess
import sys
import tempfile
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path, help='lark-cli 源码目录，需处于目标提交且无未提交修改')
    parser.add_argument('--source-commit', help='升级来源时指定目标完整提交；省略时核对现有目录提交')
    args = parser.parse_args()
    project = Path(__file__).resolve().parents[1]
    source = args.source.resolve()
    backend = project / 'local/import/lark-cli-gateway'
    shortcuts = project / 'local/import/shortcuts.json'
    shortcuts.parent.mkdir(parents=True, exist_ok=True)
    build = [sys.executable, str(project / 'scripts/build_backend.py'), str(source), '--output', str(backend)]
    if args.source_commit:
        build.extend(['--source-commit', args.source_commit])
    subprocess.run(build, cwd=project, check=True)
    with tempfile.TemporaryDirectory(prefix='gateway-export-') as home:
        env = {'HOME': home, 'PATH': '/usr/bin:/bin', 'CI': '1', 'LARKSUITE_CLI_CONFIG_DIR': home + '/config', 'LARKSUITE_CLI_NO_UPDATE_NOTIFIER': '1', 'LARKSUITE_CLI_NO_SKILLS_NOTIFIER': '1'}
        shortcuts.write_bytes(subprocess.run([str(backend), '--gateway-export-shortcuts'], env=env, cwd=home, check=True, capture_output=True).stdout)
    subprocess.run([sys.executable, str(project / 'scripts/import_catalog.py'), str(source), '--shortcuts', str(shortcuts)], cwd=project, check=True)
    print('已更新 internal/gateway/catalog.json、docs/coverage.json 与 internal/gateway/guidance/。\n'
          '下一步：核对 git diff 与覆盖台账；运行 go run ./cmd/gateway --render-skill > skills/feishu-gateway/SKILL.md；'
          '完成四项验证后，在部署配置中更新 catalog_revision。')


if __name__ == '__main__':
    main()
