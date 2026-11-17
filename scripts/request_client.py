#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""普通 MCP 客户端样例：在客户端取应用 Bearer token，单次调用 Gateway。"""

import argparse
import http.client
import ipaddress
import json
import os
from pathlib import Path
import re
import sys
import urllib.error
import urllib.parse
import urllib.request


PROTOCOL_VERSION = '2025-11-25'
HTTP_TIMEOUT = 60
MAX_ARGUMENT_BYTES = 1 << 20
MAX_TOKEN_RESPONSE_BYTES = 1 << 20
MAX_GATEWAY_RESPONSE_BYTES = 4 << 20
MAX_CALLER_FILE_BYTES = 16 << 10
TOKEN_ENDPOINTS = {
    'feishu': 'https://accounts.feishu.cn/oauth/v3/token',
    'lark': 'https://accounts.larksuite.com/oauth/v3/token',
}
BEARER_PATTERN = re.compile(r'[A-Za-z0-9\-._~+/]+=*\Z', re.ASCII)
TOOL_PATTERN = re.compile(r'[A-Za-z0-9_.-]{1,128}\Z', re.ASCII)
CALLER_FIELDS = ('channel', 'id', 'name', 'open_id', 'union_id')
CALLER_REQUIRED = ('channel', 'id')
# 标识字段限制与 Gateway ingress 一致：channel ≤64 字节，其余 ≤128 字节；
# 禁止空白、控制字符、冒号、逗号。直接写入 HTTP Header 的值另限可见 ASCII。
CALLER_LIMITS = {'channel': 64, 'id': 128, 'open_id': 128, 'union_id': 128}
MAX_CALLER_NAME_BYTES = 1024
QODERWAKE_CHANNELS = frozenset(('feishu', 'lark', 'dingtalk', 'dingtalk-user', 'wecom-bot'))
# 与网关 newID 一致：前缀 + RFC 4648 base32 文本（大写字母与数字）。
ID_PATTERN = re.compile(r'(?:inv|f)_[A-Z0-9]{16,64}\Z', re.ASCII)
OPERATION_PATTERN = re.compile(r'[A-Za-z0-9][A-Za-z0-9.+_-]{0,127}\Z', re.ASCII)
KIND_PATTERN = re.compile(r'[a-z0-9_]{1,64}\Z', re.ASCII)
LOG_ID_PATTERN = re.compile(r'[A-Za-z0-9_-]{1,128}\Z', re.ASCII)
SCOPE_PATTERN = re.compile(r'[A-Za-z0-9_.:-]{1,128}\Z', re.ASCII)
# 已登记的业务错误类别，收集自网关源码的 fault()/Fault Kind 字面量；
# upstream_* 由执行后端动态拼接（cli.go），按前缀放行。
KNOWN_KINDS = frozenset((
    'access_revoked', 'actor_denied', 'actor_required', 'application_disabled',
    'application_token_failed', 'artifact_collection_failed', 'backend_contract',
    'backend_output_limit', 'backend_output_schema', 'backend_unbound',
    'backend_version_mismatch', 'cancelled', 'concurrency_limit',
    'credential_mode_mismatch', 'domain_unavailable', 'egress_arguments',
    'egress_denied', 'event_cursor_gap', 'event_storage_failed',
    'event_type_denied', 'execution_interrupted', 'file_expired', 'file_in_use',
    'file_too_large', 'file_unavailable', 'idempotency_conflict',
    'identity_violation', 'input_too_large', 'internal', 'invalid_arguments',
    'invalid_caller', 'invalid_cursor', 'invalid_direction', 'invalid_event',
    'invalid_file_path', 'invalid_file_ref', 'invalid_filename',
    'invalid_manifest', 'invalid_mime_type', 'invalid_range',
    'invalid_request_header', 'job_queue_limit', 'network', 'not_found',
    'operation_denied', 'operation_removed', 'output_file_limit',
    'partial_failure', 'process_restarted', 'proof_binding_missing',
    'proof_header_unsupported', 'proof_key_unavailable', 'proof_required',
    'receipt_storage_failed', 'request_credential_async_unsupported',
    'resource_conflict', 'resource_denied', 'resource_egress_denied',
    'resource_scope_unsupported', 'resource_too_large', 'result_storage_failed',
    'rate_limit', 'revoked', 'route_ambiguous', 'route_denied', 'route_invalid',
    'route_not_found', 'service_stopping', 'storage_failed', 'storage_quota',
    'token_rate_limit', 'token_response', 'token_transport',
    'token_type_unsupported', 'transfer_disabled', 'unauthenticated',
    'unknown_domain', 'unknown_operation', 'unknown_parameter', 'unknown_topic',
    'unsafe_output', 'upstream_app_mismatch', 'upstream_credential_invalid',
    'upstream_credential_required', 'upstream_transport', 'workspace_limit',
    'wrong_executor',
))
TECHNICAL_KINDS = frozenset((
    'http_error', 'invalid_response', 'network_error', 'rpc_error',
    'client_error',
))
FILE_URI_PREFIX = 'feishu-file://files/'
# 失败诊断 JSON 只保留这些键；未列出的字段一律不回显。
DIAGNOSTIC_KEYS = (
    'status', 'invocation_id', 'job_id', 'operation_id', 'error', 'files',
    'http_status', 'missing_scopes', 'retry_after_seconds', 'log_id',
    'upstream_code',
)


