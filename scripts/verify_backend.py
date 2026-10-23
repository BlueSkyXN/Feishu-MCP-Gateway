#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""仅用 schema 和 dry-run 验证固定 CLI；使用临时 HOME 与占位应用凭据。"""
import argparse
import concurrent.futures
import hashlib
import json
import subprocess
import tempfile
from pathlib import Path


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('binary', type=Path)
    p.add_argument('--catalog', type=Path, default=Path('internal/gateway/catalog.json'))
    p.add_argument('--output', type=Path, default=Path('local/evidence/backend-validation.json'))
    args = p.parse_args()
    binary = args.binary.resolve()
    catalog = json.loads(args.catalog.read_text())
    with tempfile.TemporaryDirectory(prefix='gateway-backend-check-') as directory:
        env = {'HOME': directory, 'PATH': '/usr/bin:/bin', 'CI': '1', 'LARKSUITE_CLI_CONFIG_DIR': directory + '/config', 'LARKSUITE_CLI_NO_UPDATE_NOTIFIER': '1', 'LARKSUITE_CLI_NO_SKILLS_NOTIFIER': '1', 'LARKSUITE_CLI_APP_ID': 'cli_fixture', 'FEISHU_GATEWAY_WORKDIR': directory, 'LARKSUITE_CLI_AUTH_PROXY':'http://127.0.0.1:1','LARKSUITE_CLI_PROXY_KEY':'fixture-only', 'LARKSUITE_CLI_BRAND': 'feishu', 'LARKSUITE_CLI_DEFAULT_AS': 'bot', 'LARKSUITE_CLI_STRICT_MODE': 'bot'}
        def run(argv):
            return subprocess.run([str(binary), *argv], env=env, cwd=directory, capture_output=True, text=True, timeout=30)
        capabilities=json.loads(run(['--gateway-capabilities']).stdout)
        if capabilities['source_commit']!=catalog['source_commit']:raise SystemExit('backend source mismatch')
        exported=json.loads(run(['--gateway-export-shortcuts']).stdout)
        shortcuts={x['service']+'.'+x['command']:x for x in exported['shortcuts']}
        version = run(['--version'])
        def check(op):
            if op['kind']=='shortcut':
                row=shortcuts[op['id']]
                issues=[]
                if 'bot' not in row['auth_types'] or row['risk']!=op['risk']:issues.append('identity_or_risk_mismatch')
                declared={f['Name']:f for f in row['flags']}
                for binding in op['flag_bindings'].values():
                    f=declared.get(binding['name'])
                    if f is None or (f['Type'] or 'string')!=binding['type']:issues.append('flag_binding_mismatch')
                return {'operation':op['id'],'issues':issues}
            result = run(['schema', *op['command']])
            issues = []
            try:
                schema = json.loads(result.stdout)
                if result.returncode:
                    issues.append('schema_command_failed')
                if 'bot' not in schema.get('_meta', {}).get('access_tokens', []):
                    issues.append('bot_identity_missing')
                if set(op.get('file_fields',[]))!=set(schema.get('inputSchema',{}).get('properties',{}).get('file',{}).get('properties',{})):issues.append('file_fields_mismatch')
                actual = schema.get('inputSchema', {}).get('properties', {})
                for section in ('params', 'data'):
                    expected_fields = set(op['input_schema']['properties'].get(section, {}).get('properties', {}))
                    actual_fields = set(actual.get(section, {}).get('properties', {}))
                    if expected_fields != actual_fields:
                        issues.append(section + '_field_mismatch')
            except (ValueError, KeyError):
                issues.append('invalid_schema_json')
            return {'operation': op['id'], 'issues': issues}
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            results = list(pool.map(check, catalog['operations']))
        examples = [
            ['wiki', 'spaces', 'get', '--params', json.dumps({'space_id': 'fixture-space'})],
            ['drive', 'files', 'list', '--params', json.dumps({'folder_token': 'fixture-folder', 'page_size': 5})],
            ['drive', 'files', 'create_folder', '--data', json.dumps({'folder_token': 'fixture-folder', 'name': 'fixture'})],
        ]
        dry_runs = []
        for example in examples:
            result = run([*example, '--as', 'bot', '--dry-run'])
            try:
                value = json.loads(result.stdout)
            except ValueError:
                value = {}
            dry_runs.append({'command': example[:3], 'passed': result.returncode == 0 and value.get('ok') is True and value.get('identity') == 'bot' and value.get('dry_run') is True})
    failures = [r for r in results if r['issues']]
    report = {'cli_version': version.stdout.strip(), 'cli_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'catalog_source': catalog['source_commit'], 'definitions_checked': len(results), 'static_schemas_checked':sum(op['kind']=='api' for op in catalog['operations']), 'shortcuts_checked':sum(op['kind']=='shortcut' for op in catalog['operations']), 'failures': failures, 'dry_runs': dry_runs, 'live_requests': False}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({**report, 'cli_version': 'verified', 'cli_sha256': report['cli_sha256']},ensure_ascii=False))
    raise SystemExit(1 if failures or any(not r['passed'] for r in dry_runs) else 0)


if __name__ == '__main__':
    main()
