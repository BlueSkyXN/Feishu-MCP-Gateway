#!/usr/bin/env python3
# Copyright (c) 2026 BlueSkyXN
# SPDX-License-Identifier: GPL-3.0-only
"""把固定 lark-cli 源码里的用法知识转换为 Gateway 指南。

来源包括 affordance/*.md、shortcut 的 Tips 和 skills/*。只读取来源快照，不执行 CLI，不请求飞书。
转换规则是确定性的：命令写法改为 operation_id，CLI flag 改为 JSON 参数名，用户身份与登录说明删除，
网关未提供的操作显式标注。代码块中的命令只有能完整、等价映射时才改写为调用行，否则保留原文。
由 import_catalog.py 调用。
"""
import json
import re
import subprocess
from pathlib import PurePosixPath

# 上游 Skill → 主领域。领域 ID 与 Operation 的服务前缀一致；标题、别名等元数据见 internal/gateway/domains.go。
SKILLS = {
    'lark-doc': 'docs', 'lark-sheets': 'sheets', 'lark-base': 'base', 'lark-drive': 'drive', 'lark-wiki': 'wiki',
    'lark-im': 'im', 'lark-calendar': 'calendar', 'lark-task': 'task', 'lark-mail': 'mail', 'lark-okr': 'okr',
    'lark-slides': 'slides', 'lark-meeting': 'vc', 'lark-whiteboard': 'whiteboard', 'lark-markdown': 'markdown',
    'lark-contact': 'contact',
}
# 只做引用改写、不导入正文的上游 Skill 名称。
SKILL_ALIASES = {'lark-minutes': 'minutes', 'lark-vc': 'vc', 'lark-vc-agent': 'vc', 'lark-note': 'note', 'lark-event': 'events', 'lark-attendance': 'attendance'}
# 只在 CLI 环境中有意义的全局 flag；import_catalog.py 也不把它们导出为参数。
CLI_ONLY = {'as', 'profile', 'yes', 'dry-run', 'format', 'print-schema', 'flag-name', 'api-version', 'jq', 'help'}
VALUE_FLAGS = {'as', 'profile', 'format', 'flag-name', 'api-version', 'jq'}
REASONS = {'user_only': '仅支持用户身份'}
# 只适用于用户身份、CLI 登录、本机路径或本机 CLI 管理的内容，整行或整条删除。
DROP = re.compile(r'--as[ =]user\b|--as\b(?![ =]bot\b)|\bauth\s+(?:login|logout|status|scopes|check)\b|lark-cli\s+(?:auth|skills|update|config|profile|doctor)\b'
                  r'|lark-shared|\bUAT\b|user_access_token|user identity|用户身份|个人身份|登录态|\bCWD\b'
                  r'|\buser or bot\b|\bbot or user\b|both are supported|history expansion|set \+H|bool flag 语法|relying on the shell', re.I)
# 网关固定以应用身份执行：只提到 `--as bot` 的内容保留，命令中的 --as bot 去掉，说明文字改为“应用身份”。
AS_BOT = re.compile(r'--as[ =]bot\b')
AS_IN_COMMAND = re.compile(r'(?:^|\s+)--as[ =](?:bot|user)\b')
# 描述 CLI 全局 flag 的表格行整行删除；按 shell 区分写法的表格整表删除。
CLI_ONLY_ROW = re.compile(r'^\s*\|\s*`?--(?:' + '|'.join(re.escape(f) for f in sorted(CLI_ONLY, key=len, reverse=True)) + r')\b')
SHELL_TABLE = re.compile(r'\b(?:bash|zsh|PowerShell|cmd\.exe)\b')
TABLE_RULE = re.compile(r'\s*\|(?:\s*:?-{3,}:?\s*\|)+\s*')
HAN = re.compile(r'[\u4e00-\u9fff]')
# 英文条目（affordance、Go tips）中的网关说明改用英文。
ENGLISH_NOTES = [
    ('（网关不提供：仅支持用户身份）', ' (not available in the gateway: user identity only)'),
    ('（网关不提供：已由网关原生能力替代）', ' (replaced by a gateway tool)'),
    ('（网关不提供 dry-run 预览）', ' (dry-run preview is not available in the gateway)'),
    ('管理员预授权（high_risk_operations）', 'administrator pre-authorization (high_risk_operations)'),
    ('dry-run（网关不提供，改为先读取核对）', 'dry-run (not available in the gateway; read and verify first)'),
    ('jq（网关不提供）', 'jq (not available in the gateway)'),
    ('应用身份', 'app identity'),
    ('` 领域', '` domain'),
    ('主题 `', 'topic `'),
    ('（', ' ('),
    ('）', ')'),
    ('，', ', '),
]
CODE_SPAN = re.compile(r'`([^`\n]+)`')
WIKI_LINK = re.compile(r'\[\[(.+?)\]\]')
MD_LINK = re.compile(r'\[([^\]\n]+)\]\(([^)\s]+)\)')
FLAG = re.compile(r'(?<![\w-])--([a-z][a-z0-9-]*)(?:=([^\s`，。；、）]+))?')
PRINT_SCHEMA = re.compile(r'--print-schema(?:\s+--flag-name[ =]([\w<>-]+))?')
PLUS = re.compile(r'(?<![\w.`/+-])\+([a-z][a-z0-9-]*)')
SPACED = re.compile(r'(?<![\w.`/+-])([a-z][a-z_]*)\s+\+([a-z][a-z0-9-]*)')
CLI_CALL = re.compile(r'lark-cli((?:\s+(?:"[^"\n]*"|\'[^\'\n]*\'|[A-Za-z0-9_.+:/@<>=,*{}-]+))+)')
SKILL_NAME = re.compile(r'(?<![\w/.-])(lark-[a-z]+(?:-[a-z]+)*)(?![\w/.-])')
TOPIC_PATH = re.compile(r'^(?:\.\./)*(?:[\w-]+/)*[\w-]+\.md$')
SHELL_LANGS = ('', 'bash', 'sh', 'shell', 'console', 'zsh')
# 文档中的 shell 变量表示前一步取得的值，改写为同名的 <NAME> 占位值。
SHELL_VAR = re.compile(r'\$(?:\{([A-Za-z_]\w*)\}|([A-Za-z_]\w*))')
# 占位值：<doc_id>、<目标ID>、<sheet|bitable>；带引号、等号或斜杠的是 XML 等字面内容。
PLACEHOLDER = re.compile(r'<[^<>"\'=/\s]+>')
SHELL_OP = re.compile(r'[|&;<>()]+\d*')
ANSI_C = {'a': '\a', 'b': '\b', 'e': '\x1b', 'E': '\x1b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v',
          '\\': '\\', "'": "'", '"': '"', '?': '?'}
