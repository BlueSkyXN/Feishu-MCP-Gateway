> 由 lark-cli `7f70de0666a2` 的 `lark-mail` 参考文档转换；调用约定见 `mail` 领域指南（`feishu_guide` 的 domain 参数）。

# `mail.+forward`（网关不提供：仅支持用户身份）

转发指定邮件，自动处理：
- 主题前缀 `Fwd: `（已含前缀时不重复）
- 自动拼接标准 “Forwarded message” 区块（From/Date/Subject/To + 原文）
- 支持纯文本和 HTML 转发

> **默认草稿**：`mail.+forward`（网关不提供：仅支持用户身份） 默认保存为草稿，不会立即发送。如需立即发送，添加 `--confirm-send` 参数（仅在用户明确确认后使用）。

本 skill 对应 shortcut：`mail.+forward`（网关不提供：仅支持用户身份）。

## CRITICAL — 发送工作流（必须遵循）

**CRITICAL - 编辑邮件内容前 MUST 先用 Read 工具读取 lark-mail-html.md（主题 `lark-mail-html`），其中包含邮件书写规范**

此命令默认**只保存草稿**，不会发送邮件。转发会将原邮件内容发送给新收件人，需要发送时有两种合规方式：

**方式 A（推荐）** — 创建转发草稿（不带 `--confirm-send`）：
```text
mail.+forward  # 网关不提供：仅支持用户身份
```
→ 返回 `draft_id`

向用户展示转发摘要（被转发邮件、收件人、附加说明）；如果用户想先看效果，可引导其去飞书邮件里查看草稿。

用户明确同意后，发送该草稿：
```text
mail.user_mailbox.drafts.send  # 网关不提供：仅支持用户身份
```

**方式 B（允许）** — 用户已经明确确认收件人和内容时，可直接使用 `--confirm-send` 立即发送。

**禁止在用户未明确同意的情况下执行发送，无论是发送草稿还是直接使用 `--confirm-send`。**

## 命令

