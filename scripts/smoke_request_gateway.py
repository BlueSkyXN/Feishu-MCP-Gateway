#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""以独立进程验证请求级凭据、路由解释和普通 MCP 客户端；不调用飞书。"""

import argparse
import hashlib
import json
import re
from pathlib import Path
import secrets
import socket
import sqlite3
import subprocess
import sys
import tempfile
import time
from urllib.error import URLError
from urllib.request import ProxyHandler, build_opener


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    path.chmod(0o600)


def captured(command, env, credentials, stage, success=True):
    result = subprocess.run(command, env=env, capture_output=True, text=True,
                            encoding='utf-8', timeout=30)
    require(not any(value in result.stdout or value in result.stderr for value in credentials),
            stage + ' exposed a fixture credential')
    require((result.returncode == 0) == success,
            stage + (' failed' if success else ' unexpectedly succeeded'))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('gateway', type=Path)
    parser.add_argument('cli', type=Path)
    parser.add_argument('--output', type=Path,
                        default=Path('local/evidence/request-process-smoke.json'))
    args = parser.parse_args()
    gateway, cli = args.gateway.resolve(), args.cli.resolve()
    repository = Path(__file__).resolve().parent.parent
    request_client = repository / 'scripts/request_client.py'
    # Construct child environments rather than inheriting any developer token,
    # App Secret or proxy configuration. All fixture credentials stay local.
    process_env = {'PATH': '/usr/bin:/bin', 'LANG': 'C.UTF-8',
                   'PYTHONIOENCODING': 'utf-8', 'PYTHONUTF8': '1'}
    revision = captured([str(gateway), '--catalog-revision'], process_env, (),
                        'catalog revision').stdout.strip()
    version = captured([str(gateway), '--version'], process_env, (),
                       'version').stdout.splitlines()[0].removeprefix('version=')
    cli_sha256 = hashlib.sha256(cli.read_bytes()).hexdigest()
    report = {'version': version, 'live_feishu_calls': 0}

    with tempfile.TemporaryDirectory(prefix='gateway-request-smoke-') as directory:
        root = Path(directory)
        key = secrets.token_urlsafe(32)
        token = 'fixture-' + secrets.token_urlsafe(32)
        credentials = (key, token)
        key_sha256 = hashlib.sha256(key.encode()).hexdigest()
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            headers_port = sock.getsockname()[1]
        base = f'http://127.0.0.1:{port}'

        config = json.loads((repository / 'examples/config.request-bearer.json').read_text())
        config.update({'catalog_revision': revision, 'listen': f'127.0.0.1:{port}',
                       'data_dir': str(root / 'data'), 'cli_binary': str(cli),
                       'cli_sha256': cli_sha256})
        config['apps'][0].update({'app_id': 'cli_request_fixture',
                                  'organization_id': 'fixture-organization'})
        config['clients'][0]['key_sha256'] = key_sha256
        require(all(app['credential_mode'] == 'request_bearer' and 'secret_env' not in app
                    for app in config['apps']), 'request example unexpectedly requires a secret')
        require(config['clients'][0]['caller_adapter']['kind'] == 'none',
                'ordinary client example unexpectedly requires caller metadata')
        # 第二个应用与启用 caller 适配和路由的第二接入：R04–R06 的实际 MCP 闭环载体。
        routed_key = secrets.token_urlsafe(32)
        credentials = (key, token, routed_key)
        config['apps'].append({'id': 'fixture-routed-app', 'organization_id': 'fixture-organization',
                               'app_id': 'cli_routed_fixture', 'brand': 'lark',
                               'credential_mode': 'request_bearer', 'dpop_mode': 'disabled'})
        config['clients'].append({
            'id': 'routed-client', 'app_connection_id': config['apps'][0]['id'],
            'key_sha256': hashlib.sha256(routed_key.encode()).hexdigest(),
            # actor 身份模式：caller 决定稳定 owner，用于 F5 的 actor/应用隔离断言。
            'identity_mode': 'actor', 'caller_adapter': {'kind': 'qoderwake'},
            'packs': ['wiki.read'], 'resource_mode': 'application_scope',
            'tool_view': 'compact',
            'routes': [
                # 两个合法 lark caller 都匹配 team-lark：T1 只换 actor、同应用路由的载体。
                {'id': 'team-lark', 'priority': 100,
                 'match': {'channels': ['lark'],
                           'ids': ['fixture-lark-user', 'fixture-lark-other']},
                 'app_connection_id': 'fixture-routed-app'},
                # 显式备用应用路由：同一 lark caller 也可显式选择默认应用（F5 应用隔离用）。
                {'id': 'team-primary-explicit', 'priority': 40,
                 'match': {'channels': ['lark'], 'ids': ['fixture-lark-user']},
                 'app_connection_id': config['apps'][0]['id']},
                {'id': 'team-ding', 'priority': 50,
                 'match': {'channels': ['dingtalk']},
                 'app_connection_id': config['apps'][0]['id']},
            ],
        })
        config_path = root / 'config.json'
        write_json(config_path, config)
        command = [str(gateway), '--config', str(config_path)]
        captured([*command, '--check'], process_env, credentials, 'request config check')
        report['config_check_without_app_secret'] = True

        routing = json.loads((repository / 'examples/config.routing.json').read_text())
        absent_backend = root / 'backend-that-must-not-run'
        routing.update({'catalog_revision': revision, 'data_dir': str(root / 'explain-data'),
                        'cli_binary': str(absent_backend), 'cli_sha256': '0' * 64})
        for number, app in enumerate(routing['apps']):
            app.update({'app_id': f'cli_routing_fixture_{number}',
                        'organization_id': 'fixture-organization'})
        routing_client = routing['clients'][0]
        routing_client['key_sha256'] = key_sha256
        routing_client['routes'][0]['match']['ids'] = ['fixture-team-b-user']
        selected_id = routing_client['routes'][0]['app_connection_id']
        selected_app = next(app for app in routing['apps'] if app['id'] == selected_id)
        routing_request = json.loads((repository / 'examples/request.routing.json').read_text())
        routing_request['headers'].update({
            'Authorization': 'Bearer ' + key,
            'X-QoderWake-AtWaker-Sender-Id': 'fixture-team-b-user',
            'X-QoderWake-AtWaker-Sender-Name': 'Fixture%20User+A%2BB',
            'X-QoderWake-AtWaker-Sender-Open-Id': 'ou_fixture',
            'X-QoderWake-AtWaker-Sender-Union-Id': 'on_fixture',
            'X-Feishu-App-Id': selected_app['app_id'],
            'X-Feishu-Access-Token': token,
        })
        routing_path, request_path = root / 'routing.json', root / 'request.json'
        write_json(routing_path, routing)
        write_json(request_path, routing_request)
        explanation = captured([str(gateway), '--config', str(routing_path),
                                '--explain-request', str(request_path)],
                               process_env, credentials, 'offline route explanation')
        explained = json.loads(explanation.stdout)
        require(explained['app_connection_id'] == selected_id
                and explained['route_id'] == routing_client['routes'][0]['id']
                and explained['credential_mode'] == 'request_bearer'
                and explained['upstream_credential_present'] is True,
                'route explanation selected the wrong application')
        require(explained['caller'] == {
            'source': 'qoderwake', 'channel': 'feishu', 'id': 'fixture-team-b-user',
            'id_type': 'user_id', 'name': 'Fixture User+A+B',
            'open_id': 'ou_fixture', 'union_id': 'on_fixture',
        } and explained['actor'] == '', 'route explanation did not preserve optional caller metadata')
        require(not absent_backend.exists() and not (root / 'explain-data').exists(),
                'offline route explanation started backend or storage')
        report.update({'offline_route_explanation': True, 'backend_not_required_for_explanation': True,
                       'caller_five_fields': True, 'uri_name_decoding': True,
                       'explanation_omits_credentials': True})

        # A07：标准 headers 适配器（X-Caller-*）由网关配置解释；ID 类型由服务端固定。
        headers_config = json.loads((repository / 'examples/config.caller-headers.json').read_text())
        headers_config.update({'catalog_revision': revision, 'data_dir': str(root / 'explain-headers-data'),
                               'cli_binary': str(absent_backend), 'cli_sha256': '0' * 64})
        headers_config['apps'][0].update({'app_id': 'cli_headers_fixture',
                                          'organization_id': 'fixture-organization'})
        headers_client = headers_config['clients'][0]
        headers_client['key_sha256'] = key_sha256
        headers_path = root / 'headers-config.json'
        write_json(headers_path, headers_config)
        headers_request = {'headers': {
            'Authorization': 'Bearer ' + key,
            'X-Caller-Channel': 'feishu',
            'X-Caller-Id': 'fixture-headers-user',
            'X-Caller-Name': '%E7%A4%BA%E4%BE%8B%E7%94%A8%E6%88%B7',
            'X-Caller-Open-Id': 'ou_fixture',
            'X-Caller-Union-Id': 'on_fixture',
        }}
        headers_request_path = root / 'headers-request.json'
        write_json(headers_request_path, headers_request)
        explanation = captured([str(gateway), '--config', str(headers_path),
                                '--explain-request', str(headers_request_path)],
                               process_env, credentials, 'offline headers-adapter explanation')
        explained = json.loads(explanation.stdout)
        require(explained['app_connection_id'] == headers_client['app_connection_id']
                and explained['credential_mode'] == 'request_bearer',
                'headers-adapter explanation selected the wrong application')
        require(explained['caller'] == {
            'source': 'headers', 'channel': 'feishu', 'id': 'fixture-headers-user',
            'id_type': 'external_id', 'name': '示例用户',
            'open_id': 'ou_fixture', 'union_id': 'on_fixture',
        }, 'headers adapter did not parse the five caller fields with URI-decoded name')
        report['headers_adapter_caller_fields'] = True

        client_count = 0

        def client_call(tool='feishu_context', arguments=None, with_token=False,
                        success=True, extra=(), env_extra=None,
                        gateway_key=None, app_id=None, token_value=None):
            """调用 Python 客户端；凭据用显式参数传入，避免“上一条换的 Token 忘了带”。

            参数只在本测试进程内使用；captured() 断言输出不含任何凭据值。
            """
            nonlocal client_count
            client_count += 1
            env = dict(process_env, FEISHU_GATEWAY_KEY=gateway_key or key)
            if with_token:
                env.update({'FEISHU_APP_ID': app_id or 'cli_request_fixture',
                            'FEISHU_ACCESS_TOKEN': token_value or token})
            if env_extra:
                env.update(env_extra)
            invocation = [sys.executable, str(request_client), '--endpoint', base, '--tool', tool]
            if arguments is not None:
                argument_path = root / f'arguments-{client_count}.json'
                write_json(argument_path, arguments)
                invocation += ['--arguments-file', str(argument_path)]
            invocation += list(extra)
            result = captured(invocation, env, credentials, 'client ' + tool, success)
            return json.loads(result.stdout) if success else result

        log_path = root / 'gateway.stderr.log'
        with log_path.open('w', encoding='utf-8') as log:
            process = subprocess.Popen(command, env=process_env, stdout=subprocess.DEVNULL, stderr=log)
            try:
                opener = build_opener(ProxyHandler({}))
                deadline = time.monotonic() + 15
                while True:
                    try:
                        with opener.open(base + '/healthz', timeout=1) as response:
                            require(json.load(response)['status'] == 'ok', 'health check failed')
                        break
                    except URLError:
                        require(process.poll() is None and time.monotonic() < deadline,
                                'request-mode gateway did not become healthy')
                        time.sleep(0.1)
                report['startup'] = True
                local_context = client_call()['data']
                require(local_context['credential_mode'] == 'request_bearer'
                        and local_context['upstream_credential_present'] is False
                        and local_context['identity_mode'] == 'client'
                        and local_context['actor'] == '' and not local_context['caller'].get('id'),
                        'ordinary local context unexpectedly required caller or upstream credentials')
                report['ordinary_client_without_caller_or_token'] = True
                # A05：context 返回所选应用的 app_id/brand/supports_async。
                require(local_context['app_id'] == 'cli_request_fixture'
                        and local_context['brand'] == 'feishu'
                        and local_context['supports_async'] is False,
                        'context metadata did not describe the selected application')
                report['context_metadata'] = True

                # A08：--context-only 存在上游环境变量时仍不取票、不发送上游凭据。
                caller_path = root / 'caller.json'
                write_json(caller_path, {'channel': 'feishu', 'id': 'fixture-context-user',
                                         'name': '示例用户', 'open_id': 'ou_fixture',
                                         'union_id': 'on_fixture'})
                # 客户端未配置该路由：initialize 即被网关拒绝。诊断只保留
                # http_error + 403，不伪造回执编号、不回显凭据。
                context_only = client_call(
                    extra=('--context-only', '--caller-adapter', 'qoderwake',
                           '--caller-file', str(caller_path), '--route', 'context-route'),
                    env_extra={'FEISHU_APP_ID': 'cli_request_fixture',
                               'FEISHU_ACCESS_TOKEN': token,
                               'FEISHU_APP_SECRET': 'fixture-unreachable-secret'},
                    success=False)
                context_diag = json.loads(context_only.stdout)
                require(context_diag.get('error', {}).get('kind') == 'http_error'
                        and context_diag.get('http_status') == 403
                        and 'invocation_id' not in context_diag,
                        'context-only produced an unexpected diagnostic contract')
                report['context_only_no_upstream_credentials'] = True
                # 本接入的 caller_adapter 为 none：caller Header 由网关忽略，
                # 路由与授权始终由服务端配置决定。caller 字段转发在下方
                # routing 配置（qoderwake）的离线解释里验证。
                plain_context_only = client_call(extra=('--context-only',),
                                                 env_extra={'FEISHU_APP_ID': 'cli_request_fixture',
                                                            'FEISHU_ACCESS_TOKEN': token})
                require(plain_context_only['data']['upstream_credential_present'] is False
                        and plain_context_only['data']['app_id'] == 'cli_request_fixture',
                        'context-only sent or read upstream credentials')
                report['context_only_skips_token_env'] = True

                described = client_call('feishu_tools_describe', {'operation_id': 'wiki.spaces.get'})
                require(described['data']['operation_id'] == 'wiki.spaces.get',
                        'configured Feishu operation was not discoverable')
                created = client_call('feishu_file_create', {'name': 'smoke.txt', 'text': 'request-mode smoke'})
                file_id = created['files'][0]['file_id']
                read = client_call('feishu_file_read', {'file_id': file_id})
                require(read['data']['text'] == 'request-mode smoke' and read['data']['eof'] is True,
                        'local file round-trip failed')
                report['local_file_round_trip_without_upstream_token'] = True

                token_context = client_call(with_token=True)['data']
                require(token_context['upstream_credential_present'] is True
                        and token_context['app_connection_id'] == config['apps'][0]['id'],
                        'request token was not recognized by the real gateway')
                report['fixture_token_recognized_without_disclosure'] = True

                denied = client_call('feishu_execute_read', {
                    'operation_id': 'wiki.spaces.get',
                    'arguments': {'params': {'space_id': 'fixture-space'}},
                }, success=False)
                require(denied.returncode == 1 and 'Gateway 工具调用' in denied.stderr,
                        'missing upstream token did not produce a tool failure')
                denied_diag = json.loads(denied.stdout)
                require(denied_diag.get('error', {}).get('kind') == 'upstream_credential_required'
                        and 'invocation_id' not in denied_diag,
                        'missing-token diagnostics missing error kind or fabricated a receipt')
                require(denied_diag.get('status') == 'failed', 'missing-token status was not failed')
                with sqlite3.connect(str(root / 'data/gateway.db')) as db:
                    reserved = db.execute('SELECT COUNT(*) FROM invocations').fetchone()[0]
                require(reserved == 0, 'missing-token operation reached invocation reservation')
                # Gateway reserves every upstream invocation before starting its
                # backend. Zero records demonstrates this probe stopped before
                # the backend/Feishu path; all successful calls above are local.
                report.update({'missing_token_client_exit': denied.returncode,
                               'missing_token_diagnostic_kind': 'upstream_credential_required',
                               'missing_token_rejected_before_invocation': True,
                               'upstream_invocations_reserved': reserved})

                # A09：更换 Token 后本地文件归属保持；新 Token 仍被识别。
                second_token = 'fixture-' + secrets.token_urlsafe(32)
                credentials = (key, token, second_token)
                swapped = client_call(with_token=True, env_extra={'FEISHU_ACCESS_TOKEN': second_token})
                require(swapped['data']['upstream_credential_present'] is True,
                        'second request token was not recognized')
                still_readable = client_call('feishu_file_read', {'file_id': file_id})
                require(still_readable['data']['text'] == 'request-mode smoke',
                        'local file ownership changed after swapping tokens')
                report['token_swap_keeps_local_files'] = True

                # R04：真实 Python → HTTP MCP → Gateway 的成功 caller/route 闭环。
                routed_caller = root / 'routed-caller.json'
                write_json(routed_caller, {'channel': 'lark', 'id': 'fixture-lark-user'})
                ding_caller = root / 'ding-caller.json'
                write_json(ding_caller, {'channel': 'dingtalk', 'id': 'fixture-staff'})
                # qoderwake 自动路由：lark 渠道命中 team-lark，选第二应用。
                lark_context = client_call(
                    extra=('--caller-adapter', 'qoderwake', '--caller-file', str(routed_caller)),
                    gateway_key=routed_key)['data']
                require(lark_context['caller'] == {
                    'source': 'qoderwake', 'channel': 'lark', 'id': 'fixture-lark-user',
                    'id_type': 'user_id',
                }, f'qoderwake caller fields mismatch: {lark_context["caller"]}')
                require(lark_context['actor'] == 'lark:user_id:fixture-lark-user'
                        and lark_context['identity_mode'] == 'actor',
                        'actor identity was not derived from the qoderwake caller')
                require(lark_context['route_id'] == 'team-lark'
                        and lark_context['app_connection_id'] == 'fixture-routed-app'
                        and lark_context['app_id'] == 'cli_routed_fixture'
                        and lark_context['brand'] == 'lark'
                        and lark_context['supports_async'] is False,
                        'auto-routed context selected the wrong application')
                report['r04_qoderwake_auto_route'] = True
                # qoderwake 覆盖非飞书渠道：dingtalk 命中 team-ding，选默认应用。
                ding_context = client_call(
                    extra=('--caller-adapter', 'qoderwake', '--caller-file', str(ding_caller)),
                    gateway_key=routed_key)['data']
                require(ding_context['caller'] == {
                    'source': 'qoderwake', 'channel': 'dingtalk', 'id': 'fixture-staff',
                    'id_type': 'staff_id',
                }, f'dingtalk caller fields mismatch: {ding_context["caller"]}')
                require(ding_context['actor'] == 'dingtalk:staff_id:fixture-staff',
                        'dingtalk actor key was not derived by the gateway mapping')
                require(ding_context['route_id'] == 'team-ding'
                        and ding_context['app_id'] == 'cli_request_fixture',
                        'dingtalk route selected the wrong application')
                report['r04_qoderwake_dingtalk_channel'] = True
                # 显式 route：lark caller + team-ding 不满足 match（渠道不符）被拒。
                denied_route = client_call(
                    extra=('--caller-adapter', 'qoderwake', '--caller-file', str(routed_caller),
                           '--route', 'team-ding'),
                    gateway_key=routed_key, success=False)
                denied_route_diag = json.loads(denied_route.stdout)
                require(denied_route_diag.get('http_status') == 403
                        and 'invocation_id' not in denied_route_diag,
                        'explicit route mismatch was not rejected at session level')
                report['r04_explicit_route_mismatch_denied'] = True
                # headers 适配器 + 通用渠道：需要网关侧 headers 接入。用第三接入
                # 在同一进程演示（见下方 headers-adapter 进程段）。

                # R05：Token A 创建、Token B 读取同一文件；caller/route 不变。
                token_a = 'fixture-' + secrets.token_urlsafe(32)
                token_b = 'fixture-' + secrets.token_urlsafe(32)
                credentials = (key, token, routed_key, token_a, token_b)
                route_args = ('--caller-adapter', 'qoderwake', '--caller-file', str(routed_caller))
                created_b = client_call('feishu_file_create',
                                        {'name': 'token-b.txt', 'text': 'owned-by-lark-route'},
                                        with_token=True, app_id='cli_routed_fixture',
                                        token_value=token_a, gateway_key=routed_key, extra=route_args)
                shared_file = created_b['files'][0]['file_id']
                ctx_a = client_call(with_token=True, app_id='cli_routed_fixture',
                                    token_value=token_a, gateway_key=routed_key,
                                    extra=route_args)['data']
                require(ctx_a['upstream_credential_present'] is True
                        and ctx_a['app_id'] == 'cli_routed_fixture',
                        'token A was not recognized on the routed application')
                # Token B 读取：实际读取调用携带 Token B（显式参数）。
                read_b = client_call('feishu_file_read', {'file_id': shared_file},
                                     with_token=True, app_id='cli_routed_fixture',
                                     token_value=token_b, gateway_key=routed_key, extra=route_args)
                require(read_b['data']['text'] == 'owned-by-lark-route',
                        'token B could not read the file created under token A')
                ctx_b = client_call(with_token=True, app_id='cli_routed_fixture',
                                    token_value=token_b, gateway_key=routed_key,
                                    extra=route_args)['data']
                require(ctx_b['upstream_credential_present'] is True,
                        'token B was not recognized')
                report['r05_token_a_create_token_b_read'] = True
                # 隔离：默认接入（无 caller）访问该 file_id 应被拒绝——owner 不同。
                other_read = client_call('feishu_file_read', {'file_id': shared_file},
                                         success=False)
                require(json.loads(other_read.stdout).get('error', {}).get('kind')
                        not in (None, 'client_error'),
                        'cross-client file access was not denied by ownership')
                report['r05_cross_client_isolated'] = True

                # F5：成功显式路由（actor 模式接入；备用应用路由 match 同一 caller）。
                explicit_route_args = route_args + ('--route', 'team-primary-explicit')
                explicit_ctx = client_call(with_token=True, app_id='cli_request_fixture',
                                           token_value=token_a, gateway_key=routed_key,
                                           extra=explicit_route_args)['data']
                require(explicit_ctx['route_id'] == 'team-primary-explicit'
                        and explicit_ctx['app_connection_id'] == config['apps'][0]['id']
                        and explicit_ctx['app_id'] == 'cli_request_fixture'
                        and explicit_ctx['brand'] == 'feishu'
                        and explicit_ctx['supports_async'] is False
                        and explicit_ctx['actor'] == 'lark:user_id:fixture-lark-user',
                        f'explicit route did not select the primary application: {explicit_ctx}')
                report['f5_explicit_route_success'] = True

                # T1：同应用 actor 隔离——同客户端、同应用连接、同真实 App ID、同组织、
                # 同有效凭据，只换 caller 的 id（lark 渠道，两个 id 都匹配 team-lark）。
                other_actor_caller = root / 'other-actor-caller.json'
                write_json(other_actor_caller, {'channel': 'lark', 'id': 'fixture-lark-other'})
                other_args = ('--caller-adapter', 'qoderwake', '--caller-file', str(other_actor_caller))
                actor_ctx = client_call(with_token=True, app_id='cli_routed_fixture',
                                        token_value=token_a, gateway_key=routed_key,
                                        extra=other_args)['data']
                # 上下文对照：客户端、应用连接、App ID 一致（组织由测试配置固定为
                # fixture-organization，两应用同组织），仅 actor 不同。
                require(actor_ctx['app_connection_id'] == ctx_a['app_connection_id']
                        and actor_ctx['app_id'] == ctx_a['app_id']
                        and actor_ctx['app_id'] == 'cli_routed_fixture'
                        and actor_ctx['route_id'] == 'team-lark'
                        and actor_ctx['actor'] != ctx_a['actor']
                        and actor_ctx['actor'] == 'lark:user_id:fixture-lark-other',
                        f'actor control context did not keep same app and change only actor: {actor_ctx}')
                other_read = client_call('feishu_file_read', {'file_id': shared_file},
                                         with_token=True, app_id='cli_routed_fixture',
                                         token_value=token_a, gateway_key=routed_key,
                                         extra=other_args, success=False)
                other_diag = json.loads(other_read.stdout)
                require(other_diag.get('error', {}).get('kind') == 'not_found'
                        and 'invocation_id' not in other_diag,
                        f'same-app cross-actor read was not a specific not_found: {other_diag}')
                # 原 actor 仍能读取同一文件（对照：拒绝确实来自 actor 隔离）。
                owner_read = client_call('feishu_file_read', {'file_id': shared_file},
                                         with_token=True, app_id='cli_routed_fixture',
                                         token_value=token_a, gateway_key=routed_key,
                                         extra=route_args)
                require(owner_read['data']['text'] == 'owned-by-lark-route',
                        'original actor could no longer read its own file')
                report['f5_actor_isolation_kind'] = 'not_found'
                report['t1_same_app_actor_isolation'] = True

                # F5：应用隔离——同一客户端、同一 actor，显式选另一应用读取原 file_id。
                app_isolated = client_call('feishu_file_read', {'file_id': shared_file},
                                           with_token=True, app_id='cli_request_fixture',
                                           token_value=token_a, gateway_key=routed_key,
                                           extra=explicit_route_args, success=False)
                app_diag = json.loads(app_isolated.stdout)
                # 先核对：actor 未变、应用已变（隔离由应用归属差异引起，而非 actor 变化）。
                app_check = client_call(with_token=True, app_id='cli_request_fixture',
                                        token_value=token_a, gateway_key=routed_key,
                                        extra=explicit_route_args)['data']
                require(app_check['actor'] == 'lark:user_id:fixture-lark-user'
                        and app_check['app_id'] == 'cli_request_fixture',
                        'application-isolation control context changed actor or app')
                require(app_diag.get('error', {}).get('kind') == 'not_found'
                        and 'invocation_id' not in app_diag,
                        f'cross-application read was not a specific not_found: {app_diag}')
                report['f5_same_actor_app_isolation_kind'] = 'not_found'

                # R06：登记后失败保留真实回执并可用 job_get 查回。
                probe_file = client_call('feishu_file_create',
                                         {'name': 'receipt-probe.txt', 'text': 'probe'},
                                         with_token=True, app_id='cli_routed_fixture',
                                         token_value=token_a, gateway_key=routed_key,
                                         extra=route_args)['files'][0]['file_id']
                fail_arguments = {
                    'operation_id': 'wiki.spaces.get',
                    'arguments': {'params': {'space_id': 'fixture-space'},
                                  'workspace_files': {'../receipt-probe.txt': probe_file}},
                }
                failure = client_call('feishu_execute_read', fail_arguments,
                                      with_token=True, app_id='cli_routed_fixture',
                                      token_value=token_a, gateway_key=routed_key,
                                      extra=route_args, success=False)
                failure_diag = json.loads(failure.stdout)
                require(failure_diag.get('invocation_id')
                        and failure_diag.get('operation_id') == 'wiki.spaces.get'
                        and failure_diag.get('status') == 'failed'
                        and failure_diag.get('error', {}).get('kind') == 'invalid_file_path',
                        f'post-reservation failure diagnostics incomplete: {failure_diag}')
                receipt_id = failure_diag['invocation_id']
                require(re.match(r'inv_[A-Z0-9]{16,64}', receipt_id),
                        'receipt id is not gateway-generated format')
                # job_get 查回：退出码按契约仍为 1，stdout 的回执才是判定依据。
                fetched = client_call('feishu_job_get', {'job_id': receipt_id},
                                      with_token=True, app_id='cli_routed_fixture',
                                      token_value=token_b, gateway_key=routed_key,
                                      extra=route_args, success=False)
                fetched_diag = json.loads(fetched.stdout)
                require(fetched_diag.get('invocation_id') == receipt_id
                        and fetched_diag.get('status') == 'failed'
                        and fetched_diag.get('error', {}).get('kind') == 'invalid_file_path',
                        f'job_get did not return the same receipt: {fetched_diag}')
                with sqlite3.connect(str(root / 'data/gateway.db')) as db:
                    row = db.execute('SELECT id,state FROM invocations WHERE id=?',
                                     (receipt_id,)).fetchone()
                require(row == (receipt_id, 'failed'),
                        f'sqlite record mismatch for receipt: {row}')
                report.update({'r06_post_reservation_receipt': receipt_id,
                               'r06_receipt_status': 'failed',
                               'r06_receipt_kind': 'invalid_file_path',
                               'r06_job_get_round_trip': True})
                # 缺 Token 的登记前失败仍零新增记录（此前 reserved==0 已断言）。

                # R04 补充：headers 适配器 + 通用渠道的真实 MCP 成功链路。
                headers_live_config = json.loads((repository / 'examples/config.caller-headers.json').read_text())
                headers_live_config.update({'catalog_revision': revision,
                                            'listen': f'127.0.0.1:{headers_port}',
                                            'data_dir': str(root / 'headers-live-data'),
                                            'cli_binary': str(cli), 'cli_sha256': cli_sha256})
                headers_live_config['apps'][0].update({'app_id': 'cli_headers_fixture',
                                                       'organization_id': 'fixture-organization'})
                headers_live_config['clients'][0]['key_sha256'] = key_sha256
                headers_live_path = root / 'headers-live-config.json'
                write_json(headers_live_path, headers_live_config)
                headers_live = subprocess.Popen(
                    [str(gateway), '--config', str(headers_live_path)], env=process_env,
                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                try:
                    headers_base = f'http://127.0.0.1:{headers_port}'
                    deadline = time.monotonic() + 15
                    while True:
                        try:
                            with opener.open(headers_base + '/healthz', timeout=1) as response:
                                require(json.load(response)['status'] == 'ok',
                                        'headers-adapter gateway health check failed')
                            break
                        except URLError:
                            require(headers_live.poll() is None and time.monotonic() < deadline,
                                    'headers-adapter gateway did not become healthy')
                            time.sleep(0.1)
                    slack_caller = root / 'slack-caller.json'
                    write_json(slack_caller, {'channel': 'slack', 'id': 'U123ABC'})
                    headers_args = ('--caller-adapter', 'headers', '--caller-file', str(slack_caller))
                    # headers_live 网关与主网关不同端口：直接用子进程调用。
                    invocation = [sys.executable, str(request_client), '--endpoint', headers_base]
                    invocation += list(headers_args)
                    result = captured(invocation, dict(process_env, FEISHU_GATEWAY_KEY=key),
                                      credentials, 'headers-adapter live context')
                    headers_context = json.loads(result.stdout)['data']
                    require(headers_context['caller'] == {
                        'source': 'headers', 'channel': 'slack', 'id': 'U123ABC',
                        'id_type': 'external_id',
                    }, f'headers adapter caller fields mismatch: {headers_context["caller"]}')
                    require(headers_context['app_connection_id']
                            == headers_live_config['clients'][0]['app_connection_id'],
                            'headers adapter selected the wrong application')
                    report['r04_headers_generic_channel'] = True
                finally:
                    if headers_live.poll() is None:
                        headers_live.terminate()
                    try:
                        headers_live.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        headers_live.kill()
                        headers_live.wait(timeout=5)

                # A03/A09：合法业务失败保留真实回执编号与错误类别。
                fail_args = root / 'fail-arguments.json'
                write_json(fail_args, {'operation_id': 'wiki.spaces.get',
                                       'arguments': {'params': {'space_id': 'fixture-space'},
                                                     'idempotency_key': 'smoke-fail-once'}})
                failure = client_call('feishu_execute_read', json.loads(fail_args.read_text()),
                                      with_token=True, success=False)
                failure_diag = json.loads(failure.stdout)
                require(failure_diag.get('status') == 'failed'
                        and failure_diag.get('error', {}).get('kind') not in (None, 'client_error'),
                        'failed call did not preserve a real error kind')
                if 'invocation_id' in failure_diag:
                    import re as _re
                    require(_re.fullmatch(r'inv_[A-Z0-9]{16,64}', failure_diag['invocation_id'])
                            is not None, 'failure diagnostics kept a malformed receipt')
                report['failure_preserves_error_kind'] = True
            finally:
                if process.poll() is None:
                    process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
        report['request_client_subprocesses'] = client_count
        require(process.returncode == 0, 'gateway did not shut down cleanly')
        require(not any(value in log_path.read_text(encoding='utf-8') for value in credentials),
                'gateway log exposed a fixture credential')
        report.update({'graceful_shutdown': True, 'credentials_absent_from_process_output': True,
                       'success': True})

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report, ensure_ascii=False))


if __name__ == '__main__':
    main()
