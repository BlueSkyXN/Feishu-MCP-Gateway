#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""本地 HTTP Gateway 与模拟取票响应；不访问真实飞书或读取个人凭据。"""

import contextlib
import email.message
import importlib.util
import io
import json
import socket
from pathlib import Path
import tempfile
import threading
import unittest
from unittest import mock
import urllib.error
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


SPEC = importlib.util.spec_from_file_location('request_client', Path(__file__).with_name('request_client.py'))
client = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(client)

GATEWAY_KEY = 'fixture-gateway-key'
APP_ID = 'cli_fixture_app'
APP_TOKEN = 'fixture-application-token'
APP_SECRET = 'fixture-local-only-secret'


def invocation_id():
    """与网关 newID 同构的回执编号：inv_ + base32 文本。"""
    return 'inv_ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'


class Response(io.BytesIO):
    def __init__(self, value, status=200):
        super().__init__(value if isinstance(value, bytes) else json.dumps(value).encode())
        self.status = status
        self.headers = email.message.Message()
        self.headers['Content-Type'] = 'application/json'

    def getcode(self):
        return self.status


class TokenOpener:
    """Intercept only fixed token origins; other traffic must remain local."""

    def __init__(self, origin, reply=None, status=200):
        self.origin = origin
        self.reply = {'token_type': 'Bearer', 'access_token': APP_TOKEN} if reply is None else reply
        self.status = status
        self.requests = []
        self.local = urllib.request.build_opener(client.NoRedirect(), urllib.request.ProxyHandler({}))

    def open(self, request, timeout):
        if request.full_url in client.TOKEN_ENDPOINTS.values():
            self.requests.append(request)
            if self.status >= 400:
                raise urllib.error.HTTPError(request.full_url, self.status,
                                             APP_SECRET, email.message.Message(),
                                             io.BytesIO(APP_SECRET.encode()))
            return Response(self.reply)
        parsed = urllib.parse.urlsplit(request.full_url)
        if parsed.scheme + '://' + parsed.netloc != self.origin:
            raise AssertionError('Test unexpectedly attempted a nonlocal request')
        return self.local.open(request, timeout=timeout)


@contextlib.contextmanager
def gateway(**options):
    state = {'requests': [], **options}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            raw = self.rfile.read(int(self.headers.get('Content-Length', '0')))
            message = json.loads(raw)
            state['requests'].append({
                'path': self.path, 'headers': {k.lower(): v for k, v in self.headers.items()},
                'body': message,
            })
            if state.get('redirect') and self.path != '/unexpected-target':
                self.send_response(302)
                self.send_header('Location', state['origin'] + '/unexpected-target')
                self.send_header('Content-Length', '0')
                self.end_headers()
                return
            method = message.get('method')
            code = 200
            extra_headers = {}
            if method == 'initialize':
                reply = {
                    'jsonrpc': '2.0', 'id': message['id'],
                    'result': {
                        'protocolVersion': state.get('protocol', client.PROTOCOL_VERSION),
                        'capabilities': {'tools': {}},
                        'serverInfo': {'name': 'local-gateway-fixture', 'version': 'test'},
                    },
                }
                if state.get('session'):
                    extra_headers['Mcp-Session-Id'] = state['session']
            elif method == 'notifications/initialized':
                self.send_response(202)
                self.send_header('Content-Length', '0')
                self.end_headers()
                return
            else:
                code = state.get('tool_status', 200)
                reply = {'jsonrpc': '2.0', 'id': message['id'], 'result': state.get('tool_result', {
                    'structuredContent': {'status': 'succeeded', 'data': {'fixture': True}},
                    'content': [],
                })}
                if state.get('rpc_error'):
                    reply = {'jsonrpc': '2.0', 'id': message['id'], 'error': {
                        'code': -32000, 'message': APP_SECRET + APP_TOKEN + GATEWAY_KEY,
                    }}
                if code >= 400:
                    reply = {'message': APP_SECRET + APP_TOKEN + GATEWAY_KEY}
            encoded = state.get('response_bytes', json.dumps(reply).encode())
            self.send_response(code)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(encoded)))
            for name, value in extra_headers.items():
                self.send_header(name, value)
            self.end_headers()
            try:
                self.wfile.write(encoded)
            except (BrokenPipeError, ConnectionResetError):
                pass

    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    state['origin'] = 'http://127.0.0.1:' + str(server.server_port)
    thread = threading.Thread(target=server.serve_forever, kwargs={'poll_interval': 0.01}, daemon=True)
    thread.start()
    try:
        yield state['origin'], state
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