```text
# 转发邮件（默认保存为草稿）— HTML 推荐
mail.+forward  # 网关不提供：仅支持用户身份

# 转发并附加说明 + 抄送（草稿）
mail.+forward  # 网关不提供：仅支持用户身份

# 转发时插入内嵌图片（推荐：直接用相对路径，自动解析）
mail.+forward  # 网关不提供：仅支持用户身份

# 纯文本转发（仅在内容极简时使用）
mail.+forward  # 网关不提供：仅支持用户身份

# 确认发送（用户明确确认后才可使用）
mail.+forward  # 网关不提供：仅支持用户身份

# Dry Run（仅打印请求，不发送）
mail.+forward  # 网关不提供：仅支持用户身份
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `message_id=<id>` | 是 | 被转发的邮件 ID |
| `to=<email>` | 是 | 收件人邮箱。多个收件人请重复传 `to`，每次只放一个地址，参数值用单引号包住 |
| `body=<text>` | 否 | 转发时附加的说明文字。推荐使用 HTML 获得富文本排版；也支持纯文本。根据转发正文和原邮件正文自动检测 HTML。使用 `plain_text` 可强制纯文本模式。支持 `<img src="./local.png" />` 相对路径自动解析为内嵌图片（仅支持相对路径，不支持绝对路径）。与 `body_file` 互斥 |
| `--from <email>` | 否 | 发件人邮箱地址（EML From 头）。使用别名（send_as）发信时，设为别名地址并配合 `mailbox` 指定所属邮箱。默认读取邮箱主地址 |
| `mailbox=<email>` | 否 | 邮箱地址，指定草稿所属的邮箱（默认回退到 `--from`，再回退到 `me`）。当发件人（`--from`）与邮箱不同时使用。可通过 `accessible_mailboxes` 查询可用邮箱 |
| `cc=<email>` | 否 | 抄送邮箱。多个抄送请重复传 `cc`，每次只放一个地址，参数值用单引号包住 |
| `bcc=<email>` | 否 | 密送邮箱。多个密送请重复传 `bcc`，每次只放一个地址，参数值用单引号包住。与 `--event-*` 不兼容（见 `mail.+send`（网关不提供：仅支持用户身份） 日程邀请约束） |
| `plain_text` | 否 | 强制纯文本模式，忽略所有 HTML 自动检测。不可与 `inline` 同时使用。纯文本模式下也会自动追加纯文本签名（HTML 签名经 `PlainTextFromHTML` 转换，内联图片丢弃） |
| `attach=<path>` | 否 | 附件文件路径。多个附件请重复传 `attach`，每次只放一个相对路径，参数值用单引号包住；按传入顺序追加在原邮件附件之后。当附件导致 EML 总大小超过 25 MB 时，超出部分自动上传为超大附件（HTML 邮件插入下载卡片，纯文本邮件追加下载链接），单个文件上限 3 GB |
| `inline=<json>` | 否 | 高级用法：手动指定内嵌图片 CID 映射。多个 inline 图片请重复传 `inline`，每次只放一个 JSON object，并用单引号包住：`'{"cid":"mycid","file_path":"./logo.png"}'`。`file_path` 必须是相对路径；CID 应唯一，例如随机十六进制字符串；在 body 中用 `<img src="cid:mycid">` 引用。推荐直接在 `body` 中使用 `<img src="./path" />`（自动解析）。不可与 `plain_text` 同时使用 |
| `--signature-id <id>` | 否 | 签名 ID。附加邮箱签名到转发正文与引用块之间。运行 `mail.+signature`（网关不提供：仅支持用户身份） 查看可用签名。与 `--no-signature` 互斥 |
| `--no-signature` | 否 | 跳过默认签名自动追加。与 `--signature-id` 互斥，同时使用时返回参数校验错误（退出码 2） |
| `--priority <level>` | 否 | 邮件优先级：`high`、`normal`、`low`。省略或 `normal` 时不设置优先级 |
| `--event-summary <text>` | 否 | 日程标题。设置此参数即在邮件中嵌入日程邀请。需同时设置 `--event-start` 和 `--event-end` |
| `--event-start <time>` | 条件必填 | 日程开始时间（ISO 8601） |
| `--event-end <time>` | 条件必填 | 日程结束时间（ISO 8601） |
| `--event-location <text>` | 否 | 日程地点 |
| `--confirm-send` | 否 | 确认发送转发（默认只保存草稿）。仅在用户明确确认后使用 |

> **日程约束**：`--event-*` 与 `--send-time` 不可同时使用；`to` 和 `cc` 收件人自动成为日程参与者（ATTENDEE），`bcc` 收件人不计入参与者。
| `--send-time <timestamp>` | 否 | 定时发送时间，Unix 时间戳（秒）。需至少为当前时间 + 5 分钟。配合 `--confirm-send` 使用可定时发送邮件 |
| `--request-receipt` | 否 | 请求已读回执（RFC 3798 Message Disposition Notification）。在出站 EML 里写 `Disposition-Notification-To: <sender>` 头。收件人的邮件客户端可能弹出提示、自动发送或忽略——送达不保证 |
| dry-run（网关不提供，改为先读取核对） | 否 | 仅打印请求，不执行 |

## 返回值

默认（草稿模式）：
```json
{
  "ok": true,
  "data": {
    "draft_id": "草稿ID",
    "tip": "draft saved. To send: lark-cli mail user_mailbox.drafts send --params '{...}'"
  }
}
```

`--confirm-send` 模式：
```json
{
  "ok": true,
  "data": {
    "message_id": "邮件ID",
    "thread_id": "会话ID"
  }
}
```

可选字段：

- `automation_send_disable_reason`：发送被邮箱自动化设置拦截时返回的原因
- `automation_send_disable_reference`：发送被拦截时的草稿打开链接
- `recall_available` / `recall_tip`：发送成功后若返回可撤回提示，按需参考 lark-mail-recall（主题 `lark-mail-recall`）

字段语义：

- 若返回中包含 `automation_send_disable_reason` / `automation_send_disable_reference`，说明转发未真正发出，而是被邮箱设置拦截。此时应直接向用户展示原因和草稿打开链接，不要继续假设已经发送成功
- 若返回中包含 `recall_available: true`，说明该邮件支持撤回；仅当用户明确要求撤回时，读取 lark-mail-recall（主题 `lark-mail-recall`） 并执行撤回流程

## 典型场景

### 场景 1：用户说"把这封邮件转发给 Bob"（只创建草稿）
```text
mail.+forward  # 网关不提供：仅支持用户身份
```
→ 返回 `draft_id`，告诉用户转发草稿已创建。

### 场景 2：用户说"转发给 Bob 并发送"（需要发送）
```text
# 方式 A: 创建转发草稿
mail.+forward  # 网关不提供：仅支持用户身份

