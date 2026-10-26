#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""从已固定的 lark-cli 源码快照生成目录和用法指南；不执行 CLI，不请求飞书。"""
import argparse
import json
import re
import subprocess
from collections import Counter
from pathlib import Path

import lark_guidance


def field_schema(field):
    kind = field.get('type', 'object' if field.get('properties') else 'string')
    kind = {'int': 'integer', 'int64': 'integer', 'float': 'number', 'bool': 'boolean', 'list':'array'}.get(kind, kind)
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


def file_ref():
    return {'type':'object','properties':{'file_id':{'type':'string','minLength':1}},'required':['file_id'],'additionalProperties':False}


def controls(props, required, risk):
    if risk != 'read':
        props['idempotency_key'] = {'type':'string','minLength':1,'maxLength':128,'description':'同一次写入重试必须复用此键；不同参数不能复用。'}
        required.append('idempotency_key')
    props['async'] = {'type':'boolean','description':'登记后台执行后返回 job_id。'}
    props['workspace_files'] = {'type':'object','additionalProperties':{'type':'string'},'description':'可选输入集合：相对名称到 file_id 的映射，在本次受控工作区的 inputs/ 下提供。'}


def shortcut_operation(row, source):
    props, required, bindings = {}, [], {}
    for field in row['flags'] or []:
        name, typ = field['Name'], field['Type'] or 'string'
        if name in ('as','profile','yes','dry-run','format','print-schema','flag-name','api-version') or (name=='json' and typ=='bool'):
            continue
        key=name.replace('-','_')
        if key in props or key in ('async','workspace_files'):
            raise ValueError('reserved or duplicate parameter: '+key)
        kind={'string':'string','bool':'boolean','int':'integer','float64':'number','string_array':'array','string_slice':'array','int_array':'array'}[typ]
        schema={'type':kind,'description':field['Desc']}
        if kind=='array': schema['items']={'type':'integer' if typ=='int_array' else 'string'}
        if field['Enum']: schema['enum']=field['Enum']
        input_sources=field['Input'] or []
        is_file=name in ('file','image','audio','video') or name.endswith('-file')
        is_output=name in ('output','output-path','output-dir','local-dir')
        if kind=='string' and (is_file or 'file' in input_sources): schema={'oneOf':[schema,file_ref()],'description':field['Desc']+'；本地输入请提供 {"file_id":"..."}。'}
        if kind=='array' and is_file: schema['items']={'oneOf':[schema['items'],file_ref()]}
        if is_output: schema['description']='受控工作区中的相对产物名称或目录；返回对应 file_id，不接受宿主机绝对路径。'
        json_schema=(row.get('flag_schemas') or {}).get(name)
        json_value=typ=='string' and (json_schema is not None or name=='json' or name.endswith('-json'))
        if json_value:
            variants=[{'type':'string'}]
            if json_schema: variants.append(json_schema)
            else: variants.extend([{'type':'object'},{'type':'array'}])
            if is_file or 'file' in input_sources:variants.append(file_ref())
            schema={'anyOf':variants,'description':field['Desc']+'；可直接传 JSON 对象或数组。'}
        props[key]=schema
        if field['Required']: required.append(key)
        bindings[key]={'name':name,'type':typ,'input':input_sources,'file':is_file,'output':is_output,'json':json_value}
    controls(props,required,row['risk'])
    service=row['service']; source_domain={'docs':'doc'}.get(service,service)
    prefixes=set()
    for f in (source/'shortcuts'/source_domain).rglob('*.go'):
        if f.name.endswith('_test.go'): continue
        prefixes.update('/open-apis/'+x+'/' for x in re.findall(r'/open-apis/([A-Za-z0-9_-]+)/',f.read_text()))
    prefixes.update({'/open-apis/drive/'})
    fallback={'base':'bitable','docs':'docs_ai','markdown':'drive','whiteboard':'board','note':'minutes','apps':'application'}
    prefixes.add('/open-apis/'+fallback.get(service,service)+'/')
    if service=='docs': prefixes.update({'/open-apis/docx/','/open-apis/wiki/','/open-apis/board/'})
    resource={}
    if service=='docs' and row['command'] in ('+fetch','+history-list','+history-revert-status') and 'doc' in props: resource={'doc':'document'}
    if service=='base' and row['command'] in ('+record-list','+record-get','+record-search','+record-upsert','+record-batch-create','+record-batch-update','+record-batch-delete') and 'base_token' in props: resource={'base_token':'base'}
    if service=='sheets' and row['command'] in ('+workbook-info','+sheet-list','+sheet-info','+cells-get','+csv-get','+cells-set','+cells-clear','+history-list','+revision-get') and 'spreadsheet_token' in props: resource={'spreadsheet_token':'spreadsheet'}
    if service=='im' and row['command'] in ('+chat-get','+chat-members-list') and 'chat_id' in props: resource={'chat_id':'chat'}
    if service=='wiki' and row['command'] in ('+node-list','+space-get') and 'space_id' in props: resource={'space_id':'wiki_space'}
    return {'id':service+'.'+row['command'],'tool':'feishu_'+service+'_'+row['command'].lstrip('+').replace('-','_'),'kind':'shortcut','description':row['description'],'pack':service+('.read' if row['risk']=='read' else '.write'),'risk':row['risk'],'command':[service,row['command']],'input_schema':{'type':'object','properties':props,'required':required,'additionalProperties':False},'data_schema':{},'app_scopes':row['app_scopes'] or [],'conditional_app_scopes':row['conditional_app_scopes'] or [],'resource_bindings':resource,'flag_bindings':bindings,'egress_prefixes':sorted(prefixes),'hidden':row['hidden'],'has_dry_run':row['has_dry_run'],'conditional_confirmation':any(f['Name']=='yes' for f in row['flags'] or [])}


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
    parser.add_argument('--shortcuts', type=Path)
    parser.add_argument('--guidance', type=Path, default=Path('internal/gateway/guidance'), help='用法指南输出目录；需要 --shortcuts')
    args = parser.parse_args()
    commit = subprocess.check_output(['git', '-C', str(args.source), 'rev-parse', 'HEAD'], text=True).strip()
    if subprocess.check_output(['git', '-C', str(args.source), 'status', '--porcelain'], text=True).strip():
        raise SystemExit('来源工作区有未提交修改，不能声称导入固定源码版本')
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
            try:
                props, required = {}, []
                file_fields={k:v for k,v in method.get('requestBody',{}).items() if v.get('type')=='file'}
                for section, fields in [('params', method.get('parameters', {})), ('data', method.get('requestBody', {}))]:
                    if section=='data': fields={k:v for k,v in fields.items() if k not in file_fields}
                    if fields:
                        props[section] = object_schema(fields)
                        if props[section].get('required'):
                            required.append(section)
                if file_fields:
                    needed=[k for k,v in file_fields.items() if v.get('required')]
                    props['files']={'type':'object','properties':{k:file_ref() for k in file_fields},'required':needed,'additionalProperties':False}
                    if needed: required.append('files')
                controls(props,required,method['risk'])
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
            operations.append({'id': op_id, 'tool': 'feishu_' + op_id.replace('.', '_'), 'kind':'api','method':method['httpMethod'],'path':service['servicePath'].rstrip('/')+'/'+method['path'].lstrip('/'),'file_fields':list(file_fields),'query_fields':[k for k,v in method.get('parameters',{}).items() if v.get('location')=='query'],'parameter_defaults':{k:v['default'] for k,v in method.get('parameters',{}).items() if 'default' in v},'description': method['description'], 'pack': service['name'] + ('.read' if method['risk'] == 'read' else '.write'), 'risk': method['risk'], 'command': [service['name'], *path], 'input_schema': schema, 'data_schema': output, 'app_scopes': method.get('scopes', []), 'resource_bindings': RESOURCE_BINDINGS.get(op_id, {}), 'doc_url': method.get('docUrl', '')})
            item.update(status='adapter_ready', reason='generic_json; live_unverified')
    rows = []
    if args.shortcuts:
        exported=json.loads(args.shortcuts.read_text())
        rows = exported['shortcuts']
        if exported['source_commit']!=commit: raise SystemExit('shortcut/source commit mismatch')
        for row in exported['shortcuts']:
            item={'id':row['service']+'.'+row['command'],'source_commit':commit,'status':'excluded','reason':''};coverage.append(item)
            if 'bot' not in (row['auth_types'] or []):item['reason']='user_only';continue
            if row['service']=='event' and row['command']=='+subscribe':item['reason']='deprecated_event_entry; use bounded event tools';continue
            try:op=shortcut_operation(row,args.source)
            except (ValueError,KeyError) as exc:item['reason']='schema_review_required: '+str(exc);continue
            operations.append(op);item.update(status='adapter_ready',reason='shortcut; live_unverified')
    # 描述来自 CLI 帮助：去掉用户身份说明，flag 写法改为本操作的 JSON 参数名。
    index = lark_guidance.Index(operations, coverage)
    for op in operations:
        lark_guidance.clean_operation(index, op)
    envelope = {'source_commit': commit, 'operations': operations}
    for target, data in [(args.output, envelope), (args.coverage, coverage)]:
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')
    summary = {'methods': len(coverage), 'enabled_adapters': len(operations), 'coverage': dict(Counter(x['reason'] for x in coverage))}
    if args.shortcuts:
        summary['guidance'] = lark_guidance.build(args.source, commit, operations, coverage, rows, args.guidance)
    print(json.dumps(summary, ensure_ascii=False))


if __name__ == '__main__':
    main()