DRY_RUN_NOTE = '# 网关不提供 dry-run 预览，此示例省略'
# CLI 本地输入写法改成网关的文件引用说法。
REPHRASE = [
    (re.compile(r'prefer `@file` or `-` \(stdin\)'), 'pass a JSON string or `{"file_id": …}`'),
    (re.compile(r'`@file`|`@\./[^`]*`|`@绝对路径`|`@相对路径`'), '`{"file_id": …}`'),
    (re.compile(r'`-` \(stdin\)'), '`{"file_id": …}`'),
    (re.compile(r'（exit 10 强确认门禁）'), '（需管理员预授权）'),
    (re.compile(r'pass --yes to confirm \(exit 10 without it\), or preview with --dry-run first'),
     'needs administrator pre-authorization (high_risk_operations); read the target range first'),
    # 网关参数是 JSON：换行写成 JSON 字符串里的 \n，不需要 shell 的 $'...' 引号。
    (re.compile(r"prefer (?:shell ANSI-C quoting with )?`\$'\.\.\.'` for either"), lambda m: 'write line breaks as `\\n` inside the JSON string for either'),
    (re.compile(r" plus `\$'\.\.\.'`|, usually with `\$'\.\.\.'`"), ''),
]
# 只讲 shell 传参的小节整节换成网关写法：标题 → (新标题, 正文行)。
SECTION_REPLACEMENTS = {
    'Shell 传参': ('参数传递', [
        '- 网关参数是 JSON，不经过 shell：多行内容在 JSON 字符串里用 `\\n` 换行，`$`、反引号和引号按字面传递。',
        '- 长文本或以 `@` 开头的内容，先用 `feishu_file_create` 保存，再把 `{"file_id": …}` 传给 `content`；`pattern` 只接受文本。',
    ]),
}
# 说明 CLI 语法的模板（占位符无法逐项解析）改为 operation_id 写法。
CLI_TEMPLATES = [
    (re.compile(r'lark-cli ([a-z]+) \+<verb> \[flags\]'), r'\1.+<verb> {参数}'),
    (re.compile(r"lark-cli ([a-z]+) <resource> <method> --params '\{\.\.\.\}' \[--data '\{\.\.\.\}'\]"), r'\1.<resource>.<method> {"params": {…}, "data": {…}}'),
    (re.compile(r'lark-cli ([a-z]+) <resource> <method>(?: \[flags\]| \.\.\.)?'), r'\1.<resource>.<method> {参数}'),
    (re.compile(r'lark-cli ([a-z]+) \+\.\.\.'), r'\1.+…'),
]
REPHRASE.extend(CLI_TEMPLATES)
SCHEMA_CALL = re.compile(r'lark-cli schema ([a-z_]+(?:\.(?:[a-z_]+|<[a-z_]+>))+)')


def schema_text(index, op_id):
    """`lark-cli schema x.y.z` 改为 feishu_tools_describe；网关未提供的操作附说明。"""
    if '<' in op_id or op_id in index.ops:
        note = ''
    else:
        note = '（' + (index.note(op_id) if index.known(op_id) else '网关目录中没有此操作') + '）'
    return 'feishu_tools_describe（operation_id="' + op_id + '"）' + note


class Index:
    """目录、排除台账、参数名和 flag 别名的查询表。"""

    def __init__(self, operations, coverage, rows=()):
        self.ops = {op['id']: op for op in operations}
        self.aliases = {(r['service'] + '.' + r['command'], a): f['Name'] for r in rows for f in r['flags'] or [] for a in f.get('Aliases') or []}
        self.excluded = {c['id']: c['reason'] for c in coverage if c['status'] != 'adapter_ready'}
        self.services = {i.split('.')[0] for i in self.ops} | {i.split('.')[0] for i in self.excluded}
        self.domain_params = {}
        for op in operations:
            self.domain_params.setdefault(op['id'].split('.')[0], set()).update(op['input_schema'].get('properties', {}))

    def known(self, op_id):
        return op_id in self.ops or op_id in self.excluded

    def resolve(self, words, domain):
        """把命令词（docs +fetch、drive files copy、+csv-get）解析为 (operation_id, 已消耗词数)。"""
        if not words:
            return None, 0
        bases = [([words[0]], 1)] if words[0] in self.services else []
        if domain:
            bases.append(([domain], 0))
        for prefix, skip in bases:
            rest = words[skip:]
            for n in range(min(len(rest), 4), 0, -1):
                if any(w.startswith('-') for w in rest[:n]):
                    continue
                candidate = '.'.join(prefix + rest[:n])
                if self.known(candidate):
                    return candidate, skip + n
        if words[0].startswith('+'):
            matches = sorted(s + '.' + words[0] for s in self.services if self.known(s + '.' + words[0]))
            if len(matches) == 1:
                return matches[0], 1
        return None, 0

    def note(self, op_id):
        return '网关不提供：' + REASONS.get(self.excluded.get(op_id, ''), '已由网关原生能力替代')

    def ref(self, op_id):
        return '`' + op_id + '`' + ('' if op_id in self.ops else '（' + self.note(op_id) + '）')

    def param(self, flag, op_id, domain):
        key = flag.replace('-', '_')
        op = self.ops.get(op_id)
        if op and key in op['input_schema'].get('properties', {}):
            return key
        return key if key in self.domain_params.get(domain, ()) else None


class Ctx:
    """一次翻译的上下文：当前操作、领域，以及当前文件可见的主题路径。"""

    def __init__(self, index, op_id=None, domain=None, topics=None):
        self.index, self.op_id, self.domain, self.topics = index, op_id, domain, topics or {}


class Op(str):
    """shell 的控制与重定向符号；与引号内同样写法的字面量区分。"""


class ShellSyntax(ValueError):
    """网关参数无法等价表达的 shell 写法：命令替换、特殊变量、未闭合的引号等。"""


