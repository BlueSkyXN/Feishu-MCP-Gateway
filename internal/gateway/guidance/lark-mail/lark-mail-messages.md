> 由 lark-cli `7f70de0666a2` 的 `lark-mail` 参考文档转换；调用约定见 `mail` 领域指南（`feishu_guide` 的 domain 参数）。

# `mail.+messages`

通过传入逗号分隔的 `message_id` 列表，一次性读取多封邮件的完整内容。

超过 20 个 ID 可以直接传入 CLI；CLI 会按 20 个 ID 自动拆批并合并输出，不需要手动拆批，也不要逐封循环调用 `mail.+message`。

本 shortcut 是 `mail.+message` 的批量版本。每个返回的 `messages[]` 项使用与 `mail.+message` 相同的归一化结构：安全元数据字段直接透传，正文和辅助字段由 shortcut 派生。

优先使用本 shortcut，因为：
- 正文字段已 base64url 解码
- 每条邮件的输出结构已归一化
- 不可用的 message ID 会被显式列出

本 skill 对应 shortcut `mail.+messages`；每条返回的邮件使用与 `mail.+message` 相同的规则归一化输出。

## 命令

```text
# 读取多封邮件（默认包含 HTML 正文）
mail.+messages {"message_ids": "<id1>,<id2>,<id3>"}

# 仅纯文本正文（更小的负载，适合 AI 处理）
mail.+messages {"message_ids": "<id1>,<id2>,<id3>", "html": false}

# 指定邮箱
mail.+messages {"mailbox": "user@example.com", "message_ids": "<id1>,<id2>"}

# JSON 输出
mail.+messages {"message_ids": "<id1>,<id2>"}

# Dry Run
mail.+messages {"message_ids": "<id1>,<id2>"}
```

## 参数

| 参数 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `message_ids=<id1>,<id2>,<id3>` | 是 | — | 逗号分隔的邮件 ID 列表；超过 20 个 ID 时 CLI 自动按 20 拆批并合并输出 |
| `mailbox=<email>` | 否 | 当前用户 | 邮箱地址（`user_mailbox_id`） |
| `html` | 否 | true | 是否返回 HTML 正文（`false` 仅返回纯文本，减少带宽） |

## 返回值

成功时返回 `{"ok": true, "data": ...}` 结构，`data` 字段包含：

```json
{
  "messages": [
    { "...与 +message 输出结构相同..." }
  ],
  "total": 1,
  "unavailable_message_ids": ["msg-2"]
}
```

顶层字段：

| 字段 | 说明 |
|------|------|
| `messages` | 返回的邮件列表，顺序与请求的 `message_ids` 一致，排除 API 未返回的 ID |
| `total` | 成功返回的邮件数量 |
| `unavailable_message_ids` | 请求了但 Mail API 未返回详情的 ID 列表 |

每个 `messages[]` 项使用与 `mail.+message`（主题 `lark-mail-message`） 相同的结构。完整字段列表参见 `mail.+message` 字段说明（主题 `lark-mail-message`） 和 `mail.+message` security_level（主题 `lark-mail-message`）。

> 注意：使用  获取结构化输出。所有 JSON 输出统一包裹在 `{"ok": true, "data": ...}` 结构中。

## 注意事项

- **JSON 输出可直接使用**，可直接读取，无需额外编码转换。
- 只需读取一封邮件时请使用 `mail.+message`。
- CLI 每 20 个 ID 拆成一次调用并合并输出，不需要为大列表手动拆请求。
- JSON 输出中 `messages[].body_html` 里的 `<` / `>` 可能显示为 `\u003c` / `\u003e`（JSON 安全转义，内容不变，`jq -r` 可还原）。
- `mail.+messages` 仅返回附件元数据。如后续步骤需要下载 URL，请针对特定的 `message_id` 和 `attachment_ids` 调用原生附件 URL API。
- 与 `mail.+message` 一样，普通附件和内嵌图片都出现在 `messages[].attachments[]` 中，使用同一个 `user_mailbox.message.attachments download_url` API。

## 典型场景

### 批量摘要多封已知邮件

```text
# 一次性读取多封邮件
mail.+messages {"message_ids": "<id1>,<id2>,<id3>", "html": false}

# 让 LLM 分析 .data.messages[].body_plain_text 并生成分组摘要
```

### 对比多封邮件内容后决策

```text
# 获取多封邮件的归一化输出
mail.+messages {"message_ids": "<id1>,<id2>", "html": false}

# 检查 subject/from/body_preview 或 body_plain_text，对比意图和下一步操作
```

## 相关命令

- `mail.+message` — 读取单封邮件
- `mail.+thread` — 读取会话中所有邮件
- `mail.+reply`（网关不提供：仅支持用户身份） — 回复邮件
- `mail.+forward`（网关不提供：仅支持用户身份） — 转发邮件
- `mail.user_mailbox.message.attachments.download_url` — 按需获取邮件附件/图片下载 URL
