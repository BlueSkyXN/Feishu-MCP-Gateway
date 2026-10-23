#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""启动独立 Gateway 进程验证 MCP 与文件链路；不调用飞书操作。"""
import argparse
import hashlib
import json
import secrets
import socket
import subprocess
import tempfile
import time
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('gateway', type=Path)
    parser.add_argument('cli', type=Path)
    parser.add_argument('--output', type=Path, default=Path('local/evidence/process-smoke.json'))
    args = parser.parse_args()
    cli = args.cli.resolve()
    with tempfile.TemporaryDirectory(prefix='gateway-process-smoke-') as directory:
        root = Path(directory)
        key = secrets.token_urlsafe(32)
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        revision=subprocess.check_output([str(args.gateway.resolve()), '--catalog-revision'],text=True).strip()
        config = {'catalog_revision':revision, 'listen': f'127.0.0.1:{port}', 'data_dir': str(root/'data'), 'cli_binary': str(cli), 'cli_sha256': hashlib.sha256(cli.read_bytes()).hexdigest(), 'apps': [{'id': 'fixture-app', 'organization_id': 'fixture-org', 'app_id': 'cli_fixture', 'brand': 'feishu', 'secret_env': 'FIXTURE_APP_SECRET'}], 'clients': [{'id': 'fixture-client', 'app_connection_id': 'fixture-app', 'key_sha256': hashlib.sha256(key.encode()).hexdigest(), 'identity_mode': 'client', 'packs': ['wiki.read'], 'resource_mode': 'resource_scoped', 'resources': {'wiki_space': ['fixture-space']}}]}
        path = root/'config.json'
        path.write_text(json.dumps(config))
        path.chmod(0o600)
        env = {'PATH': '/usr/bin:/bin', 'HOME': str(root), 'FIXTURE_APP_SECRET': 'fixture-app-secret'}
        command = [str(args.gateway.resolve()), '--config', str(path)]
        check = subprocess.run([*command, '--check'], env=env, capture_output=True, text=True, timeout=20)
        if check.returncode:
            raise RuntimeError('configuration preflight failed: ' + check.stderr)
        base = f'http://127.0.0.1:{port}'
        log = (root/'stderr.log').open('w')
        process = subprocess.Popen(command, env=env, stdout=subprocess.DEVNULL, stderr=log)
        counter = 0
        def rpc(method, params):
            nonlocal counter
            counter += 1
            data = json.dumps({'jsonrpc': '2.0', 'id': counter, 'method': method, 'params': params}).encode()
            req = Request(base+'/mcp', data=data, headers={'Authorization': 'Bearer '+key, 'Content-Type': 'application/json', 'Accept': 'application/json, text/event-stream', 'Mcp-Protocol-Version': '2025-11-25'})
            with urlopen(req, timeout=10) as response:
                assert 'private' in response.headers.get('Cache-Control', '')
                value = json.load(response)
                if 'error' in value:
                    raise RuntimeError('MCP error: '+json.dumps(value['error']))
                return value['result']
        try:
            deadline = time.monotonic()+10
            while True:
                try:
                    with urlopen(base+'/healthz', timeout=1) as response:
                        assert json.load(response)['status'] == 'ok'
                    break
                except URLError:
                    if process.poll() is not None or time.monotonic()>deadline:
                        raise RuntimeError('server did not become healthy')
                    time.sleep(0.1)
            init = rpc('initialize', {'protocolVersion': '2025-11-25', 'clientInfo': {'name': 'smoke', 'version': '1'}, 'capabilities': {}})
            assert init['protocolVersion'] == '2025-11-25'
            listing = rpc('tools/list', {})
            context = rpc('tools/call', {'name': 'feishu_context', 'arguments': {}})
            assert context['structuredContent']['data']['execution_identity'] == 'bot'
            result = rpc('tools/call', {'name': 'feishu_file_create', 'arguments': {'name': 'smoke.txt', 'text': 'gateway smoke'}})
            file = result['structuredContent']['files'][0]
            resource = rpc('resources/read', {'uri': file['uri']})
            assert resource['contents'][0]['text'] == 'gateway smoke'
            with urlopen(Request(base+'/files/'+file['file_id'], headers={'Authorization': 'Bearer '+key}), timeout=5) as response:
                assert response.read() == b'gateway smoke'
            try:
                urlopen(base+'/files/'+file['file_id'], timeout=5)
                raise AssertionError('unauthenticated download accepted')
            except HTTPError as exc:
                assert exc.code == 401
            report = {'startup': True, 'config_check': True, 'protocol': init['protocolVersion'], 'tools': len(listing['tools']), 'application_identity': True, 'resource_read': True, 'authenticated_download': True, 'unauthenticated_download_denied': True, 'live_feishu_calls': 0}
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
            log.close()
        assert process.returncode == 0
        report['graceful_shutdown'] = True
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps(report, ensure_ascii=False))


if __name__ == '__main__':
    main()