# 用户确认后发送
mail.user_mailbox.drafts.send  # 网关不提供：仅支持用户身份

# 方式 B: 用户已明确确认时，直接发送
mail.+forward  # 网关不提供：仅支持用户身份
```

### 场景 3：用户说"下午 3 点转发给 Bob"（定时发送）
```text
# Step 1: 创建转发草稿
mail.+forward  # 网关不提供：仅支持用户身份

# Step 3: 用户确认后定时发送（send_time 为 Unix 时间戳，需至少当前时间 + 5 分钟）
mail.user_mailbox.drafts.send  # 网关不提供：仅支持用户身份
```

### 场景 4：用户说"等等，先不转发了"（取消定时发送）
```text
# 取消定时发送（取消后邮件变回草稿）
mail.user_mailbox.drafts.cancel_scheduled_send  # 网关不提供：仅支持用户身份
```
→ 取消成功后邮件恢复为草稿状态，用户可重新编辑或在之后重新发送。

## 转发整个会话

`mail.+forward`（网关不提供：仅支持用户身份） 操作的是单封邮件（`message_id`），但转发整个会话时应 forward **会话中最后一条消息**，因为邮件客户端会将完整的回复链嵌套在最新一条中。典型流程：

```text
# 1. 用 +triage 或 +thread 找到会话
mail.+thread {"thread_id": "<THREAD_ID>", "html": false}

# 3. 转发该消息
mail.+forward  # 网关不提供：仅支持用户身份
```

## 实现说明

- 自动拉取原邮件后构建转发内容。
- 纯文本模式下会生成标准转发头块并附上原文文本。
- HTML 模式下会生成结构化转发块并尽量保留原 HTML 正文。

## 发送后跟进

转发发送后，分两种情况处理：

- 若返回中有 `automation_send_disable_reason` / `automation_send_disable_reference`：说明发送被邮箱设置拦截，应直接告诉用户原因并提供草稿打开链接，**不要**调用 `send_status`
- 若用户基于发送结果要求撤回，先读取 lark-mail-recall（主题 `lark-mail-recall`），再执行撤回流程

**1. 确认投递状态**（仅立即发送且返回非空 `message_id` 时必须）

用返回的 `message_id` 查询投递状态：

```text
mail.user_mailbox.messages.send_status  # 网关不提供：仅支持用户身份
```

状态码：1=正在投递, 2=投递失败重试, 3=退信, 4=投递成功, 5=待审批, 6=审批拒绝。向用户简要报告投递结果，异常状态需重点提示。

**1b. 定时发送（指定了 `--send-time`）**

定时发送不会立即产生 `message_id`，因此 `send_status` 在定时发送成功后会返回"待发送"状态，**不建议在定时发送后立即查询**。可在预定发送时间后再查询。

如需取消定时发送：

```text
mail.user_mailbox.drafts.cancel_scheduled_send  # 网关不提供：仅支持用户身份
```

**取消后邮件会变回草稿**，可继续编辑或在之后重新发送。

**2. 标记已读**（可选）— 询问用户是否需要将原邮件标记为已读。如果用户同意：

```text
mail.+message-modify  # 网关不提供：仅支持用户身份
```

## 编辑转发草稿

`mail.+forward`（网关不提供：仅支持用户身份） 创建的草稿正文包含引用区（原邮件的引用块）。如果需要编辑转发草稿的正文，**必须通过 `patch_file` 使用 `set_reply_body` op**，它仅替换用户撰写部分，自动保留引用区。value 只传新的用户撰写内容，不要包含引用区。

```bash
# 编辑转发草稿正文（自动保留引用区）
cat > ./patch.json << 'EOF'
{ "ops": [{ "op": "set_reply_body", "value": "<p>修改后的转发附言</p>" }] }
EOF
mail.+draft-edit  # 网关不提供：仅支持用户身份
```

如果用户要修改引用区内容或去掉引用区，则使用 `set_body` 全量替换。

## 相关命令

- `mail.+send`（网关不提供：仅支持用户身份） — 发送新邮件
- `mail.+reply`（网关不提供：仅支持用户身份） — 回复邮件
- `mail.user_mailbox.messages.get` — 查看邮件详情