class ClientError(Exception):
    """Only fixed, credential-free descriptions may be used as this message."""

    def __init__(self, message, diagnostics=None):
        super().__init__(message)
        # diagnostics 是本次失败可选的结构化回执摘要；字段经过校验与秘密过滤。
        self.diagnostics = diagnostics


def _known_kind(value):
    if not isinstance(value, str):
        return None
    candidate = value.lower()
    if candidate in KNOWN_KINDS:
        return candidate
    # upstream_* 由执行后端按问题类型拼接；格式受 KIND_PATTERN 约束。
    if KIND_PATTERN.fullmatch(candidate) and candidate.startswith('upstream_'):
        return candidate
    # HTTP 入口可能收到 "kind: message" 形式的文本；只回收已登记的 kind。
    if ':' in candidate:
        head = candidate.split(':', 1)[0].strip()
        if head in KNOWN_KINDS or (KIND_PATTERN.fullmatch(head) and head.startswith('upstream_')):
            return head
    return None


class UsageError(Exception):
    """命令组合错误：在网络请求前以与 argparse 相同的出口退出 2。"""


class QuietParser(argparse.ArgumentParser):
    def error(self, message):
        # argparse's default includes rejected argument values, which may have
        # accidentally been used for credentials. Do not echo them.
        self.exit(2, '命令行参数无效；请使用 --help 查看用法。\n')


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class HTTPTransport:
    def __init__(self):
        self.opener = urllib.request.build_opener(
            NoRedirect(), urllib.request.ProxyHandler({}))

    def post(self, endpoint, body, headers, limit, secrets=()):
        request = urllib.request.Request(
            endpoint, data=body, headers=headers, method='POST')
        try:
            with self.opener.open(request, timeout=HTTP_TIMEOUT) as response:
                status = response.getcode()
                if not 200 <= status < 300:
                    raise ClientError('HTTP 请求未成功。',
                                      failure_diagnostics({}, secrets, kind='http_error', http_status=status))
                raw = response.read(limit + 1)
                if len(raw) > limit:
                    raise ClientError('响应超过客户端允许的大小。',
                                      failure_diagnostics({}, secrets, kind='invalid_response', http_status=status))
                return raw, response.headers, status
        except urllib.error.HTTPError as exc:
            code = exc.code
            exc.close()
            if 300 <= code < 400:
                raise ClientError('请求被重定向；客户端不会转发凭据到其他地址。',
                                  failure_diagnostics({}, secrets, kind='http_error', http_status=code)) from None
            raise ClientError(f'HTTP 请求失败（状态码 {code}）；请检查地址与凭据配置。',
                              failure_diagnostics({}, secrets, kind='http_error', http_status=code)) from None
        except (urllib.error.URLError, OSError, ValueError, http.client.HTTPException):
            raise ClientError('网络请求失败或超时；请检查服务连接。',
                              failure_diagnostics({}, secrets, kind='network_error')) from None