def ansi_c(body):
    """按 bash 规则解码 $'...' 的内容；未知转义保留反斜杠。"""
    out, i = [], 0
    while i < len(body):
        c, n = body[i], body[i + 1:i + 2]
        if c != '\\' or not n:
            out.append(c)
            i += 1
        elif n in ANSI_C:
            out.append(ANSI_C[n])
            i += 2
        elif n in '01234567':
            digits = re.match(r'[0-7]{1,3}', body[i + 1:]).group(0)
            out.append(chr(int(digits, 8) & 0xff))
            i += 1 + len(digits)
        elif n in 'xuU' and re.match(r'[0-9A-Fa-f]', body[i + 2:i + 3]):
            digits = re.match(r'[0-9A-Fa-f]{1,%d}' % {'x': 2, 'u': 4, 'U': 8}[n], body[i + 2:]).group(0)
            if int(digits, 16) > 0x10ffff:
                raise ShellSyntax('无效的 Unicode 转义')
            out.append(chr(int(digits, 16)))
            i += 2 + len(digits)
        elif n == 'c' and body[i + 2:i + 3]:
            out.append(chr(ord(body[i + 2]) & 0x1f))
            i += 3
        else:
            out.append(c + n)
            i += 2
    return ''.join(out)


def shell_words(text):
    """按 POSIX shell 规则拆词，返回 (词列表, 行尾注释)。

    支持单引号、双引号、$'...'（按 bash 解码转义）、反斜杠转义与续行；$VAR、${VAR} 改写为 <VAR> 占位值，
    未加引号的 <占位值> 保持原样。控制与重定向符号作为 Op 返回。命令替换、特殊变量和未闭合的引号抛出 ShellSyntax。"""
    words, word, started, comment, i, n = [], [], False, None, 0, len(text)

    def variable(j, quoted):
        m = SHELL_VAR.match(text, j)
        if m:
            return '<' + (m.group(1) or m.group(2)) + '>', m.end()
        follow = text[j + 1:j + 2]
        if not follow or follow.isspace() or (quoted and follow == '"'):
            return '$', j + 1
        raise ShellSyntax('shell 展开 $' + follow)

    while i < n:
        c = text[i]
        if c in ' \t\n':
            if started:
                words.append(''.join(word))
            word, started = [], False
            if c == '\n':
                words.append(Op('\n'))
            i += 1
        elif c == '#' and not started:
            end = text.find('\n', i)
            comment = text[i + 1:end if end >= 0 else n].strip()
            i = end if end >= 0 else n
        elif c == '\\':
            if text[i + 1:i + 2] in ('\n', ''):
                i += 2  # 续行；命令末尾的反斜杠同样视为续行
                continue
            word.append(text[i + 1])
            started, i = True, i + 2
        elif c == "'":
            end = text.find("'", i + 1)
            if end < 0:
                raise ShellSyntax('未闭合的单引号')
            word.append(text[i + 1:end])
            started, i = True, end + 1
        elif text.startswith("$'", i):
            j = i + 2
            while j < n and text[j] != "'":
                j += 2 if text[j] == '\\' else 1
            if j >= n:
                raise ShellSyntax('未闭合的 $\'')
            word.append(ansi_c(text[i + 2:j]))
            started, i = True, j + 1
        elif c == '"' or text.startswith('$"', i):
            j = i + (2 if c == '$' else 1)
            while True:
                if j >= n:
                    raise ShellSyntax('未闭合的双引号')
                d = text[j]
                if d == '"':
                    break
                if d == '\\' and text[j + 1:j + 2] in ('$', '`', '"', '\\', '\n'):
                    word.append(text[j + 1] if text[j + 1] != '\n' else '')
                    j += 2
                elif d == '$':
                    value, j = variable(j, True)
                    word.append(value)
                elif d == '`':
                    raise ShellSyntax('命令替换')
                else:
                    word.append(d)
                    j += 1
            started, i = True, j + 1
        elif c == '$':
            value, i = variable(i, False)
            word.append(value)
            started = True
        elif c == '`':
            raise ShellSyntax('命令替换')
        elif c in '|&;<>()':
            m = PLACEHOLDER.match(text, i) if c == '<' else None
            if m:
                word.append(m.group(0))
                started, i = True, m.end()
                continue
            # 2>&1 这类重定向的文件描述符并入符号
            prefix = ''.join(word) if started and ''.join(word).isdigit() and c in '<>' else ''
            if started and not prefix:
                words.append(''.join(word))
            word, started = [], False
            m = SHELL_OP.match(text, i)
            words.append(Op(prefix + m.group(0)))
            i = m.end()
        else:
            word.append(c)
            started, i = True, i + 1
    if started:
        words.append(''.join(word))
    return words, comment


def shell_open(text):
    """命令文本末尾仍在引号内或以续行反斜杠结尾时返回 True。"""
    quote, i, n = None, 0, len(text)
    while i < n:
        c = text[i]
        if quote:
            if c == '\\' and quote != "'":
                i += 1
            elif c == quote[-1]:
                quote = None
        elif c == '\\':
            if i + 1 >= n:
                return True
            i += 1
        elif c == '#' and (i == 0 or text[i - 1] in ' \t\n'):
            end = text.find('\n', i)
            if end < 0:
                break
            i = end
        elif text.startswith("$'", i):
            quote, i = "$'", i + 1
        elif c in '\'"':
            quote = c
        i += 1
    return quote is not None


def command_end(lines, i):
    """从第 i 行开始的命令结束于哪一行之前：续行与跨行引号属于同一条命令。"""
    j = i + 1
    while j < len(lines) and shell_open('\n'.join(x.rstrip() for x in lines[i:j])):
        j += 1
    return j


def split_words(text):
    """宽松拆词，用于正文中的命令引用；无法解析时返回 None。"""
    try:
        return shell_words(text)[0]
    except ShellSyntax:
        return None


def command_words(words):
    """命令路径里的点号写法（user_mailbox.drafts）拆成多个词；-h 视同 --help。"""
    out, path = [], True
    for w in words:
        if w == '-h':
            w = '--help'
        if w.startswith('-'):
            path = False
        if path and '.' in w and not w.startswith('+') and re.fullmatch(r'[a-z_.]+', w):
            out.extend(x for x in w.split('.') if x)
        else:
            out.append(w)
    return out


def local_path(value):
    return '://' not in value and not PLACEHOLDER.fullmatch(value) and ('/' in value or re.search(r'\.\w{1,5}$', value) is not None)


