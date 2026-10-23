#!/usr/bin/env python3
"""从已固定的 lark-cli 源码快照生成目录；不执行 CLI，不请求飞书。"""
import argparse
import json
import subprocess
from collections import Counter
from pathlib import Path


def field_schema(field):
    kind = field.get('type', 'object' if field.get('properties') else 'string')
    kind = {'int': 'integer', 'int64': 'integer', 'float': 'number', 'bool': 'boolean'}.get(kind, kind)
    if kind not in ('object', 'array', 'integer', 'number', 'boolean', 'string'):
        raise ValueError('unsupported field type: ' + kind)
    result = {'type': kind}
    if field.get('description'):
        result['description'] = field['description']
    if field.get('enum'):
        vals = field['enum']
        if kind in ('integer', 'number'):
            vals = [float(x) if kind == 'number' else int(x) for x in vals]
        result['enum'] = vals
    for source, target in [('min', 'minimum'), ('max', 'maximum')]:
        if kind in ('integer', 'number') and field.get(source) not in (None, ''):
            result[target] = float(field[source])
    children = field.get('properties')
    if children:
        child = object_schema(children)
        if kind == 'array':
            result['items'] = child
        else:
            result.update(child)
    elif kind == 'array':
        result['items'] = {}
    return result


def object_schema(fields):
    result = {'type': 'object', 'properties': {k: field_schema(v) for k, v in sorted(fields.items())}, 'additionalProperties': False}
    required = [k for k, v in sorted(fields.items()) if v.get('required')]
    if required:
        result['required'] = required
    return result


RESOURCE_BINDINGS = {
    'wiki.spaces.get': {'params.space_id': 'wiki_space'},
    'wiki.nodes.list': {'params.space_id': 'wiki_space'},
    'wiki.members.list': {'params.space_id': 'wiki_space'},
    'im.chats.get': {'params.chat_id': 'chat'},
    'im.chat.members.get': {'params.chat_id': 'chat'},
    'im.chat.moderation.get': {'params.chat_id': 'chat'},
    'im.pins.list': {'params.chat_id': 'chat'},
    'drive.files.list': {'params.folder_token': 'folder'},
    'drive.files.create_folder': {'data.folder_token': 'folder'},
}


def walk(resources, path=()):
    for name, resource in sorted(resources.items()):
        for method, value in sorted(resource.get('methods', {}).items()):
            yield (*path, name, method), value
        yield from walk(resource.get('resources', {}), (*path, name))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('--output', type=Path, default=Path('internal/gateway/catalog.json'))
    parser.add_argument('--coverage', type=Path, default=Path('docs/coverage.json'))
    args = parser.parse_args()
    commit = subprocess.check_output(['git', '-C', str(args.source), 'rev-parse', 'HEAD'], text=True).strip()
    operations, coverage = [], []
    for source in sorted((args.source / 'internal/registry/catalog/services').glob('*.json')):
        service = json.loads(source.read_text())
        for path, method in walk(service['resources']):
            op_id = '.'.join((service['name'], *path))
            item = {'id': op_id, 'source_commit': commit, 'status': 'excluded', 'reason': ''}
            coverage.append(item)
            if 'tenant' not in method.get('accessTokens', []):
                item['reason'] = 'user_only'
                continue
            if method.get('risk') not in ('read', 'write', 'high-risk-write'):
                item['reason'] = 'risk_unknown'
                continue
            if method['risk'] == 'high-risk-write':
                item['reason'] = 'high_risk_not_enabled_in_v0'
                continue
            try:
                props, required = {}, []
                for section, fields in [('params', method.get('parameters', {})), ('data', method.get('requestBody', {}))]:
                    if fields:
                        props[section] = object_schema(fields)
                        if props[section].get('required'):
                            required.append(section)
                if method['risk'] != 'read':
                    props['idempotency_key'] = {'type': 'string', 'minLength': 1, 'maxLength': 128, 'description': '同一次写入重试必须复用此键；不同参数不能复用。'}
                    required.append('idempotency_key')
                props['async'] = {'type': 'boolean', 'description': '登记后台 Job 后返回；通过 feishu_job_get 查询。'}
                schema = {'type': 'object', 'properties': props, 'additionalProperties': False}
                if required:
                    schema['required'] = required
                output = object_schema(method.get('responseBody', {}))
                # 上游允许扩展返回字段，不能因快照滞后丢失数据。
                def loosen(obj):
                    if isinstance(obj, dict):
                        obj.pop('required', None)
                        if obj.get('type') == 'object':
                            obj['additionalProperties'] = True
                        for v in obj.values():
                            loosen(v)
                    elif isinstance(obj, list):
                        for v in obj:
                            loosen(v)
                loosen(output)
                if not method.get('responseBody'):
                    output = {'anyOf': [output, {'type': 'null'}]}
            except (ValueError, TypeError) as exc:
                item['reason'] = str(exc)
                continue
            if 'page_token' in props.get('params', {}).get('properties', {}):
                props['params']['properties']['page_token']['description'] = '使用 Gateway 上次返回的分页标识；不要填入原始飞书 token。'
            operations.append({'id': op_id, 'tool': 'feishu_' + op_id.replace('.', '_'), 'description': method['description'], 'pack': service['name'] + ('.read' if method['risk'] == 'read' else '.write'), 'risk': method['risk'], 'command': [service['name'], *path], 'input_schema': schema, 'data_schema': output, 'app_scopes': method.get('scopes', []), 'resource_bindings': RESOURCE_BINDINGS.get(op_id, {}), 'doc_url': method.get('docUrl', '')})
            item.update(status='adapter_ready', reason='generic_json; live_unverified')
    envelope = {'source_commit': commit, 'operations': operations}
    for target, data in [(args.output, envelope), (args.coverage, coverage)]:
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'methods': len(coverage), 'enabled_adapters': len(operations), 'coverage': dict(Counter(x['reason'] for x in coverage))}, ensure_ascii=False))


if __name__ == '__main__':
    main()