def normalize_endpoint(value):
    try:
        if not isinstance(value, str) or any(ord(ch) <= 32 for ch in value):
            raise ValueError
        parsed = urllib.parse.urlsplit(value)
        if (parsed.scheme not in ('http', 'https') or not parsed.hostname
                or parsed.username is not None or parsed.password is not None
                or '?' in value or '#' in value):
            raise ValueError
        port = parsed.port
        if port is not None and not 1 <= port <= 65535:
            raise ValueError
        if parsed.scheme == 'http':
            loopback = parsed.hostname.lower() == 'localhost'
            if not loopback:
                try:
                    loopback = ipaddress.ip_address(parsed.hostname).is_loopback
                except ValueError:
                    loopback = False
            if not loopback:
                raise ValueError
        path = '/mcp' if parsed.path in ('', '/') else parsed.path
        return urllib.parse.urlunsplit((parsed.scheme, parsed.netloc, path, '', ''))
    except (ValueError, AttributeError):
        raise ClientError('endpoint 必须为 HTTPS 或本机回环 HTTP，且不能含用户信息、查询或片段。') from None


def valid_atom(value, limit):
    return (isinstance(value, str) and 0 < len(value) <= limit
            and all(0x21 <= ord(ch) <= 0x7e for ch in value))


def valid_bearer(value):
    return (isinstance(value, str) and len(value) <= 8192
            and BEARER_PATTERN.fullmatch(value) is not None)


def reject_json_constant(value):
    raise ValueError


def decode_json(raw, error_message, secrets=(), http_status=None):
    try:
        return json.loads(raw.decode('utf-8'), parse_constant=reject_json_constant)
    except (UnicodeError, ValueError, RecursionError):
        raise ClientError(error_message,
                          failure_diagnostics({}, secrets, kind='invalid_response',
                                              http_status=http_status)) from None


def load_arguments(path):
    if path is None:
        return {}
    try:
        with path.open('rb') as stream:
            raw = stream.read(MAX_ARGUMENT_BYTES + 1)
    except OSError:
        raise ClientError('无法读取参数文件。',
                          failure_diagnostics((), kind='client_error')) from None
    if len(raw) > MAX_ARGUMENT_BYTES:
        raise ClientError('参数文件超过 1 MiB。',
                          failure_diagnostics((), kind='client_error'))
    # 本地参数文件的格式/编码/数值错误是客户端输入问题，归 client_error；
    # Gateway 与取票端点的畸形响应才使用 invalid_response（decode_json 默认）。
    try:
        arguments = json.loads(raw.decode('utf-8'), parse_constant=reject_json_constant)
    except (UnicodeError, ValueError, RecursionError):
        raise ClientError('参数文件必须是有效的 JSON 对象。',
                          failure_diagnostics((), kind='client_error')) from None
    if not isinstance(arguments, dict):
        raise ClientError('参数文件必须是 JSON 对象。',
                          failure_diagnostics((), kind='client_error'))
    return arguments


def application_headers(environ, transport):
    brand = environ.get('FEISHU_BRAND', 'feishu')
    if brand not in TOKEN_ENDPOINTS:
        raise ClientError('FEISHU_BRAND 只能是 feishu 或 lark。')
    app_id = environ.get('FEISHU_APP_ID', '')
    token = environ.get('FEISHU_ACCESS_TOKEN', '')
    secret = environ.get('FEISHU_APP_SECRET', '')
    if not app_id and not token and not secret:
        return {}
    if not valid_atom(app_id, 256) or not (token or secret):
        raise ClientError('上游凭据需要 FEISHU_APP_ID 与应用 token 或 App Secret 配合。')
    if token:
        if not valid_bearer(token):
            raise ClientError('FEISHU_ACCESS_TOKEN 需要原始 Bearer token，不带前缀或空白。')
    else:
        if not isinstance(secret, str) or not secret or len(secret) > 8192:
            raise ClientError('客户端 App Secret 配置无效。')
        # This is the only request that contains App Secret. Its origin is
        # fixed by brand and cannot be replaced by the Gateway endpoint.
        form = urllib.parse.urlencode({
            'grant_type': 'client_credentials',
            'client_id': app_id,
            'client_secret': secret,
        }).encode('utf-8')
        raw, _, _ = transport.post(
            TOKEN_ENDPOINTS[brand], form,
            {'Content-Type': 'application/x-www-form-urlencoded', 'Accept': 'application/json'},
            MAX_TOKEN_RESPONSE_BYTES)
        reply = decode_json(raw, '应用取票响应不是有效 JSON。')
        if not isinstance(reply, dict) or reply.get('error') or reply.get('code', 0) != 0:
            raise ClientError('应用取票失败；请在客户端检查应用凭据与取票方式。')
        token_type = reply.get('token_type')
        if isinstance(token_type, str) and token_type.lower() == 'dpop':
            raise ClientError('客户端样例不支持 DPoP 绑定 token；需要 Bearer 应用 token。')
        if not isinstance(token_type, str) or token_type.lower() != 'bearer':
            raise ClientError('应用取票未返回明确的 Bearer token 类型。')
        token = reply.get('access_token')
        if not valid_bearer(token):
            raise ClientError('应用取票未返回可用的 Bearer token。')
    return {'X-Feishu-App-Id': app_id, 'X-Feishu-Access-Token': token}


