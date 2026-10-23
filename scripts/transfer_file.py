#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""在本机与 Feishu-MCP-Gateway 之间传输单个文件；票据 JSON 从 stdin 读取。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import tempfile
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--endpoint', required=True, help='Gateway 地址，HTTPS 或本机回环 HTTP')
    parser.add_argument('--file', type=Path, required=True, help='上传输入或下载目标；下载不覆盖已有文件')
    args = parser.parse_args()
    import sys
    ticket = json.load(sys.stdin)
    ticket = ticket.get('data', ticket)
    endpoint = urllib.parse.urlsplit(args.endpoint)
    if endpoint.scheme != 'https' and not (endpoint.scheme == 'http' and endpoint.hostname in ('127.0.0.1', 'localhost', '::1')):
        raise SystemExit('endpoint 需要 HTTPS 或回环 HTTP')
    if endpoint.username or endpoint.password or endpoint.query or endpoint.fragment or endpoint.path not in ('', '/'):
        raise SystemExit('endpoint 只接受 Gateway origin')
    if ticket.get('path') != '/transfers/file' or ticket.get('method') not in ('GET', 'POST'):
        raise SystemExit('不是 Gateway 文件票据')
    limit = min(int(ticket['max_bytes']), 256 * 1024 * 1024)
    if limit < 1:
        raise SystemExit('文件上限无效')
    body = None
    if ticket['method'] == 'POST':
        with args.file.open('rb') as source:
            body = source.read(limit + 1)
        if len(body) > limit:
            raise SystemExit('输入文件超过票据上限')
    elif args.file.exists():
        raise SystemExit('目标文件已存在，请使用新的文件名')
    request = urllib.request.Request(args.endpoint.rstrip('/') + ticket['path'], data=body,
                                     method=ticket['method'], headers={'Authorization': 'Bearer ' + ticket['ticket']})
    opener = urllib.request.build_opener(NoRedirect, urllib.request.ProxyHandler({}))
    try:
        with opener.open(request, timeout=60) as response:
            if body is not None:
                print(response.read(65536).decode('utf-8'))
                return
            args.file.parent.mkdir(parents=True, exist_ok=True)
            fd, name = tempfile.mkstemp(prefix='.gateway-transfer-', dir=args.file.parent)
            try:
                size, sha = 0, hashlib.sha256()
                with os.fdopen(fd, 'wb') as out:
                    while chunk := response.read(min(65536, limit - size + 1)):
                        size += len(chunk)
                        if size > limit:
                            raise ValueError('下载超过票据上限')
                        out.write(chunk)
                        sha.update(chunk)
                    out.flush()
                    os.fsync(out.fileno())
                os.link(name, args.file)
                print(json.dumps({'file': str(args.file), 'size': size, 'sha256': sha.hexdigest()}))
            finally:
                os.unlink(name)
    except urllib.error.HTTPError as exc:
        raise SystemExit(f'文件传输失败：HTTP {exc.code}；请重新取得票据并核对权限') from None
    except urllib.error.URLError:
        raise SystemExit('文件传输连接失败；请检查 Gateway 地址') from None


if __name__ == '__main__':
    main()