def convert_value(binding, value, strict):
    """按 flag 声明转换取值；返回 (值, 是否可用)。

    @path 是 CLI 读取本地文件的写法，改为 file_id；文件参数（--image、--file 等）的本地路径同样改为 file_id，
    URL 和已有的 key 保留文本。其他取值都是字面量，含 / 也不当作路径。"""
    kind = binding['type']
    reads_file = binding.get('file') or 'file' in (binding.get('input') or [])
    if value == '-' or value.startswith('@@') or (value.startswith('@') and not reads_file):
        return '<内容>', not strict
    if value.startswith('@') or (binding.get('file') and local_path(value)):
        return {'file_id': '<file_id>'}, True
    if PLACEHOLDER.fullmatch(value):
        return value, True
    if kind in ('int', 'float64', 'int_array'):
        try:
            return (float(value) if kind == 'float64' else int(value)), True
        except ValueError:
            return value, not strict
    if binding.get('json'):
        try:
            return json.loads(value), True
        except ValueError:
            return value, True
    return value, True


def parse_call(index, words, domain, strict):
    """解析去掉 lark-cli 前缀的命令，返回 (operation_id, 参数, 省略的 CLI 专用 flag)；网关未提供的操作参数为 None。

    静态 API 的单项参数 flag 放入 params，--file 放入 files。strict 模式下任何无法等价映射的部分都使整条命令失效；
    否则跳过无法映射的部分。"""
    words = command_words(words)
    op_id, used = index.resolve(words, domain)
    if not op_id:
        return None
    rest = words[used:]
    dropped = {w[2:].partition('=')[0] for w in rest if w.startswith('--') and w[2:].partition('=')[0] in CLI_ONLY}
    op = index.ops.get(op_id)
    if op is None:
        return op_id, None, dropped
    bindings = {b['name']: (key, b) for key, b in (op.get('flag_bindings') or {}).items()}
    params = op['input_schema'].get('properties', {}).get('params', {}).get('properties', {})
    file_fields = op.get('file_fields') or []
    args, i = {}, 0
    while i < len(rest):
        token = rest[i]
        i += 1
        if not token.startswith('--'):
            if strict:
                return None
            continue
        name, eq, value = token[2:].partition('=')
        name = index.aliases.get((op_id, name), name)
        if name in CLI_ONLY:
            if name in VALUE_FLAGS and not eq and i < len(rest) and not rest[i].startswith('--'):
                i += 1
            continue
        if name == 'json' and name not in bindings and not eq and (i >= len(rest) or rest[i].startswith('--')):
            continue  # CLI 的 JSON 输出开关；网关始终返回 JSON
        key, binding = bindings.get(name, (None, None))
        target = args
        if op['kind'] == 'api':
            if name in ('params', 'data', 'file'):
                key, binding = name, {'type': 'string'}
            elif name.replace('-', '_') in params:
                key = name.replace('-', '_')
                kind = params[key].get('type')
                binding = {'type': {'integer': 'int', 'number': 'float64', 'boolean': 'bool'}.get(kind, 'string')}
                target = args.setdefault('params', {})
                if not isinstance(target, dict):
                    return None
        if key is None:
            if strict:
                return None
            if not eq and i < len(rest) and not rest[i].startswith('--'):
                i += 1
            continue
        if binding['type'] == 'bool':
            target[key] = value.lower() not in ('false', '0') if eq else True
            continue
        if not eq:
            if i >= len(rest) or rest[i].startswith('--'):
                if strict:
                    return None
                continue
            value = rest[i]
            i += 1
        if op['kind'] == 'api' and key in ('params', 'data'):
            # 可以与单项参数 flag 合并；占位值保留原文。
            placeholder = PLACEHOLDER.fullmatch(value) is not None
            try:
                converted = value if placeholder else json.loads(value)
            except ValueError:
                converted = None
            if isinstance(converted, dict) and isinstance(args.get(key), dict):
                args[key].update(converted)
            elif (placeholder or isinstance(converted, dict)) and key not in args:
                args[key] = converted
            elif strict:
                return None
            continue
        if op['kind'] == 'api' and key == 'file':
            # --file 取值为 字段=路径，单个文件字段时可省略字段名。
            field, sep, path = value.partition('=')
            if not (sep and field in file_fields):
                field, path = (file_fields[0] if len(file_fields) == 1 else None), value
            if field is None or not (path.startswith('@') or local_path(path)):
                if strict:
                    return None
                continue
            args.setdefault('files', {})[field] = {'file_id': '<file_id>'}
            continue
        converted, ok = convert_value(binding, value, strict)
        if not ok:
            return None
        if binding['type'] in ('string_array', 'string_slice', 'int_array'):
            items = converted.split(',') if binding['type'] == 'string_slice' and isinstance(converted, str) else [converted]
            target.setdefault(key, []).extend(items)
        else:
            target[key] = converted
    return op_id, args, dropped


def describe_hint(index, words, domain):
    """--print-schema、--help、schema 子命令改写为发现工具调用。"""
    parameter = ''
    for i, w in enumerate(words):
        if w == '--flag-name' and i + 1 < len(words):
            parameter = words[i + 1]
        elif w.startswith('--flag-name='):
            parameter = w.split('=', 1)[1]
    head = [w for w in words if not w.startswith('--')]
    if head and head[0] == 'schema':
        head = head[1:]
        if len(head) == 1 and '.' in head[0]:
            head = head[0].split('.')
    op_id, _ = index.resolve(head, domain) if head and (head[0] in index.services or head[0].startswith('+')) else (None, 0)
    if op_id is None and len(head) == 1 and head[0] in index.services:
        return '`feishu_tools_search`（domain=' + json.dumps(head[0]) + '）'
    parts = []
    if op_id:
        if op_id not in index.ops:
            return index.ref(op_id)
        parts.append('operation_id=' + json.dumps(op_id))
    if parameter and not parameter.startswith('<'):
        parts.append('parameter=' + json.dumps(parameter.replace('-', '_')))
    return '`feishu_tools_describe`' + ('（' + '，'.join(parts) + '）' if parts else '')


def render_inline(index, op_id, args):
    if not args or op_id not in index.ops:
        return index.ref(op_id)
    return index.ref(op_id) + '（' + '，'.join(k + '=' + json.dumps(v, ensure_ascii=False) for k, v in args.items()) + '）'


def render_line(index, op_id, args):
    if op_id not in index.ops:
        return op_id + '  # ' + index.note(op_id)
    return op_id + (' ' + json.dumps(args, ensure_ascii=False) if args else '')


def special_call(index, words, domain):
    """CLI 自带的 schema、api、event 子命令；返回 None 表示不是这些写法。"""
    if not words:
        return None
    words = command_words(words)
    if words[0] == 'schema' or '--print-schema' in words or '--help' in words:
        return describe_hint(index, words, domain)
    if words[0] == 'api':
        target = ' '.join(w for w in words[1:3] if not w.startswith('-'))
        return '`' + target + '`（CLI 通用 API 调用，网关不提供）'
    if words[0] == 'event' and not index.known('event.' + (words[1] if len(words) > 1 else '')):
        return '`events.read`（网关用有界读取替代 CLI 事件订阅）'
    return None