def rpc_request(transport, endpoint, headers, message, expected_id, secrets=()):
    raw, response_headers, status = transport.post(
        endpoint, json.dumps(message, ensure_ascii=False, allow_nan=False).encode('utf-8'),
        headers, MAX_GATEWAY_RESPONSE_BYTES, secrets)
    reply = decode_json(raw, 'Gateway 未返回有效的 JSON-RPC 响应；此样例需要 JSONResponse 服务。',
                        secrets, status)
    if (not isinstance(reply, dict) or reply.get('jsonrpc') != '2.0'
            or type(reply.get('id')) is not int or reply['id'] != expected_id):
        raise ClientError('Gateway 返回的 JSON-RPC 响应格式不匹配。',
                          failure_diagnostics({}, secrets, kind='invalid_response', http_status=status))
    if 'error' in reply:
        raise ClientError('Gateway 返回 JSON-RPC 错误；本次调用不会自动重试。',
                          failure_diagnostics({}, secrets, kind='rpc_error', http_status=status))
    result = reply.get('result')
    if not isinstance(result, dict):
        raise ClientError('Gateway 未返回预期的结果对象。',
                          failure_diagnostics({}, secrets, kind='invalid_response', http_status=status))
    return result, response_headers, status


def _contains_secret(value, secret):
    """value 的任何嵌套字符串中包含完整秘密值即为泄露。"""
    if isinstance(value, str):
        return secret in value
    if isinstance(value, dict):
        return any(_contains_secret(v, secret) for v in value.values())
    if isinstance(value, list):
        return any(_contains_secret(v, secret) for v in value)
    return False