class RequestClientTest(unittest.TestCase):
    def invoke(self, endpoint, environ=None, flags=(), opener=None):
        output, errors = io.StringIO(), io.StringIO()
        env = {'FEISHU_GATEWAY_KEY': GATEWAY_KEY} if environ is None else environ
        with contextlib.ExitStack() as stack:
            stack.enter_context(contextlib.redirect_stdout(output))
            stack.enter_context(contextlib.redirect_stderr(errors))
            if opener is not None:
                stack.enter_context(mock.patch.object(client.urllib.request, 'build_opener', return_value=opener))
            code = client.main(['--endpoint', endpoint, *flags], env)
        return code, output.getvalue(), errors.getvalue()

    def assert_private_error(self, result):
        code, output, error = result
        self.assertNotEqual(code, 0)
        self.assertTrue(error)
        for value in (APP_SECRET, APP_TOKEN, GATEWAY_KEY):
            self.assertNotIn(value, error)
            self.assertNotIn(value, output)
        self.assertNotIn('Traceback', error)

    def assert_failure_diagnostics(self, result, status=None, kind=None):
        """失败输出：stdout 为一个经过校验的诊断 JSON，stderr 为固定中文说明。"""
        code, output, error = result
        self.assertEqual(code, 1)
        self.assertTrue(error)
        diagnostics = json.loads(output)
        self.assertIsInstance(diagnostics, dict)
        # 诊断键只能是白名单字段。
        allowed = {'status', 'invocation_id', 'job_id', 'operation_id', 'error',
                   'files', 'http_status', 'missing_scopes', 'retry_after_seconds',
                   'log_id', 'upstream_code'}
        self.assertTrue(set(diagnostics) <= allowed, diagnostics)
        if status is not None:
            self.assertEqual(diagnostics.get('status'), status)
        if kind is not None:
            self.assertEqual(diagnostics.get('error', {}).get('kind'), kind)
        self.assert_private_error(result)
        return diagnostics

    def test_context_without_upstream_credentials_and_handshake(self):
        with gateway() as (origin, state):
            result = self.invoke(origin)
        self.assertEqual(result[0], 0, result[2])
        self.assertEqual(json.loads(result[1]), {'status': 'succeeded', 'data': {'fixture': True}})
        self.assertEqual(result[2], '')
        requests = state['requests']
        self.assertEqual([r['body']['method'] for r in requests],
                         ['initialize', 'notifications/initialized', 'tools/call'])
        self.assertTrue(all(r['path'] == '/mcp' for r in requests))
        self.assertEqual(requests[0]['body']['params']['protocolVersion'], '2025-11-25')
        self.assertNotIn('id', requests[1]['body'])
        self.assertEqual(requests[2]['body']['params'], {'name': 'feishu_context', 'arguments': {}})
        for index, request in enumerate(requests):
            self.assertEqual(request['headers']['authorization'], 'Bearer ' + GATEWAY_KEY)
            self.assertNotIn('x-feishu-access-token', request['headers'])
            self.assertNotIn('x-feishu-app-id', request['headers'])
            if index:
                self.assertEqual(request['headers']['mcp-protocol-version'], '2025-11-25')

    def test_existing_token_wins_over_secret_and_arguments_route_are_forwarded(self):
        arguments = {'operation_id': 'wiki.spaces.list', 'arguments': {'params': {'page_size': 5}}}
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'arguments.json'
            path.write_text(json.dumps(arguments), encoding='utf-8')
            opener = TokenOpener(origin)
            result = self.invoke(origin + '/mcp', {
                'FEISHU_GATEWAY_KEY': GATEWAY_KEY, 'FEISHU_APP_ID': APP_ID,
                'FEISHU_ACCESS_TOKEN': APP_TOKEN, 'FEISHU_APP_SECRET': APP_SECRET,
            }, ('--tool', 'feishu_execute_read', '--arguments-file', str(path), '--route', 'example-route'), opener)
        self.assertEqual(result[0], 0, result[2])
        self.assertEqual(opener.requests, [])
        self.assertEqual(state['requests'][-1]['body']['params'], {'name': 'feishu_execute_read', 'arguments': arguments})
        for request in state['requests']:
            self.assertEqual(request['headers']['x-feishu-app-id'], APP_ID)
            self.assertEqual(request['headers']['x-feishu-access-token'], APP_TOKEN)
            self.assertEqual(request['headers']['x-feishu-gateway-route'], 'example-route')
            self.assertFalse(any(name.startswith('x-qoderwake') for name in request['headers']))
        self.assertNotIn(APP_SECRET, json.dumps(state['requests']))

    def test_secret_only_goes_to_fixed_token_endpoint(self):
        for brand in ('feishu', 'lark'):
            with self.subTest(brand=brand), gateway() as (origin, state):
                opener = TokenOpener(origin)
                result = self.invoke(origin, {
                    'FEISHU_GATEWAY_KEY': GATEWAY_KEY, 'FEISHU_APP_ID': APP_ID,
                    'FEISHU_APP_SECRET': APP_SECRET, 'FEISHU_BRAND': brand,
                }, opener=opener)
                self.assertEqual(result[0], 0, result[2])
                self.assertEqual(len(opener.requests), 1)
                issued = opener.requests[0]
                self.assertEqual(issued.full_url, client.TOKEN_ENDPOINTS[brand])
                self.assertEqual(urllib.parse.parse_qs(issued.data.decode()), {
                    'grant_type': ['client_credentials'], 'client_id': [APP_ID], 'client_secret': [APP_SECRET],
                })
                self.assertNotIn('authorization', {key.lower() for key, _ in issued.header_items()})
                for request in state['requests']:
                    self.assertEqual(request['headers']['authorization'], 'Bearer ' + GATEWAY_KEY)
                    self.assertEqual(request['headers']['x-feishu-access-token'], APP_TOKEN)
                self.assertNotIn(APP_SECRET, json.dumps(state['requests']))
                self.assertNotIn(APP_SECRET, result[1] + result[2])

    def test_dpop_token_is_rejected_before_contacting_gateway(self):
        with gateway() as (origin, state):
            opener = TokenOpener(origin, {'token_type': 'DPoP', 'access_token': APP_TOKEN})
            result = self.invoke(origin, {
                'FEISHU_GATEWAY_KEY': GATEWAY_KEY, 'FEISHU_APP_ID': APP_ID, 'FEISHU_APP_SECRET': APP_SECRET,
            }, opener=opener)
        self.assert_private_error(result)
        self.assertIn('DPoP', result[2])
        self.assertEqual(len(opener.requests), 1)
        self.assertEqual(state['requests'], [])

    def test_invalid_token_responses_are_not_accepted(self):
        responses = [
            {'access_token': APP_TOKEN},
            {'token_type': 'MAC', 'access_token': APP_TOKEN},
            {'token_type': 'Bearer', 'access_token': 'Bearer ' + APP_TOKEN},
            {'token_type': 'Bearer', 'access_token': ''},
            {'token_type': 'Bearer', 'access_token': APP_TOKEN, 'error': APP_SECRET},
            {'code': 100, 'message': APP_SECRET}, b'not-json-' + APP_SECRET.encode(), [],
        ]
        with gateway() as (origin, state):
            for response in responses:
                with self.subTest(response_type=type(response).__name__):
                    result = self.invoke(origin, {
                        'FEISHU_GATEWAY_KEY': GATEWAY_KEY, 'FEISHU_APP_ID': APP_ID, 'FEISHU_APP_SECRET': APP_SECRET,
                    }, opener=TokenOpener(origin, response))
                    self.assert_private_error(result)
            self.assertEqual(state['requests'], [])

    def test_token_http_failure_is_generic(self):
        with gateway() as (origin, state):
            opener = TokenOpener(origin, status=401)
            result = self.invoke(origin, {
                'FEISHU_GATEWAY_KEY': GATEWAY_KEY, 'FEISHU_APP_ID': APP_ID, 'FEISHU_APP_SECRET': APP_SECRET,
            }, opener=opener)
            self.assert_private_error(result)
            self.assertEqual(state['requests'], [])

    def test_gateway_http_failure_does_not_retry_tool(self):
        with gateway(tool_status=503) as (origin, state):
            result = self.invoke(origin, flags=('--tool', 'feishu_execute_write'))
        self.assert_private_error(result)
        self.assertEqual([r['body']['method'] for r in state['requests']].count('tools/call'), 1)

    def test_rpc_error_is_failure_and_tool_is_called_once(self):
        with gateway(rpc_error=True) as (origin, state):
            result = self.invoke(origin)
        self.assert_failure_diagnostics(result, status='failed', kind='rpc_error')
        self.assertEqual(len(state['requests']), 3)

    def test_tool_and_business_errors_are_not_success(self):
        cases = [
            {'isError': True, 'structuredContent': {'status': 'failed', 'error': {'message': APP_SECRET}}},
            {'structuredContent': {'status': 'succeeded', 'error': {'message': APP_TOKEN}}},
            *({'structuredContent': {'status': status}} for status in ('failed', 'unknown', 'partial', 'cancelled')),
        ]
        for tool_result in cases:
            status = tool_result.get('structuredContent', {}).get('status')
            with self.subTest(status=status):
                with gateway(tool_result=tool_result) as (origin, state):
                    result = self.invoke(origin)
                # 失败保留真实状态（succeeded 伴有 error 时按 succeeded 报告），
                # 错误 message 一律不回显；回执编号缺失时不伪造。
                diagnostics = self.assert_failure_diagnostics(result)
                self.assertIn(diagnostics.get('status'), (status, 'failed'))
                self.assertNotIn('invocation_id', diagnostics)
                self.assertEqual(len(state['requests']), 3)

    def test_single_text_json_business_result_is_supported(self):
        with gateway(tool_result={'content': [{'type': 'text', 'text': '{"status":"succeeded","data":42}'}]}) as (origin, _):
            result = self.invoke(origin)
        self.assertEqual(result[0], 0, result[2])
        self.assertEqual(json.loads(result[1])['data'], 42)

    def test_failure_keeps_valid_receipt_and_gateway_generated_ids(self):
        invocation = 'inv_ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
        cases = [
            # isError 且带 structuredContent：回执编号、job、operation、files 全保留。
            {'isError': True, 'structuredContent': {
                'status': 'unknown', 'invocation_id': invocation,
                'job_id': invocation, 'operation_id': 'wiki.spaces.get',
                'files': [{'file_id': 'f_ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'}],
                'error': {'kind': 'upstream_timeout', 'upstream_code': 124,
                          'log_id': 'log_20261117', 'missing_scopes': ['wiki:wiki:readonly'],
                          'retry_after_seconds': 30},
            }},
            # 单一 JSON 文本结果同样保留。
            {'content': [{'type': 'text', 'text': json.dumps({
                'status': 'partial', 'invocation_id': invocation,
                'error': {'kind': 'upstream_error'},
            })}]},
        ]
        for tool_result in cases:
            with self.subTest(channel='structured' if 'structuredContent' in tool_result else 'text'):
                with gateway(tool_result=tool_result) as (origin, state):
                    result = self.invoke(origin)
                diagnostics = self.assert_failure_diagnostics(result)
                self.assertEqual(diagnostics.get('status'),
                                 tool_result.get('structuredContent', {}).get('status')
                                 or json.loads(tool_result['content'][0]['text'])['status'])
                self.assertEqual(diagnostics.get('invocation_id'), invocation)
                if 'files' in str(tool_result):
                    self.assertEqual(diagnostics.get('files'),
                                     [{'file_id': 'f_ABCDEFGHIJKLMNOPQRSTUVWXYZ234567',
                                       'uri': 'feishu-file://files/f_ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'}])
                self.assertEqual(diagnostics['error']['kind'],
                                 'upstream_timeout' if 'upstream_timeout' in str(tool_result) else 'upstream_error')
                self.assertEqual([r['body']['method'] for r in state['requests']].count('tools/call'), 1)

    def test_invalid_ids_and_unknown_fields_are_dropped(self):
        bad = [
            {'structuredContent': {'status': 'failed', 'invocation_id': 'inv_!!!'}},
            {'structuredContent': {'status': 'failed', 'invocation_id': APP_TOKEN}},
            {'structuredContent': {'status': 'failed', 'job_id': 'job_short'}},
            {'structuredContent': {'status': 'failed', 'invocation_id': 'inv_with-hyphen'}},
            {'structuredContent': {'status': 'failed', 'invocation_id': 'inv_lowercaseabcdefghijklmnopqrstuvwxyz'}},
            {'structuredContent': {'status': 'failed', 'operation_id': '../etc/passwd'}},
            {'structuredContent': {'status': 'failed', 'error': {'kind': 'made_up_kind'}}},
            {'structuredContent': {'status': 'failed', 'error': {'kind': 'UPSTREAM_TIMEOUT'}}},
            {'structuredContent': {'status': 'failed', 'log_id': 'log with spaces' + APP_SECRET}},
            {'structuredContent': {'status': 'failed', 'missing_scopes': ['a'] * 33}},
            {'structuredContent': {'status': 'failed', 'retry_after_seconds': -5}},
            {'structuredContent': {'status': 'failed', 'retry_after_seconds': float('inf') if False else 10 ** 9}},
            {'structuredContent': {'status': 'failed', 'upstream_code': True}},
            # 未知/危险字段一律不复制。
            {'structuredContent': {'status': 'failed', 'data': {'secret': APP_SECRET}, 'meta': {'x': APP_TOKEN}}},
        ]
        for tool_result in bad:
            with self.subTest(case=tool_result.get('structuredContent', tool_result)):
                with gateway(tool_result=tool_result) as (origin, _):
                    result = self.invoke(origin)
                diagnostics = self.assert_failure_diagnostics(result)
                self.assertNotIn('invocation_id', diagnostics)
                self.assertNotIn('data', diagnostics)
                self.assertNotIn('meta', diagnostics)

    def test_credentials_embedded_in_valid_shaped_fields_are_filtered(self):
        invocation = 'inv_ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
        # 格式看似合法的 ID/log/scope 字段承载假凭据：字段整体省略，不泄露完整秘密值。
        tool_result = {'isError': True, 'structuredContent': {
            'status': 'failed', 'invocation_id': invocation,
            'log_id': APP_TOKEN, 'operation_id': APP_SECRET,
            'missing_scopes': [GATEWAY_KEY], 'files': [{'file_id': APP_TOKEN}],
            'error': {'kind': 'upstream_error', 'log_id': APP_SECRET},
        }}
        with gateway(tool_result=tool_result) as (origin, _):
            result = self.invoke(origin, environ={
                'FEISHU_GATEWAY_KEY': GATEWAY_KEY,
                'FEISHU_APP_ID': APP_ID, 'FEISHU_ACCESS_TOKEN': APP_TOKEN,
                'FEISHU_APP_SECRET': APP_SECRET,
            })
        diagnostics = self.assert_failure_diagnostics(result)
        self.assertEqual(diagnostics.get('invocation_id'), invocation)
        self.assertNotIn('log_id', diagnostics)
        self.assertNotIn('operation_id', diagnostics)
        self.assertNotIn('missing_scopes', diagnostics)
        self.assertNotIn('files', diagnostics)
        self.assertNotIn('log_id', diagnostics.get('error', {}))
        self.assertEqual(diagnostics['error']['kind'], 'upstream_error')

    def test_http_and_malformed_failures_do_not_fabricate_receipts(self):
        with gateway(tool_status=503) as (origin, state):
            result = self.invoke(origin)
        diagnostics = self.assert_failure_diagnostics(result)
        self.assertEqual(diagnostics.get('status'), 'failed')
        self.assertNotIn('invocation_id', diagnostics)
        self.assertEqual(diagnostics.get('error', {}).get('kind'), 'http_error')
        self.assertEqual(diagnostics.get('http_status'), 503)
        # HTTP 错误体中即使带合法形状的 ID 也不回显。
        with gateway(tool_status=503, response_bytes=json.dumps(
                {'invocation_id': invocation_id(), 'detail': APP_SECRET}).encode()) as (origin, _):
            result = self.invoke(origin)
        diagnostics = self.assert_failure_diagnostics(result)
        self.assertNotIn('invocation_id', diagnostics)
        # 畸形 JSON 响应：固定类别，无伪造字段。
        with gateway(response_bytes=b'not json ' + APP_SECRET.encode()) as (origin, _):
            result = self.invoke(origin)
        self.assert_failure_diagnostics(result, status='failed', kind='invalid_response')

    def test_invalid_environment_fails_without_network_requests(self):
        cases = [
            {}, {'FEISHU_GATEWAY_KEY': GATEWAY_KEY + '\n'},
            {'FEISHU_ACCESS_TOKEN': APP_TOKEN}, {'FEISHU_APP_SECRET': APP_SECRET},
            {'FEISHU_APP_ID': APP_ID}, {'FEISHU_BRAND': 'unconfigured'},
            {'FEISHU_APP_ID': APP_ID, 'FEISHU_ACCESS_TOKEN': 'Bearer ' + APP_TOKEN},
            {'FEISHU_APP_ID': APP_ID + '\r\n', 'FEISHU_ACCESS_TOKEN': APP_TOKEN},
        ]
        with gateway() as (origin, state):
            for index, extra in enumerate(cases):
                with self.subTest(case=index):
                    env = {} if index == 0 else {'FEISHU_GATEWAY_KEY': GATEWAY_KEY, **extra}
                    result = self.invoke(origin, env)
                    self.assert_private_error(result)
            self.assertEqual(state['requests'], [])

    def test_endpoint_rejects_nonlocal_http_and_embedded_information(self):
        bad = [
            'http://example.com/mcp', 'ftp://127.0.0.1/mcp',
            'https://name:' + APP_SECRET + '@example.com/mcp',
            'http://127.0.0.1/mcp?token=' + APP_TOKEN, 'https://example.com/mcp?',
            'https://example.com/mcp#fragment', 'https://example.com/mcp#',
            'http://127.0.0.1:65536/mcp', 'http://127.0.0.1:0/mcp',
            'https://example.com/\n', 'https:///mcp',
        ]
        for endpoint in bad:
            with self.subTest(endpoint_kind=endpoint.split(':')[0]):
                self.assert_private_error(self.invoke(endpoint))
        self.assertEqual(client.normalize_endpoint('http://[::1]:18790'), 'http://[::1]:18790/mcp')
        self.assertEqual(client.normalize_endpoint('http://localhost:18790/'), 'http://localhost:18790/mcp')
        self.assertEqual(client.normalize_endpoint('https://example.com/custom/mcp'), 'https://example.com/custom/mcp')

    def test_gateway_redirect_is_not_followed(self):
        with gateway(redirect=True) as (origin, state):
            result = self.invoke(origin + '/redirect')
        self.assert_private_error(result)
        self.assertEqual(len(state['requests']), 1)
        self.assertEqual(state['requests'][0]['path'], '/redirect')

    def test_transport_has_no_redirect_or_environment_proxy(self):
        with mock.patch.dict(client.os.environ, {'HTTPS_PROXY': 'http://unreachable.example:81', 'HTTP_PROXY': 'http://unreachable.example:81'}):
            transport = client.HTTPTransport()
        self.assertTrue(any(isinstance(h, client.NoRedirect) for h in transport.opener.handlers))
        self.assertFalse(any(isinstance(h, urllib.request.ProxyHandler) and h.proxies for h in transport.opener.handlers))

    def test_response_size_is_bounded(self):
        with gateway(response_bytes=b'x' * 4097) as (origin, state):
            with mock.patch.object(client, 'MAX_GATEWAY_RESPONSE_BYTES', 4096):
                result = self.invoke(origin)
        self.assert_private_error(result)
        self.assertEqual(len(state['requests']), 1)

    def test_invalid_arguments_fail_before_network(self):
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'arguments.json'
            for raw in ('[]', '{"value":NaN}', APP_SECRET, '{"missing":'):
                path.write_text(raw, encoding='utf-8')
                self.assert_private_error(self.invoke(origin, flags=('--arguments-file', str(path))))
            self.assertEqual(state['requests'], [])

    def test_unsupported_protocol_prevents_tool_call(self):
        with gateway(protocol='2099-01-01') as (origin, state):
            result = self.invoke(origin)
        self.assert_private_error(result)
        self.assertEqual(len(state['requests']), 1)

    def test_session_header_is_forwarded_after_initialize(self):
        with gateway(session='fixture-session') as (origin, state):
            result = self.invoke(origin)
        self.assertEqual(result[0], 0, result[2])
        self.assertNotIn('mcp-session-id', state['requests'][0]['headers'])
        for request in state['requests'][1:]:
            self.assertEqual(request['headers']['mcp-session-id'], 'fixture-session')

    def caller_file(self, directory, **overrides):
        caller = {'channel': 'feishu', 'id': 'example-user', 'name': '示例用户',
                  'open_id': 'ou_example', 'union_id': 'on_example'}
        caller.update(overrides)
        path = Path(directory) / 'caller.json'
        path.write_text(json.dumps(caller, ensure_ascii=False), encoding='utf-8')
        return path

    def test_caller_file_qoderwake_headers(self):
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            result = self.invoke(origin, flags=(
                '--caller-adapter', 'qoderwake', '--caller-file', str(self.caller_file(directory)), '--route', 'r1'))
        self.assertEqual(result[0], 0, result[2])
        headers = state['requests'][2]['headers']
        self.assertEqual(headers['x-qoderwake-atwaker-sender-channel'], 'feishu')
        self.assertEqual(headers['x-qoderwake-atwaker-sender-id'], 'example-user')
        # 中文姓名按 URI component 编码一次。
        self.assertEqual(headers['x-qoderwake-atwaker-sender-name'],
                         urllib.parse.quote('示例用户', safe=''))
        self.assertEqual(headers['x-qoderwake-atwaker-sender-open-id'], 'ou_example')
        self.assertEqual(headers['x-qoderwake-atwaker-sender-union-id'], 'on_example')
        self.assertEqual(headers['x-feishu-gateway-route'], 'r1')

    def test_caller_file_standard_headers_adapter(self):
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            path = self.caller_file(directory, name='名字+加号 a/b', channel='lark')
            result = self.invoke(origin, flags=(
                '--caller-adapter', 'headers', '--caller-file', str(path)))
        self.assertEqual(result[0], 0, result[2])
        headers = state['requests'][2]['headers']
        self.assertEqual(headers['x-caller-channel'], 'lark')
        self.assertEqual(headers['x-caller-id'], 'example-user')
        # 加号与斜杠都要被编码，+ 不能变成空格。
        self.assertEqual(headers['x-caller-name'], urllib.parse.quote('名字+加号 a/b', safe=''))
        self.assertEqual(headers['x-caller-open-id'], 'ou_example')
        self.assertEqual(headers['x-caller-union-id'], 'on_example')
        self.assertFalse(any(name.startswith('x-qoderwake') for name in headers))

    def test_caller_file_rejects_invalid_payloads(self):
        invalid = [
            '{"channel":"feishu","id":"u","name":"n","open_id":"o","union_id":"u2",}',  # 尾逗号
            '{"channel":"feishu","id":"u","name":"n","open_id":"o","union_id":"u2","extra":"x"}',
            '{"channel":"feishu","id":"u","name":"n","open_id":"o","union_id":"u2","id":"dup"}',
            '{"channel":"feishu","name":"n","open_id":"o","union_id":"u2"}',  # 缺 id
            '{"channel":"feishu","id":"","name":"n","open_id":"o","union_id":"u2"}',
            '{"id":"u","name":"n","open_id":"o","union_id":"u2"}',  # 缺 channel
            '{"channel":"feishu","id":"has space","name":"n","open_id":"o","union_id":"u2"}',
            '{"channel":"feishu","id":"comma,x","name":"n","open_id":"o","union_id":"u2"}',
            '{"channel":"feishu","id":"co:lon","name":"n","open_id":"o","union_id":"u2"}',
            '{"channel":"feishu","id":"' + 'a' * 129 + '","name":"n","open_id":"o","union_id":"u2"}',
            '{"channel":"' + 'c' * 65 + '","id":"u","name":"n","open_id":"o","union_id":"u2"}',
            '{"channel":"feishu","id":123,"name":"n","open_id":"o","union_id":"u2"}',
            '{"channel":"feishu","id":"u","name":123,"open_id":"o","union_id":"u2"}',  # name 类型错误
            '{"channel":"feishu","id":"u","name":"n","open_id":"o","union_id":"u2","channel":"dup"}',
            '[]', 'null', 'not-json',
        ]
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            for raw in invalid:
                with self.subTest(raw=raw[:40]):
                    path = Path(directory) / 'caller.json'
                    path.write_text(raw, encoding='utf-8')
                    result = self.invoke(origin, flags=(
                        '--caller-adapter', 'headers', '--caller-file', str(path)))
                    self.assert_private_error(result)
            self.assertEqual(state['requests'], [])

    def test_caller_file_rejects_size_limit_and_missing_pair(self):
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'caller.json'
            path.write_text(json.dumps({'channel': 'feishu', 'id': 'u', 'name': 'x' * 17000,
                                        'open_id': 'o', 'union_id': 'u2'}), encoding='utf-8')
            self.assert_private_error(self.invoke(origin, flags=(
                '--caller-adapter', 'headers', '--caller-file', str(path))))
            # 只给 adapter 不给文件（以及反过来）都是用法错误。
            self.assert_private_error(self.invoke(origin, flags=('--caller-adapter', 'headers')))
            self.assert_private_error(self.invoke(origin, flags=('--caller-file', str(path))))
            self.assertEqual(state['requests'], [])

    def test_context_only_never_reads_or_sends_upstream_credentials(self):
        environ = {'FEISHU_GATEWAY_KEY': GATEWAY_KEY, 'FEISHU_APP_ID': APP_ID,
                   'FEISHU_ACCESS_TOKEN': APP_TOKEN, 'FEISHU_APP_SECRET': APP_SECRET}
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            opener = TokenOpener(origin)
            flags = ('--context-only', '--caller-adapter', 'qoderwake',
                     '--caller-file', str(self.caller_file(directory)), '--route', 'example-route')
            result = self.invoke(origin, environ, flags, opener=opener)
        self.assertEqual(result[0], 0, result[2])
        self.assertEqual(opener.requests, [])  # 不取票
        self.assertEqual(state['requests'][2]['body']['params'],
                         {'name': 'feishu_context', 'arguments': {}})
        for request in state['requests']:
            self.assertNotIn('x-feishu-access-token', request['headers'])
            self.assertNotIn('x-feishu-app-id', request['headers'])
            self.assertEqual(request['headers']['x-qoderwake-atwaker-sender-id'], 'example-user')
            self.assertEqual(request['headers']['x-feishu-gateway-route'], 'example-route')

    def test_caller_minimal_fields_and_optional_omission(self):
        """R01：channel/id 足够；可选字段缺省或空字符串省略 Header。"""
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            minimal = Path(directory) / 'minimal.json'
            minimal.write_text('{"channel":"dingtalk","id":"example-staff"}', encoding='utf-8')
            result = self.invoke(origin, flags=(
                '--caller-adapter', 'qoderwake', '--caller-file', str(minimal)))
            self.assertEqual(result[0], 0, result[2])
            headers = state['requests'][2]['headers']
            self.assertEqual(headers['x-qoderwake-atwaker-sender-channel'], 'dingtalk')
            self.assertEqual(headers['x-qoderwake-atwaker-sender-id'], 'example-staff')
            for absent in ('x-qoderwake-atwaker-sender-name',
                           'x-qoderwake-atwaker-sender-open-id',
                           'x-qoderwake-atwaker-sender-union-id'):
                self.assertNotIn(absent, headers)
            # 可选字段为空字符串同样省略。
            empties = Path(directory) / 'empties.json'
            empties.write_text('{"channel":"feishu","id":"u","name":"","open_id":"","union_id":""}',
                               encoding='utf-8')
            result = self.invoke(origin, flags=(
                '--caller-adapter', 'headers', '--caller-file', str(empties)))
            self.assertEqual(result[0], 0, result[2])
            headers = state['requests'][5]['headers']
            self.assertEqual(headers['x-caller-channel'], 'feishu')
            self.assertEqual(headers['x-caller-id'], 'u')
            for absent in ('x-caller-name', 'x-caller-open-id', 'x-caller-union-id'):
                self.assertNotIn(absent, headers)

    def test_caller_channel_scopes_and_boundaries(self):
        """R01：qoderwake 五渠道放行；headers 通用渠道放行；边界拒绝且无网络请求。"""
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            # qoderwake 渠道：全部五个合法。
            for channel in ('feishu', 'lark', 'dingtalk', 'dingtalk-user', 'wecom-bot'):
                path = Path(directory) / 'caller.json'
                path.write_text(json.dumps({'channel': channel, 'id': 'staff-1'}),
                                encoding='utf-8')
                result = self.invoke(origin, flags=(
                    '--caller-adapter', 'qoderwake', '--caller-file', str(path)))
                self.assertEqual(result[0], 0, f'{channel}: {result[2]}')
            # headers 适配器：任意符合格式的通用渠道（例 slack、crm），不受 QoderWake 白名单限制。
            for channel in ('slack', 'crm'):
                path = Path(directory) / 'caller.json'
                path.write_text(json.dumps({'channel': channel, 'id': 'U123'}),
                                encoding='utf-8')
                result = self.invoke(origin, flags=(
                    '--caller-adapter', 'headers', '--caller-file', str(path)))
                self.assertEqual(result[0], 0, f'{channel}: {result[2]}')
            # qoderwake + 非白名单渠道在取票/请求前被拒。
            before = len(state['requests'])
            path = Path(directory) / 'bad.json'
            path.write_text('{"channel":"slack","id":"u"}', encoding='utf-8')
            self.assert_private_error(self.invoke(origin, flags=(
                '--caller-adapter', 'qoderwake', '--caller-file', str(path))))
            # 三个拒绝用例都不产生任何请求。
            self.assertEqual(len(state['requests']), before)
            # 64/65 字节 channel 边界（headers 适配器，非白名单渠道）。
            path.write_text(json.dumps({'channel': 'c' * 64, 'id': 'u'}), encoding='utf-8')
            result = self.invoke(origin, flags=(
                '--caller-adapter', 'headers', '--caller-file', str(path)))
            self.assertEqual(result[0], 0, result[2])
            self.assertEqual(state['requests'][-1]['headers']['x-caller-channel'], 'c' * 64)
            path.write_text(json.dumps({'channel': 'c' * 65, 'id': 'u'}), encoding='utf-8')
            self.assert_private_error(self.invoke(origin, flags=(
                '--caller-adapter', 'headers', '--caller-file', str(path))))
            # 128/129 字节 ID 边界。
            path.write_text(json.dumps({'channel': 'feishu', 'id': 'i' * 128}), encoding='utf-8')
            result = self.invoke(origin, flags=(
                '--caller-adapter', 'qoderwake', '--caller-file', str(path)))
            self.assertEqual(result[0], 0, result[2])
            path.write_text(json.dumps({'channel': 'feishu', 'id': 'i' * 129}), encoding='utf-8')
            self.assert_private_error(self.invoke(origin, flags=(
                '--caller-adapter', 'qoderwake', '--caller-file', str(path))))
            # 后续只有两次成功调用（各 3 请求）；三个拒绝不产生请求。
            self.assertEqual(len(state['requests']), before + 6)

    def test_caller_invalid_name_omitted_but_identity_kept(self):
        """R01：name 超限或含控制字符时省略显示名，主身份仍发送。"""
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'caller.json'
            # UTF-8 后超过 1024 字节的显示名。
            path.write_text(json.dumps({'channel': 'feishu', 'id': 'u',
                                        'name': '名' * 513}, ensure_ascii=False), encoding='utf-8')
            result = self.invoke(origin, flags=(
                '--caller-adapter', 'qoderwake', '--caller-file', str(path)))
            self.assertEqual(result[0], 0, result[2])
            headers = state['requests'][2]['headers']
            self.assertEqual(headers['x-qoderwake-atwaker-sender-id'], 'u')
            self.assertNotIn('x-qoderwake-atwaker-sender-name', headers)
            # 1024 字节以内（含中文、加号、斜杠、百分号）按一次编码发送。
            path.write_text(json.dumps({'channel': 'feishu', 'id': 'u',
                                        'name': '名字+加/号%25'}, ensure_ascii=False), encoding='utf-8')
            result = self.invoke(origin, flags=(
                '--caller-adapter', 'qoderwake', '--caller-file', str(path)))
            self.assertEqual(result[0], 0, result[2])
            headers = state['requests'][5]['headers']
            self.assertEqual(headers['x-qoderwake-atwaker-sender-name'],
                             urllib.parse.quote('名字+加/号%25', safe=''))

    def test_caller_invalid_names_omit_but_call_completes(self):
        """F1：孤立 surrogate 与 C0/DEL/C1 控制字符显示名只省略 name，调用照常完成。"""
        invalid_names = [
            '{"channel":"feishu","id":"u","name":"\\ud800"}',  # 孤立高代理
            '{"channel":"feishu","id":"u","name":"\\udfff"}',  # 孤立低代理
            '{"channel":"feishu","id":"u","name":"a\\u0000b"}',   # NUL
            '{"channel":"feishu","id":"u","name":"a\\u001fb"}',   # C0
            '{"channel":"feishu","id":"u","name":"a\\u007fb"}',   # DEL
            '{"channel":"feishu","id":"u","name":"a\\u0080b"}',   # C1 起
            '{"channel":"feishu","id":"u","name":"a\\u0085b"}',   # NEL（C1）
            '{"channel":"feishu","id":"u","name":"a\\u009fb"}',   # C1 止
        ]
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            for index, raw in enumerate(invalid_names):
                with self.subTest(case=index):
                    path = Path(directory) / 'caller.json'
                    path.write_text(raw, encoding='utf-8')
                    result = self.invoke(origin, flags=(
                        '--caller-adapter', 'qoderwake', '--caller-file', str(path)))
                    # 完整握手仍发生，退出 0。
                    self.assertEqual(result[0], 0, result[2])
                    requests = state['requests'][3 * (index + 1) - 3:3 * (index + 1)]
                    self.assertEqual([r['body']['method'] for r in requests],
                                     ['initialize', 'notifications/initialized', 'tools/call'])
                    headers = requests[2]['headers']
                    self.assertEqual(headers['x-qoderwake-atwaker-sender-id'], 'u')
                    self.assertEqual(headers['x-qoderwake-atwaker-sender-channel'], 'feishu')
                    self.assertNotIn('x-qoderwake-atwaker-sender-name', headers)
            # 合法显示名（中文/加号/斜杠/百分号）仍一次编码发送。
            path = Path(directory) / 'caller.json'
            path.write_text('{"channel":"feishu","id":"u","name":"名字+加/号%25"}', encoding='utf-8')
            result = self.invoke(origin, flags=(
                '--caller-adapter', 'qoderwake', '--caller-file', str(path)))
            self.assertEqual(result[0], 0, result[2])
            headers = state['requests'][-1]['headers']
            self.assertEqual(headers['x-qoderwake-atwaker-sender-name'],
                             urllib.parse.quote('名字+加/号%25', safe=''))

    def test_local_argument_file_errors_are_client_error(self):
        """F2：本地参数文件的格式/编码/数值错误归 client_error，零网络请求。"""
        payloads = [
            b'{"missing":',                      # 损坏 JSON
            b'{"value":NaN}',                    # 非标准数值
            b'\xff\xfe invalid utf8',           # 无效 UTF-8
            b'[]',                               # 不是对象
            b'null',                             # 不是对象
        ]
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            for raw in payloads:
                with self.subTest(payload=raw[:20]):
                    path = Path(directory) / 'args.json'
                    path.write_bytes(raw)
                    result = self.invoke(origin, flags=('--tool', 'feishu_execute_read',
                                                         '--arguments-file', str(path)))
                    diagnostics = self.assert_failure_diagnostics(result, kind='client_error')
                    self.assertNotIn('invocation_id', diagnostics)
            self.assertEqual(state['requests'], [])
        # 远端畸形响应继续 invalid_response。
        with gateway(response_bytes=b'not json') as (origin, _):
            result = self.invoke(origin)
        self.assert_failure_diagnostics(result, kind='invalid_response')

    def test_caller_validation_precedes_any_token_request(self):
        """R01：caller 文件非法时不取票、不访问 Gateway。"""
        environ = {'FEISHU_GATEWAY_KEY': GATEWAY_KEY, 'FEISHU_APP_ID': APP_ID,
                   'FEISHU_APP_SECRET': APP_SECRET}
        opener = TokenOpener('http://127.0.0.1:9')  # origin 不匹配即 AssertionError
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'caller.json'
            path.write_text('{"channel":"feishu","id":"bad id"}', encoding='utf-8')
            result = self.invoke(origin, environ, (
                '--caller-adapter', 'qoderwake', '--caller-file', str(path)), opener=opener)
            self.assert_private_error(result)
            self.assertEqual(opener.requests, [])
            self.assertEqual(state['requests'], [])

    def assert_single_json_stdout(self, result):
        """R02：失败 stdout 恰好一个 JSON 文档（JSONDecoder 全量解析）。"""
        code, output, error = result
        self.assertEqual(code, 1)
        self.assertTrue(output.strip())
        decoder = json.JSONDecoder()
        text = output.strip()
        value, end = decoder.raw_decode(text)
        self.assertEqual(text[end:].strip(), '', 'stdout contained more than one JSON document')
        self.assertIsInstance(value, dict)
        return value

    def test_network_refusal_and_timeout_are_classified(self):
        """R02：真实 socket 连接拒绝归 network_error；模拟超时同样归类。"""
        # 真实连接拒绝：绑定后立刻关闭的本地端口。
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        result = self.invoke(f'http://127.0.0.1:{port}')
        diagnostics = self.assert_single_json_stdout(result)
        self.assertEqual(diagnostics.get('error', {}).get('kind'), 'network_error')
        self.assertNotIn('invocation_id', diagnostics)
        self.assertNotIn('http_status', diagnostics)
        # 模拟超时：让 opener 抛 URLError，post 的真实分类逻辑仍在场。
        class TimeoutOpener:
            def open(self, request, timeout):
                raise urllib.error.URLError(socket.timeout('timed out'))
        class TimeoutTransport(client.HTTPTransport):
            def __init__(self):
                self.opener = TimeoutOpener()
        with mock.patch.object(client, 'HTTPTransport', TimeoutTransport):
            with gateway() as (origin, _):
                result = self.invoke(origin)
        diagnostics = self.assert_single_json_stdout(result)
        self.assertEqual(diagnostics.get('error', {}).get('kind'), 'network_error')

    def test_failure_classification_matrix(self):
        """R02：各失败形态的正确类别、http_status 与单 JSON 输出。"""
        cases = [
            ('http_4xx', {'tool_status': 403}, 'http_error', 403),
            ('http_5xx', {'tool_status': 503}, 'http_error', 503),
            # 被拒绝的重定向保留真实 302；rpc/畸形响应发生时确有 200 响应，可保留。
            ('redirect', {'redirect': True}, 'http_error', 302),
            ('rpc_error', {'rpc_error': True}, 'rpc_error', 200),
            ('malformed', {'response_bytes': b'not json'}, 'invalid_response', 200),
            ('protocol', {'protocol': '2099-01-01'}, 'invalid_response', None),
        ]
        for name, options, kind, status in cases:
            with self.subTest(case=name), gateway(**options) as (origin, state):
                result = self.invoke(origin)
                diagnostics = self.assert_single_json_stdout(result)
                self.assertEqual(diagnostics.get('error', {}).get('kind'), kind, name)
                if status is None:
                    self.assertNotIn('http_status', diagnostics, name)
                else:
                    self.assertEqual(diagnostics.get('http_status'), status, name)
                self.assertEqual(diagnostics.get('status'), 'failed', name)
                self.assertNotIn('invocation_id', diagnostics, name)
                # 重定向只发一次请求；其余在失败后不重试业务。
                self.assertLessEqual(
                    [r['body']['method'] for r in state['requests']].count('tools/call'), 1)

    def test_response_size_limit_is_invalid_response(self):
        with gateway(response_bytes=b'x' * 4097) as (origin, _):
            with mock.patch.object(client, 'MAX_GATEWAY_RESPONSE_BYTES', 4096):
                result = self.invoke(origin)
        diagnostics = self.assert_single_json_stdout(result)
        self.assertEqual(diagnostics.get('error', {}).get('kind'), 'invalid_response')

    def test_rate_limit_diagnostics_are_preserved(self):
        """R02/F2a：rate_limit 是已登记 kind，retry_after_seconds 保留。"""
        tool_result = {'structuredContent': {
            'status': 'failed',
            'error': {'kind': 'rate_limit', 'retry_after_seconds': 30.5},
        }}
        with gateway(tool_result=tool_result) as (origin, state):
            result = self.invoke(origin)
        diagnostics = self.assert_single_json_stdout(result)
        self.assertEqual(diagnostics['status'], 'failed')
        self.assertEqual(diagnostics['error']['kind'], 'rate_limit')
        self.assertEqual(diagnostics['error']['retry_after_seconds'], 30.5)
        self.assertEqual([r['body']['method'] for r in state['requests']].count('tools/call'), 1)

    def test_result_retention_errors_keep_kind_and_receipt(self):
        for kind in ('result_storage_failed', 'result_recovery_required', 'result_expired'):
            with self.subTest(kind=kind):
                receipt = {'status': 'failed', 'error': {'kind': kind}}
                if kind != 'result_recovery_required':
                    receipt['invocation_id'] = invocation_id()
                with gateway(tool_result={'isError': True, 'structuredContent': receipt}) as (origin, state):
                    result = self.invoke(origin)
                diagnostics = self.assert_failure_diagnostics(result, status='failed', kind=kind)
                self.assertEqual(diagnostics.get('invocation_id'), receipt.get('invocation_id'))
                self.assertEqual([r['body']['method'] for r in state['requests']].count('tools/call'), 1)

    def test_local_configuration_failures_use_client_error(self):
        """R02：本地配置/文件问题归 client_error，仍输出单 JSON。"""
        with gateway() as (origin, state):
            result = self.invoke(origin, environ={})
            diagnostics = self.assert_single_json_stdout(result)
            self.assertEqual(diagnostics.get('error', {}).get('kind'), 'client_error')
            self.assertEqual(state['requests'], [])

    def test_usage_combinations_exit_2_before_network(self):
        """R02：命令组合错误退出 2、无 stdout、无任何网络请求。"""
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            caller = Path(directory) / 'caller.json'
            caller.write_text('{"channel":"feishu","id":"u"}', encoding='utf-8')
            arguments = Path(directory) / 'args.json'
            arguments.write_text('{}', encoding='utf-8')
            for flags in (
                ('--context-only', '--tool', 'feishu_execute_read'),
                ('--context-only', '--arguments-file', str(arguments)),
                ('--caller-adapter', 'qoderwake'),
                ('--caller-file', str(caller)),
            ):
                with self.subTest(flags=flags):
                    output, errors = io.StringIO(), io.StringIO()
                    env = {'FEISHU_GATEWAY_KEY': GATEWAY_KEY}
                    with contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
                        code = client.main(['--endpoint', origin, *flags], env)
                    self.assertEqual(code, 2)
                    self.assertEqual(output.getvalue(), '')
                    self.assertTrue(errors.getvalue().strip())
            # 组合错误的四种形态都没有产生请求。
            self.assertEqual(state['requests'], [])
            # --context-only --tool feishu_context 可以使用（组合合法，不退出 2）。
            result = self.invoke(origin, flags=('--context-only', '--tool', 'feishu_context'))
            self.assertEqual(result[0], 0, result[2])
            self.assertEqual(len(state['requests']), 3)

    def test_context_only_never_reads_upstream_environment(self):
        """R03：context-only 不读取上游环境变量（访问即抛错的 spy environ）。"""
        class ExplodingEnv(dict):
            """读取上游变量即失败的环境：证明 context-only 不触碰它们。"""

            def get(self, key, default=None):
                if key in ('FEISHU_APP_ID', 'FEISHU_APP_SECRET',
                           'FEISHU_ACCESS_TOKEN', 'FEISHU_BRAND'):
                    raise AssertionError(f'context-only read upstream variable {key}')
                return super().get(key, default)

        env = ExplodingEnv({'FEISHU_GATEWAY_KEY': GATEWAY_KEY})
        with gateway() as (origin, state):
            output, errors = io.StringIO(), io.StringIO()
            with contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
                code = client.main(['--endpoint', origin, '--context-only'], env)
            self.assertEqual(code, 0, errors.getvalue())
            for request in state['requests']:
                self.assertNotIn('x-feishu-access-token', request['headers'])
                self.assertNotIn('x-feishu-app-id', request['headers'])

    def test_context_only_rejects_incompatible_combinations(self):
        with gateway() as (origin, state), tempfile.TemporaryDirectory() as directory:
            arguments = Path(directory) / 'args.json'
            arguments.write_text('{}', encoding='utf-8')
            for flags in (
                ('--context-only', '--tool', 'feishu_execute_read'),
                ('--context-only', '--arguments-file', str(arguments)),
            ):
                with self.subTest(flags=flags):
                    result = self.invoke(origin, flags=flags)
                    self.assert_private_error(result)
            self.assertEqual(state['requests'], [])

    def test_context_only_failure_keeps_diagnostics(self):
        tool_result = {'structuredContent': {'status': 'failed',
                                             'invocation_id': invocation_id(),
                                             'error': {'kind': 'actor_required'}}}
        with gateway(tool_result=tool_result) as (origin, _), tempfile.TemporaryDirectory() as directory:
            result = self.invoke(origin, flags=(
                '--context-only', '--caller-adapter', 'headers',
                '--caller-file', str(self.caller_file(directory))))
        diagnostics = self.assert_failure_diagnostics(result)
        self.assertEqual(diagnostics.get('error', {}).get('kind'), 'actor_required')
        self.assertIn('invocation_id', diagnostics)


if __name__ == '__main__':
    unittest.main()
