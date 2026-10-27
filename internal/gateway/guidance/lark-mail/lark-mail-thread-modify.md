> 由 lark-cli `7f70de0666a2` 的 `lark-mail` 参考文档转换；调用约定见 `mail` 领域指南（`feishu_guide` 的 domain 参数）。

# `mail.+thread-modify`

如果操作对象是具体邮件 `message_id`，不是整个会话，使用 `mail.+message-modify`（网关不提供：仅支持用户身份）（主题 `lark-mail-message-modify`）。

## 命令

```text
# 给多个会话添加未读标签
mail.+thread-modify {"thread_ids": ["<thread_id1>,<thread_id2>"], "add_label_ids": ["unread"]}

# 移除星标标签
mail.+thread-modify {"thread_ids": ["<thread_id>"], "remove_label_ids": ["FLAGGED"]}

# 归档会话
mail.+thread-modify {"thread_ids": ["<thread_id>"], "add_folder": "archive"}

# 指定公共邮箱或共享邮箱
mail.+thread-modify {"mailbox": "shared@example.com", "thread_ids": ["<thread_id>"], "add_folder": "folder_xxx"}

# 使用 bot 身份时必须显式指定邮箱
mail.+thread-modify {"mailbox": "user@example.com", "thread_ids": ["<thread_id>"], "add_folder": "archive"}

# Dry Run：只预览请求，不执行
mail.+thread-modify {"thread_ids": ["<thread_id>"], "add_label_ids": ["custom_label_id"]}
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `mailbox=<email>` | 否 | 会话所属邮箱，默认 `me`；使用 应用身份 时必须显式传邮箱地址 |
| `thread_ids=<ids>` | 是 | 会话 ID 列表，支持逗号分隔和重复传参；超过 20 个时自动分批提交 |
| `add_label_ids=<ids>` | 否 | 要添加的标签 ID。系统标签可传 `unread` / `important` / `other` / `flagged`；自定义标签传标签 ID |
| `remove_label_ids=<ids>` | 否 | 要移除的标签 ID。不能与 `add_label_ids` 传入重复标签 |
| `add_folder=<id>` | 否 | 要移动到的文件夹。系统文件夹可传 `inbox` / `sent` / `spam` / `archive` / `archived`；自定义文件夹传文件夹 ID |

`add_label_ids`、`remove_label_ids`、`add_folder` 至少传一个。

`TRASH` 不允许通过本 shortcut 作为目标文件夹传入。需要软删除会话时，使用 `mail.+thread-trash`（主题 `lark-mail-thread-trash`），并在用户确认后加 管理员预授权（high_risk_operations） 执行。

`READ_RECEIPT_REQUEST` / `read_receipt_request` 不允许通过本 shortcut 添加或移除。已读回执请求必须先读取具体 message、确认用户意图，再使用 `mail.+send-receipt`（网关不提供：仅支持用户身份）（主题 `lark-mail-send-receipt`） 或 `mail.+decline-receipt`（网关不提供：仅支持用户身份）（主题 `lark-mail-decline-receipt`）。

## 注意事项

- `thread_id` 必须来自 `mail.+triage`、`mail.+message`、`mail.+thread`、会话列表或搜索等真实查询结果；不要用数字主键或占位符。
- 命令在本地解析逗号分隔和重复 flag，按首次出现顺序去重，并按 20 个一批提交。
- 单个 batch 请求失败时，该批次的所有 `thread_id` 都记录为同一个失败原因；后续批次继续执行。

## 返回值

返回示例：

```json
{
  "success_thread_ids": ["thread_id1"],
  "failed_thread_ids": [
    {"thread_id": "thread_id2", "reason": "api error"}
  ]
}
```

## 原生 API 适用场景

只有在需要精确复现后端/API 行为做诊断，或需要 shortcut 未暴露的请求结构时，才直接调用 `mail.user_mailbox.threads.batch_modify`。普通会话整理优先使用本 shortcut，因为它内置了 ID 校验、分批、批量输出和 dry-run 预览。

## 相关命令

- `mail.+triage` — 浏览邮件摘要，获取 `thread_id`
- `mail.+thread` — 读取完整会话
- `mail.+message-modify`（网关不提供：仅支持用户身份） — 按 `message_id` 修改具体邮件
- `mail.+thread-trash` — 按 `thread_id` 软删除会话