def call_text(index, words, domain):
    """正文中的命令引用改写为 operation_id 与参数；原命令带 --dry-run 时注明网关不提供预览。"""
    special = special_call(index, words, domain)
    if special:
        return special
    parsed = parse_call(index, words, domain, False)
    if not parsed:
        return None
    op_id, args, dropped = parsed
    return render_inline(index, op_id, args) + ('（网关不提供 dry-run 预览）' if 'dry-run' in dropped and args is not None else '')


def flag_text(ctx, name, value):
    """把 CLI flag 改写为网关说法：返回 (文本, 是否代码)；None 表示保持原文。"""
    special = {'help': ('feishu_tools_describe', True), 'print-schema': ('feishu_tools_describe', True),
               'yes': ('管理员预授权（high_risk_operations）', False), 'dry-run': ('dry-run（网关不提供，改为先读取核对）', False),
               'jq': ('jq（网关不提供）', False)}
    if name in special:
        return special[name]
    if name in ('format', 'profile', 'flag-name', 'api-version'):
        return '', False
    key = ctx.index.param(name, ctx.op_id, ctx.domain)
    if key is None:
        return None
    return key + ('=' + value if value is not None else ''), True


def skill_text(name):
    domain = SKILLS.get(name) or SKILL_ALIASES.get(name)
    return '`' + domain + '` 领域' if domain else None


def translate_span(ctx, content):
    """翻译一个行内代码片段，返回替换文本。"""
    index = ctx.index
    if AS_BOT.fullmatch(content.strip()):
        return '应用身份'
    content = AS_IN_COMMAND.sub('', content).strip()
    if content in ctx.topics:
        return '主题 `' + ctx.topics[content] + '`'
    if SKILL_NAME.fullmatch(content) and skill_text(content):
        return skill_text(content)
    words = split_words(content)
    if words and not any(isinstance(w, Op) for w in words):
        head = words[1:] if words[0] == 'lark-cli' else words
        if words[0] == 'lark-cli' or words[0].startswith('+') or (words[0] in index.services and len(words) > 1):
            result = call_text(index, head, ctx.domain)
            if result:
                return result
        if words[0] in ('--print-schema', '--help'):
            return describe_hint(index, words, ctx.domain)
        if words[0].startswith('--') and len(words) <= 2:
            name, eq, value = words[0][2:].partition('=')
            if not eq and len(words) == 2:
                value, eq = words[1], '='
            result = flag_text(ctx, name, value if eq else None)
            if result is not None:
                text, code = result
                return ('`' + text + '`' if code else text) if text else ''

    def flag(m):
        result = flag_text(ctx, m.group(1), m.group(2))
        return m.group(0) if result is None else result[0]
    return '`' + FLAG.sub(flag, content) + '`'


def translate_plain(ctx, text):
    index = ctx.index

    def wiki(m):
        target, _ = index.resolve(split_words(m.group(1)) or m.group(1).split(), ctx.domain)
        return index.ref(target) if target else '`' + m.group(1) + '`'

    def cli(m):
        words = split_words(m.group(1))
        if words is None:
            return m.group(0)
        cut = next((i for i, w in enumerate(words) if isinstance(w, Op)), len(words))
        return call_text(index, words[:cut], ctx.domain) or m.group(0)

    def spaced(m):
        candidate = m.group(1) + '.+' + m.group(2)
        return index.ref(candidate) if index.known(candidate) else m.group(0)

    def plus(m):
        target, _ = index.resolve(['+' + m.group(1)], ctx.domain)
        return index.ref(target) if target else m.group(0)

    def schema(m):
        return describe_hint(index, ['--print-schema'] + (['--flag-name', m.group(1)] if m.group(1) else []), ctx.domain)

    def flag(m):
        result = flag_text(ctx, m.group(1), m.group(2))
        if result is None:
            return m.group(0)
        text, code = result
        return '`' + text + '`' if text and code else text

    def skill(m):
        return skill_text(m.group(1)) or m.group(0)

    text = AS_BOT.sub('应用身份', text)
    text = WIKI_LINK.sub(wiki, text)
    text = CLI_CALL.sub(cli, text)
    text = SPACED.sub(spaced, text)
    text = PLUS.sub(plus, text)
    text = PRINT_SCHEMA.sub(schema, text)
    text = FLAG.sub(flag, text)
    return SKILL_NAME.sub(skill, text)


def translate_text(ctx, text, layout=False, english=None):
    """翻译一段文本：代码片段与普通文本分别处理。layout=True 时保留缩进与空白（用于文档行）。
    english 按上游原文判断；英文条目中插入的网关说明改用英文。"""
    if english is None:
        english = not HAN.search(text)
    for pattern, repl in REPHRASE:
        text = pattern.sub(repl, text)
    text = SCHEMA_CALL.sub(lambda m: schema_text(ctx.index, m.group(1)), text)
    out, last = [], 0
    for m in CODE_SPAN.finditer(text):
        out.append(translate_plain(ctx, text[last:m.start()]))
        out.append(translate_span(ctx, m.group(1)))
        last = m.end()
    out.append(translate_plain(ctx, text[last:]))
    result = ''.join(out)
    if english:
        for zh, en in ENGLISH_NOTES:
            result = result.replace(zh, en)
    return result if layout else re.sub(r'[ \t]{2,}', ' ', result).strip()


def _capital(m):
    return m.group(1).upper() if m.group(1) else ''


IDENTITY_PATTERNS = [
    (re.compile(r'Identity:\s*supports\s*`user`\s*and\s*`bot`\s*(?:[;.]\s*)?([a-z])?'), _capital),
    (re.compile(r'Identity:\s*`bot`\s*only\s*\(`tenant_access_token`\)\s*(?:[;.]\s*)?([a-z])?'), _capital),
    (re.compile(r'(?:[;.]\s*)?user identity requires `[^`]+` scope on the UAT\.?', re.I), ''),
    (re.compile(r';\s*user/bot(?=\s*;|\s*$)'), ''),
    (re.compile(r'\buser/bot\b'), 'bot'),
    (re.compile(r'\s*\((?:user-only|user identity only)\)'), '（仅用户身份，网关不可用）'),
    (re.compile(r'defaults to bot \(--as bot\) or authorized user \(--as user\)'), 'defaults to the bot'),
    (re.compile(r'deprecated compatibility flag; run the corresponding \w+ command with --help for the current interface'),
     '已弃用的兼容参数，请勿使用；以 feishu_tools_describe 列出的当前参数为准'),
]