def scrub_diagnostics(fields, secrets=()):
    """把可选字段收敛为有限、校验过且不含已知秘密值的诊断字典。"""
    out = {}
    for name in DIAGNOSTIC_KEYS:
        if name not in fields:
            continue
        value = fields[name]
        if name == 'status':
            if isinstance(value, str) and value in ('succeeded', 'failed', 'unknown', 'partial', 'cancelled', 'running', 'queued'):
                out['status'] = value
        elif name in ('invocation_id', 'job_id'):
            if isinstance(value, str) and ID_PATTERN.fullmatch(value):
                out[name] = value
        elif name == 'operation_id':
            if isinstance(value, str) and OPERATION_PATTERN.fullmatch(value):
                out['operation_id'] = value
        elif name == 'http_status':
            if type(value) is int and 100 <= value <= 599:
                out['http_status'] = value
        elif name == 'upstream_code':
            if type(value) is int and value >= 0:
                out['upstream_code'] = value
        elif name == 'log_id':
            if isinstance(value, str) and LOG_ID_PATTERN.fullmatch(value):
                out['log_id'] = value
        elif name == 'retry_after_seconds':
            if type(value) in (int, float) and 0 <= value <= 86400:
                out['retry_after_seconds'] = value
        elif name == 'missing_scopes':
            if isinstance(value, list) and 0 < len(value) <= 32:
                scopes = [s for s in value if isinstance(s, str) and SCOPE_PATTERN.fullmatch(s)]
                if len(scopes) == len(value):
                    out['missing_scopes'] = scopes
        elif name == 'files':
            if isinstance(value, list) and 0 < len(value) <= 64:
                files = []
                for item in value:
                    file_id = item.get('file_id') if isinstance(item, dict) else None
                    if isinstance(file_id, str) and ID_PATTERN.fullmatch(file_id):
                        files.append({'file_id': file_id, 'uri': FILE_URI_PREFIX + file_id})
                    else:
                        files = None
                        break
                if files is not None:
                    out['files'] = files
        elif name == 'error':
            error = {}
            source = value if isinstance(value, dict) else {}
            kind = _known_kind(source.get('kind'))
            if kind is not None:
                error['kind'] = kind
            code = source.get('upstream_code')
            if type(code) is int and code >= 0:
                error['upstream_code'] = code
            log_id = source.get('log_id')
            if isinstance(log_id, str) and LOG_ID_PATTERN.fullmatch(log_id):
                error['log_id'] = log_id
            scopes = source.get('missing_scopes')
            if isinstance(scopes, list) and 0 < len(scopes) <= 32:
                checked = [s for s in scopes if isinstance(s, str) and SCOPE_PATTERN.fullmatch(s)]
                if len(checked) == len(scopes):
                    error['missing_scopes'] = checked
            retry = source.get('retry_after_seconds')
            if type(retry) in (int, float) and 0 <= retry <= 86400:
                error['retry_after_seconds'] = retry
            if error:
                out['error'] = error
    # 含有本次任何完整秘密值的字段整体省略：嵌套结构里出现秘密同样要移除该字段。
    # error 对象里的 kind 是已登记的固定类别，不含秘密输入；被污染时降级为仅保留 kind。
    for secret in secrets:
        if not secret:
            continue
        cleaned = {}
        for key, value in out.items():
            if not _contains_secret(value, secret):
                cleaned[key] = value
            elif key == 'error' and isinstance(value, dict) and 'kind' in value                     and not _contains_secret(value['kind'], secret):
                cleaned[key] = {'kind': value['kind']}
        out = cleaned
    if 'status' not in out:
        out['status'] = 'failed'
    return out


def failure_diagnostics(fields, secrets=(), kind=None, http_status=None):
    """构造失败诊断；kind 是固定的技术类别，http_status 仅在确有 HTTP 状态时提供。"""
    out = scrub_diagnostics(fields, secrets)
    error = dict(out.get('error') or {})
    if kind and 'kind' not in error:
        if kind in TECHNICAL_KINDS or _known_kind(kind):
            error['kind'] = kind
    if error:
        out['error'] = error
    if http_status is not None and type(http_status) is int and 100 <= http_status <= 599:
        out['http_status'] = http_status
    out.setdefault('status', 'failed')
    return out


def result_fields(result):
    """从业务结果对象提取允许出现在诊断里的字段；不复制 data、meta 或未知字段。"""
    if not isinstance(result, dict):
        return {}
    return {name: result[name] for name in DIAGNOSTIC_KEYS if name in result}


def business_result(tool_result, secrets=()):
    """返回 (result, diagnostics)。成功时 diagnostics 为 None；失败时为校验后的诊断字典。"""
    def fail(kind, fields=None):
        return None, failure_diagnostics(fields or {}, secrets, kind=kind)

    if tool_result.get('isError') is True:
        fields = {}
        structured = tool_result.get('structuredContent')
        if isinstance(structured, dict):
            fields = result_fields(structured)
        else:
            content = tool_result.get('content')
            texts = [item.get('text') for item in content
                     if isinstance(item, dict) and item.get('type') == 'text'] if isinstance(content, list) else []
            if len(texts) == 1 and isinstance(texts[0], str):
                try:
                    parsed = json.loads(texts[0], parse_constant=reject_json_constant)
                except (ValueError, RecursionError):
                    parsed = None
                if isinstance(parsed, dict):
                    fields = result_fields(parsed)
        return fail('client_error', fields)
    if tool_result.get('isError') not in (None, False):
        return fail('invalid_response')
    result = tool_result.get('structuredContent')
    if result is None:
        content = tool_result.get('content')
        texts = [item.get('text') for item in content
                 if isinstance(item, dict) and item.get('type') == 'text'] if isinstance(content, list) else []
        if len(texts) != 1 or not isinstance(texts[0], str):
            return fail('invalid_response')
        try:
            result = json.loads(texts[0], parse_constant=reject_json_constant)
        except (ValueError, RecursionError):
            return fail('invalid_response')
    if not isinstance(result, dict):
        return fail('invalid_response')
    if result.get('error') is not None or result.get('status') in ('failed', 'unknown', 'partial', 'cancelled'):
        return fail('client_error', result_fields(result))
    return result, None