def clean_catalog_text(index, op, text):
    """清理目录描述：去掉用户身份说明，CLI flag 改为本操作的参数名。"""
    for pattern, repl in IDENTITY_PATTERNS:
        text = pattern.sub(repl, text)
    text = translate_text(Ctx(index, op['id'], op['id'].split('.')[0]), text)
    return re.sub(r'[\s;；]+$', '', text)


def clean_operation(index, op):
    op['description'] = clean_catalog_text(index, op, op['description']) or op['description']

    def walk(node):
        if isinstance(node, dict):
            for key, value in node.items():
                if key == 'description' and isinstance(value, str):
                    node[key] = clean_catalog_text(index, op, value)
                else:
                    walk(value)
        elif isinstance(node, list):
            for value in node:
                walk(value)
    walk(op['input_schema'].get('properties', {}))


def parse_affordance(md):
    """移植 lark-cli internal/affordance/mdparse.go 的固定 markdown 子集，返回 {heading: entry}。"""
    out = {}
    st = {'key': None, 'use': [], 'para': [], 'secs': [], 'sec': None, 'pending': '', 'fence': [], 'in_fence': False}

    def flush():
        if st['sec'] is not None and st['pending']:
            st['sec']['items'].append(st['pending'])
            st['pending'] = ''

    def assemble():
        if st['key']:
            out[st['key']] = {'use_when': st['use'] + ([' '.join(st['para'])] if st['para'] else []), 'sections': st['secs']}

    domain_skills = False
    for raw in md.split('\n'):
        line = raw.rstrip('\r')
        t = line.strip()
        if st['in_fence'] and not t.startswith('```'):
            st['fence'].append(line)
            continue
        if line.startswith('## '):
            flush()
            assemble()
            heading = line[3:].strip()
            domain_skills = heading == 'Skills'
            st.update(key=None if domain_skills else heading, use=[], para=[], secs=[], sec=None, pending='', fence=[], in_fence=False)
            continue
        if line.startswith('# ') or t.startswith('> skill:'):
            continue
        if line.startswith('### '):
            flush()
            st['sec'] = {'label': line[4:].strip(), 'items': [], 'cases': []}
            st['secs'].append(st['sec'])
            st.update(pending='', fence=[], in_fence=False)
            continue
        if domain_skills or not st['key']:
            continue
        if st['sec'] is None:
            if t:
                st['para'].append(t)
            elif st['para']:
                st['use'].append(' '.join(st['para']))
                st['para'] = []
            continue
        if t.startswith('```'):
            if not st['in_fence']:
                st.update(in_fence=True, fence=[])
            else:
                st['in_fence'] = False
                st['sec']['cases'].append({'description': st['pending'], 'command': '\n'.join(st['fence'])})
                st['pending'] = ''
            continue
        if t.startswith('-'):
            flush()
            st['sec']['items'].append(t[1:].strip())
        elif t:
            flush()
            st['pending'] = t.strip('* ')
    flush()
    assemble()
    return out


def translate_example(index, command, domain):
    """严格翻译 affordance 示例；只接受单条、完全可映射的命令。"""
    calls, lines, i = [], command.split('\n'), 0
    while i < len(lines):
        j = command_end(lines, i)
        t = '\n'.join(x.rstrip() for x in lines[i:j]).strip()
        i = j
        if not t or t.startswith('#'):
            continue
        words = split_words(t)
        if not words or words[0] != 'lark-cli' or any(isinstance(w, Op) for w in words):
            return None
        parsed = parse_call(index, words[1:], domain, True)
        if not parsed or parsed[1] is None or parsed[2] & {'dry-run', 'print-schema', 'jq', 'help'}:
            return None
        calls.append(parsed[:2])
    return calls[0] if len(calls) == 1 else None


def operation_guidance(index, rows, source, topic_files):
    """合成每个 Operation 的指南：affordance 的 Tips 覆盖 Go tips（与上游规则一致），再统一翻译。"""
    raw = {}
    for f in sorted((source / 'affordance').glob('*.md')):
        if f.name == 'README.md':
            continue
        for heading, entry in parse_affordance(f.read_text()).items():
            op_id = f.stem + '.' + (heading if heading.startswith('+') else heading.replace(' ', '.'))
            if op_id in index.ops:
                raw[op_id] = entry
    for row in rows:
        op_id = row['service'] + '.' + row['command']
        if op_id in index.ops and row.get('tips'):
            entry = raw.setdefault(op_id, {'use_when': [], 'sections': []})
            if not any(s['label'] == 'Tips' for s in entry['sections']):
                entry['sections'].append({'label': 'Tips', 'items': list(row['tips']), 'cases': []})
    result = {}
    for op_id, entry in sorted(raw.items()):
        domain = op_id.split('.')[0]
        ctx = Ctx(index, op_id, domain)

        def items(values):
            return [x for x in (translate_text(ctx, v) for v in values if not DROP.search(v)) if x]

        g, related, topics, examples, notes = {'use_when': items(entry['use_when'])}, [], [], [], []
        for sec in entry['sections']:
            label = sec['label']
            if label in ('Avoid when', 'Prerequisites', 'Tips'):
                g[{'Avoid when': 'avoid_when', 'Prerequisites': 'prerequisites', 'Tips': 'tips'}[label]] = items(sec['items'])
                for item in sec['items']:
                    for target in WIKI_LINK.findall(item):
                        resolved, _ = index.resolve(split_words(target) or target.split(), domain)
                        if resolved in index.ops and resolved != op_id and resolved not in related:
                            related.append(resolved)
            elif label == 'Examples':
                for case in sec['cases']:
                    parsed = translate_example(index, case['command'], domain)
                    if parsed and not DROP.search(case['description']):
                        examples.append({'description': translate_text(ctx, case['description']), 'operation_id': parsed[0], 'arguments': parsed[1]})
            elif label == 'Skills':
                for ref in sec['items']:
                    topic = topic_files.get(ref.strip('`'))
                    if topic and topic not in topics:
                        topics.append(topic)
            else:
                notes.extend(label + '：' + x for x in items(sec['items']))
        if notes:
            g.setdefault('tips', []).extend(notes)
        g.update(related=related, topics=topics, examples=examples)
        g = {k: v for k, v in g.items() if v}
        if g:
            result[op_id] = g
    return result


def _normalize(path):
    out = []
    for part in path.parts:
        if part == '..':
            if out:
                out.pop()
        elif part not in ('.', ''):
            out.append(part)
    return '/'.join(out)


def local_topics(file_path, topic_files):
    """当前文件能用相对路径写到的主题：references/x.md、x.md、../references/x.md 等。"""
    base = PurePosixPath(file_path).parent
    skill = PurePosixPath(file_path).parts[0]
    out = {}
    for path, topic in topic_files.items():
        if not path.startswith(skill + '/'):
            continue
        rel = PurePosixPath(path).relative_to(skill)
        out[rel.as_posix()] = topic
        if PurePosixPath(path).parent == base:
            out[rel.name] = topic
    return out


def rewrite_links(line, file_path, topic_files):
    """把 Skill 内的相对链接改写为指南主题或领域引用；代码片段里的链接是语法示例，保持原样。"""
    spans = [m.span() for m in CODE_SPAN.finditer(line)]

    def repl(m):
        text, target = m.group(1), m.group(2)
        if re.match(r'^[a-z]+:', target) or target.startswith('#') or any(a <= m.start() and m.end() <= b for a, b in spans):
            return m.group(0)
        path = _normalize(PurePosixPath(file_path).parent / target.split('#')[0])
        if path in topic_files:
            topic = topic_files[path]
            # 链接文字就是文件名或主题 ID 时只写一次。
            if text.strip().strip('`') in (topic, target, PurePosixPath(path).name, PurePosixPath(path).stem):
                return '主题 `' + topic + '`'
            return text + '（主题 `' + topic + '`）'
        parts = path.split('/')
        if len(parts) == 2 and parts[1] == 'SKILL.md':
            domain = SKILLS.get(parts[0]) or SKILL_ALIASES.get(parts[0])
            if text.strip('`') == parts[0]:
                return '`' + domain + '` 领域指南' if domain else text + '（CLI 专用指南，网关不提供）'
            return text + ('（`' + domain + '` 领域指南）' if domain else '（CLI 专用指南，网关不提供）')
        return text
    return MD_LINK.sub(repl, line)


def convert_command(index, text, domain):
    """把一条完整的 lark-cli 命令改写为网关调用行；无法完整、等价映射时返回 None，由调用方保留原文。"""
    try:
        words, comment = shell_words(text)
    except ShellSyntax:
        return None
    cut = next((i for i, w in enumerate(words) if isinstance(w, Op)), len(words))
    if not words or words[0] != 'lark-cli' or (cut < len(words) and words[cut] != '|'):
        return None
    special = special_call(index, words[1:cut], domain)
    if special:
        return '# ' + special.replace('`', '')
    parsed = parse_call(index, words[1:cut], domain, True)
    if not parsed:
        return None
    op_id, args, dropped = parsed
    if args is not None and 'dry-run' in dropped:
        return DRY_RUN_NOTE
    line = render_line(index, op_id, args)
    if args is not None and cut < len(words):
        line += '  # 原命令后接管道处理；网关直接返回 JSON'
    elif args is not None and 'jq' in dropped:
        line += '  # 原命令用 jq 筛选输出；网关直接返回 JSON'
    return line + ('  # ' + comment if comment else '')


def translate_block(index, lines, lang, domain):
    """翻译代码块。shell 代码块中的 lark-cli 命令（含续行和跨行引号）整条转换，无法转换时保留原文各行。"""
    shell = lang in SHELL_LANGS
    out, converted, other, i = [], 0, 0, 0
    while i < len(lines):
        j = command_end(lines, i) if shell and lines[i].strip().startswith('lark-cli ') else i + 1
        group, i = lines[i:j], j
        if shell and DROP.search('\n'.join(group)):
            continue
        rewritten = []
        for line in group:
            line = AS_IN_COMMAND.sub('', line)
            for pattern, repl in CLI_TEMPLATES:
                line = pattern.sub(repl, line)
            rewritten.append(SCHEMA_CALL.sub(lambda m: schema_text(index, m.group(1)), line))
        t = '\n'.join(x.rstrip() for x in rewritten).strip()
        if shell and t.startswith('lark-cli '):
            result = convert_command(index, t, domain)
            if result is not None:
                out.append(rewritten[0][:len(rewritten[0]) - len(rewritten[0].lstrip())] + result)
                converted += 1
                continue
        out.extend(rewritten)
        other += sum(1 for x in rewritten if x.strip() and not x.strip().startswith('#'))
    if not any(x.strip() for x in out):
        return []
    return ['```' + ('text' if shell and converted and not other else lang), *out, '```']


def tidy_tables(lines):
    """整理表格：删除按 shell 区分写法的表格与只剩表头的表格；首格为空的续行写“同上”。代码块原样保留。"""
    out, i, fence = [], 0, False
    while i < len(lines):
        if lines[i].lstrip().startswith('```'):
            fence = not fence
        if fence or lines[i].lstrip().startswith('```') or not lines[i].lstrip().startswith('|'):
            out.append(lines[i])
            i += 1
            continue
        j = i
        while j < len(lines) and lines[j].lstrip().startswith('|'):
            j += 1
        table, i = lines[i:j], j
        if len(table) < 2 or not TABLE_RULE.fullmatch(table[1]):
            out.extend(table)
            continue
        if SHELL_TABLE.search(table[0]) or len(table) == 2:
            continue
        out.extend(table[:2])
        for n, row in enumerate(table[2:]):
            cells = row.split('|')
            if n > 0 and len(cells) > 2 and not cells[1].strip():
                cells[1] = ' 同上 '
                row = '|'.join(cells)
            out.append(row)
    return out


def heading_level(line):
    level = len(line) - len(line.lstrip('#'))
    return level if 0 < level <= 6 and line[level:level + 1] == ' ' else 0


def drop_empty_sections(lines):
    """删除正文被过滤后只剩标题的小节。代码块里以 # 开头的注释不是标题。"""
    out, fence, fenced = [], False, []
    for line in lines:
        if line.lstrip().lstrip('>').lstrip().startswith('```'):
            fence = not fence
            fenced.append(True)
        else:
            fenced.append(fence)
    for i, line in enumerate(lines):
        level = 0 if fenced[i] else heading_level(line)
        if level:
            following = next((n for n in range(i + 1, len(lines)) if lines[n].strip()), None)
            if following is None:
                continue
            next_level = 0 if fenced[following] else heading_level(lines[following])
            if 0 < next_level <= level:
                continue
        out.append(line)
    return out


QUOTE_PREFIX = re.compile(r'^\s*>\s?')