def call_tool(transport, endpoint, headers, tool, arguments, secrets=()):
    headers = dict(headers)
    headers.update({'Content-Type': 'application/json', 'Accept': 'application/json, text/event-stream'})
    initialize, response_headers, _ = rpc_request(transport, endpoint, headers, {
        'jsonrpc': '2.0', 'id': 1, 'method': 'initialize',
        'params': {
            'protocolVersion': PROTOCOL_VERSION,
            'capabilities': {},
            'clientInfo': {'name': 'feishu-gateway-request-client', 'version': '1.0'},
        },
    }, 1, secrets)
    if initialize.get('protocolVersion') != PROTOCOL_VERSION:
        raise ClientError('Gateway 协商的 MCP 协议版本不在此样例支持范围内。',
                          failure_diagnostics({}, secrets, kind='invalid_response'))
    headers['MCP-Protocol-Version'] = PROTOCOL_VERSION
    session_id = response_headers.get('Mcp-Session-Id')
    if session_id is not None:
        if not valid_atom(session_id, 256):
            raise ClientError('Gateway 返回的会话标识格式无效。',
                              failure_diagnostics({}, secrets, kind='invalid_response'))
        headers['Mcp-Session-Id'] = session_id
    raw, _, _ = transport.post(endpoint, json.dumps({
        'jsonrpc': '2.0', 'method': 'notifications/initialized',
    }).encode('utf-8'), headers, MAX_GATEWAY_RESPONSE_BYTES, secrets)
    if raw.strip():
        raise ClientError('Gateway 初始化通知返回了意外内容。',
                          failure_diagnostics({}, secrets, kind='invalid_response'))
    result, _, _ = rpc_request(transport, endpoint, headers, {
        'jsonrpc': '2.0', 'id': 2, 'method': 'tools/call',
        'params': {'name': tool, 'arguments': arguments},
    }, 2, secrets)
    return business_result(result, secrets)


def caller_headers(adapter, caller):
    """按声明式映射把 caller 字段序列化为 Header。

    可选字段缺省或为空时省略对应 Header，不补造身份值。
    name 是显示信息：合法 Unicode、UTF-8 后 ≤1024 字节且无控制字符时
    URI component 编码一次后发送；无效时省略显示名，保留有效主身份。
    """
    if adapter == 'qoderwake':
        prefix, field_of = 'X-QoderWake-AtWaker-Sender-', {
            'channel': 'Channel', 'id': 'Id', 'name': 'Name',
            'open_id': 'Open-Id', 'union_id': 'Union-Id'}
    elif adapter == 'headers':
        prefix, field_of = 'X-Caller-', {
            'channel': 'Channel', 'id': 'Id', 'name': 'Name',
            'open_id': 'Open-Id', 'union_id': 'Union-Id'}
    else:
        raise ClientError('caller-adapter 只能是 qoderwake 或 headers。')
    headers = {}
    for field, suffix in field_of.items():
        value = caller.get(field, '')
        if not value:
            continue
        if field == 'name':
            encoded = urllib.parse.quote(value, safe='')
            headers[prefix + suffix] = encoded
        else:
            headers[prefix + suffix] = value
    return headers


def _valid_caller_identifier(value, limit):
    """标识字段：UTF-8 字节数有界，禁止空白、控制字符、冒号、逗号。"""
    if not isinstance(value, str) or not value:
        return False
    encoded = value.encode('utf-8', errors='strict')
    if len(encoded) > limit:
        return False
    for ch in value:
        if ch.isspace() or ord(ch) < 0x20 or ord(ch) == 0x7f or ch in ':,':
            return False
    return True


def _valid_header_atom(value):
    """直接写入 HTTP Header 的标识值继续限制为可见 ASCII。"""
    return all(0x21 <= ord(ch) <= 0x7e for ch in value)