def translate_document(index, md, domain, file_path, topic_files):
    if md.startswith('---\n'):
        end = md.find('\n---', 4)
        if end > 0:
            md = md[end + 4:].lstrip('\n')
    ctx = Ctx(index, None, domain, local_topics(file_path, topic_files))
    out, fence, lang, in_fence, quote, skip = [], [], '', False, '', 0

    def emit_block():
        if not skip:
            out.extend(quote + x if x else quote.rstrip() for x in translate_block(index, fence, lang, domain))

    for line in md.split('\n'):
        # 引用块中的代码块（> ```）去掉引用前缀后按代码块翻译，再加回前缀。
        m = QUOTE_PREFIX.match(line)
        body = line[m.end():] if m and (not in_fence or quote) else line
        t = body.strip()
        if t.startswith('```'):
            if not in_fence:
                in_fence, lang, fence, quote = True, t[3:].strip(), [], m.group(0) if m else ''
            else:
                in_fence = False
                emit_block()
            continue
        if in_fence:
            fence.append(body)
            continue
        level = heading_level(line)
        if skip:
            if not level or level > skip:
                continue
            skip = 0
        if level and line[level + 1:].strip() in SECTION_REPLACEMENTS:
            title, body_lines = SECTION_REPLACEMENTS[line[level + 1:].strip()]
            out.extend(['#' * level + ' ' + title, *body_lines, ''])
            skip = level
            continue
        if DROP.search(line) or CLI_ONLY_ROW.match(line):
            continue
        out.append(translate_text(ctx, rewrite_links(line, file_path, topic_files), layout=True, english=not HAN.search(line)).rstrip())
    if in_fence:
        emit_block()
    return re.sub(r'\n{3,}', '\n\n', '\n'.join(drop_empty_sections(tidy_tables(out)))).strip() + '\n'


def topic_title(md, fallback):
    for line in md.split('\n'):
        if line.startswith('# '):
            return line[2:].strip()
    return fallback


GUIDE_PREFACE = '''> 由 Feishu-MCP-Gateway 从 lark-cli `{commit}` 的 `{skill}` Skill 自动转换。
> - `docs.+fetch` 这类标识是 operation_id：具名视图调用对应工具（如 `feishu_docs_fetch`），紧凑视图用 `feishu_execute_read` / `feishu_execute_write` 传 operation_id 和 arguments。
> - CLI 参数 `--foo-bar` 对应 JSON 参数 `foo_bar`；代码块里的 `operation_id {{…}}` 行是参数示例，`<…>` 是要替换的占位值。代码块中仍以 `lark-cli` 开头的命令无法等价转换，只作参考。
> - 文中的“主题 `x`”用 `feishu_guide`（topic="x"）读取。
> - 网关固定以应用（bot）身份执行，没有 dry-run；高风险操作需要管理员预授权。
> - 本地文件改用 `{{"file_id": "…"}}`（先 `feishu_file_create` 或文件传输），输出路径填受控工作区内的相对名称；文中的本地脚本、jq、stdin 在网关中不可用。
> - 标注“网关不提供”的操作不能调用；参数以 `feishu_tools_describe` 或具名工具 schema 为准。
> - 操作结果中的 `next_command` 等后续命令是 CLI 写法，按上述规则换成对应的 operation_id 和参数再调用。

'''
TOPIC_PREFACE = '> 由 lark-cli `{commit}` 的 `{skill}` 参考文档转换；调用约定见 `{domain}` 领域指南（`feishu_guide` 的 domain 参数）。\n\n'


def build(source, commit, operations, coverage, rows, output):
    """生成 output 目录：operations.json，以及 <skill>/GUIDE.md、<skill>/<topic>.md。"""
    index = Index(operations, coverage, rows)
    files = subprocess.check_output(['git', '-C', str(source), 'ls-files', 'skills'], text=True).split('\n')
    topic_files, skills, owner = {}, {}, {}
    for skill in SKILLS:
        topics = {}
        for f in sorted(f for f in files if f.startswith('skills/' + skill + '/') and f.endswith('.md') and f != 'skills/' + skill + '/SKILL.md'):
            rel = PurePosixPath(f).relative_to('skills/' + skill)
            topic = rel.stem if rel.stem not in topics else rel.with_suffix('').as_posix().replace('/', '-')
            if topic in owner or topic in topics or topic == 'GUIDE':
                raise SystemExit('指南主题重名: ' + topic)
            topics[topic] = f
            owner[topic] = skill
            topic_files[f[len('skills/'):]] = topic
        skills[skill] = topics
    if output.exists() and any(output.iterdir()) and not (output / 'operations.json').exists():
        raise SystemExit('指南输出目录不是之前生成的指南目录，拒绝清空: ' + str(output))
    if output.exists():
        for f in sorted(output.rglob('*'), reverse=True):
            if f.is_dir():
                f.rmdir()
            else:
                f.unlink()
    output.mkdir(parents=True, exist_ok=True)
    short = commit[:12]
    index_skills = {}
    for skill, domain in SKILLS.items():
        skill_md = (source / 'skills' / skill / 'SKILL.md').read_text()
        meta = re.search(r'^description:\s*"?(.*?)"?\s*$', skill_md, re.M)
        (output / skill).mkdir()
        guide = translate_document(index, skill_md, domain, skill + '/SKILL.md', topic_files)
        (output / skill / 'GUIDE.md').write_text(GUIDE_PREFACE.format(commit=short, skill=skill) + guide)
        topics = []
        for topic, path in sorted(skills[skill].items()):
            md = (source / path).read_text()
            body = translate_document(index, md, domain, path[len('skills/'):], topic_files)
            (output / skill / (topic + '.md')).write_text(TOPIC_PREFACE.format(commit=short, skill=skill, domain=domain) + body)
            topics.append({'id': topic, 'title': translate_text(Ctx(index, None, domain), topic_title(md, topic))})
        description = translate_text(Ctx(index, None, domain), meta.group(1) if meta else '')
        index_skills[skill] = {'domain': domain, 'description': description, 'topics': topics}
    guidance = operation_guidance(index, rows, source, topic_files)
    data = {'source_commit': commit, 'skills': index_skills, 'operations': guidance}
    (output / 'operations.json').write_text(json.dumps(data, ensure_ascii=False, indent=1, sort_keys=True) + '\n')
    return {'operations_with_guidance': len(guidance), 'skills': len(index_skills), 'topics': sum(len(v['topics']) for v in index_skills.values()),
            'examples': sum(len(g.get('examples', [])) for g in guidance.values())}