def _is_control_code(ch):
    """与 Gateway unicode.IsControl 对齐：C0 (0x00-0x1F)、DEL (0x7F)、C1 (0x80-0x9F)。

    直接按码位判断：Python isspace 的空字符集与 Unicode 控制字符不完全重合
    （例如 \x1f、\x85 是控制字符但 isspace 为 True/False 的划分不同）。
    """
    code = ord(ch)
    return code < 0x20 or 0x7f <= code <= 0x9f


def _valid_caller_name(value):
    """显示名：合法 Unicode、UTF-8 后 ≤1024 字节且无控制字符。

    孤立 surrogate 无法编码为 UTF-8：按显示名无效处理，省略 name 而不阻断调用。
    """
    if not isinstance(value, str) or not value:
        return False
    try:
        encoded = value.encode('utf-8', errors='strict')
    except UnicodeEncodeError:
        return False
    if len(encoded) > MAX_CALLER_NAME_BYTES:
        return False
    return not any(_is_control_code(ch) for ch in value)


def load_caller(path, adapter):
    """读取并校验 caller 文件；必填 channel/id，可选字段可省略或为空。"""
    try:
        with path.open('rb') as stream:
            raw = stream.read(MAX_CALLER_FILE_BYTES + 1)
    except OSError:
        raise ClientError('无法读取 caller 文件。') from None
    if len(raw) > MAX_CALLER_FILE_BYTES:
        raise ClientError('caller 文件超过 16 KiB。')
    try:
        text = raw.decode('utf-8')
    except UnicodeError:
        raise ClientError('caller 文件必须是 UTF-8 JSON。') from None

    def reject_duplicates(pairs):
        seen = set()
        for key, _ in pairs:
            if key in seen:
                raise ValueError
            seen.add(key)
        return dict(pairs)

    try:
        caller = json.loads(text, parse_constant=reject_json_constant,
                            object_pairs_hook=reject_duplicates)
    except (ValueError, RecursionError):
        raise ClientError('caller 文件必须是有效的 JSON 对象。') from None
    if not isinstance(caller, dict) or not set(caller) <= set(CALLER_FIELDS):
        raise ClientError('caller 文件只允许 channel、id、name、open_id、union_id 字段。')
    for field in CALLER_FIELDS:
        if field not in caller:
            continue
        value = caller[field]
        if not isinstance(value, str):
            raise ClientError(f'caller 字段 {field} 必须是字符串。')
    for field in CALLER_REQUIRED:
        if not _valid_caller_identifier(caller.get(field, ''), CALLER_LIMITS[field]):
            raise ClientError('caller 需要非空的 channel 与 id（长度与字符受限）。')
        if not _valid_header_atom(caller[field]):
            raise ClientError('caller 的 channel/id 只能包含可见 ASCII 字符。')
    for field in ('open_id', 'union_id', 'name'):
        value = caller.get(field, '')
        if value == '':
            continue
        if field == 'name':
            # name 类型错误在上方统一拒绝；此处只决定是否发送显示名。
            continue
        if not _valid_caller_identifier(value, CALLER_LIMITS[field]):
            raise ClientError(f'caller 字段 {field} 长度或字符无效。')
        if not _valid_header_atom(value):
            raise ClientError(f'caller 字段 {field} 只能包含可见 ASCII 字符。')
    channel = caller['channel']
    if adapter == 'qoderwake' and channel not in QODERWAKE_CHANNELS:
        raise ClientError('qoderwake 渠道只能是 feishu、lark、dingtalk、dingtalk-user 或 wecom-bot。')
    # headers 适配器接受任意符合格式的通用渠道，客户端不设白名单。
    # 显示名无效时省略，不阻断调用。
    if not _valid_caller_name(caller.get('name', '')):
        caller = dict(caller)
        caller['name'] = ''
    return caller_headers(adapter, caller)


def run(args, environ):
    """执行一次调用；本地校验先于任何取票或 Gateway 请求。"""
    # 先做所有本地校验（组合、格式、文件），再触发任何取票或 Gateway 请求。
    if (args.caller_file is None) != (args.caller_adapter is None):
        raise UsageError('--caller-file 与 --caller-adapter 必须成对提供。')
    context_only = args.context_only
    if context_only and (args.tool != 'feishu_context' or args.arguments_file is not None):
        raise UsageError('--context-only 不能与自定义 --tool 或 --arguments-file 组合。')
    endpoint = normalize_endpoint(args.endpoint)
    if not TOOL_PATTERN.fullmatch(args.tool):
        raise ClientError('工具名称格式无效。')
    if args.route is not None and not valid_atom(args.route, 128):
        raise ClientError('路由 ID 格式无效。')
    arguments = load_arguments(args.arguments_file)
    # caller 文件校验在任何取票之前完成。
    extra_caller = load_caller(args.caller_file, args.caller_adapter) if args.caller_file is not None else None
    gateway_key = environ.get('FEISHU_GATEWAY_KEY', '')
    if not valid_bearer(gateway_key):
        raise ClientError('需要有效的 FEISHU_GATEWAY_KEY。')
    transport = HTTPTransport()
    headers = {'Authorization': 'Bearer ' + gateway_key}
    secrets = [gateway_key]
    if not context_only:
        upstream = application_headers(environ, transport)
        headers.update(upstream)
        if 'X-Feishu-Access-Token' in upstream:
            secrets.append(upstream['X-Feishu-Access-Token'])
        secret = environ.get('FEISHU_APP_SECRET', '')
        if secret:
            secrets.append(secret)
        app_id = environ.get('FEISHU_APP_ID', '')
        if app_id:
            secrets.append(app_id)
    if extra_caller:
        headers.update(extra_caller)
    if args.route is not None:
        headers['X-Feishu-Gateway-Route'] = args.route
    return call_tool(transport, endpoint, headers, args.tool, arguments, tuple(secrets))


def main(argv=None, environ=None):
    parser = QuietParser(description=__doc__)
    parser.add_argument('--endpoint', required=True, help='Gateway HTTPS 或回环 HTTP 地址；origin 自动补 /mcp')
    parser.add_argument('--tool', default='feishu_context', help='要调用的 MCP 工具名')
    parser.add_argument('--arguments-file', type=Path, help='工具参数的 JSON 对象文件，最多 1 MiB')
    parser.add_argument('--route', help='此客户端已配置的路由 ID')
    parser.add_argument('--caller-adapter', choices=('qoderwake', 'headers'),
                        help='caller 文件的 Header 适配方式；须与 --caller-file 成对提供')
    parser.add_argument('--caller-file', type=Path,
                        help='caller JSON 文件；channel/id 必填，name/open_id/union_id 可选，最多 16 KiB')
    parser.add_argument('--context-only', action='store_true',
                        help='只初始化并调用 feishu_context；不读取或发送上游凭据，也不取票')
    args = parser.parse_args(argv)
    environ = os.environ if environ is None else environ
    try:
        result, diagnostics = run(args, environ)
        if result is None:
            # 运行失败统一出口：stdout 恰好一个受限诊断 JSON，stderr 一句固定说明。
            print(json.dumps(diagnostics, ensure_ascii=False, allow_nan=False, indent=2))
            print('Gateway 工具调用未成功；本次调用不会自动重试。', file=sys.stderr)
            return 1
        print(json.dumps(result, ensure_ascii=False, allow_nan=False, indent=2))
        return 0
    except UsageError as exc:
        # 命令组合错误：无网络请求、无 stdout，与 argparse 用法错误同一出口。
        print(str(exc) if str(exc) else '命令行参数无效；请使用 --help 查看用法。', file=sys.stderr)
        return 2
    except ClientError as exc:
        # 运行失败的统一出口：诊断可能来自业务结果或传输/响应层。
        print(str(exc), file=sys.stderr)
        print(json.dumps(exc.diagnostics or failure_diagnostics((), kind='client_error'),
                         ensure_ascii=False, allow_nan=False, indent=2))
        return 1
    except KeyboardInterrupt:
        print('客户端调用已取消；未自动重试。', file=sys.stderr)
        return 130
    except Exception:
        # Never expose raw urllib errors, upstream bodies, environment values,
        # JSON fragments or tracebacks in this credential-handling entry point.
        print('客户端处理失败；未自动重试，也未输出凭据。', file=sys.stderr)
        print(json.dumps(failure_diagnostics((), kind='client_error'),
                         ensure_ascii=False, allow_nan=False, indent=2))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
