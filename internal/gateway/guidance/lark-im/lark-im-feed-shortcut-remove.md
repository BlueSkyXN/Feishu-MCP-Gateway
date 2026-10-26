> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+feed-shortcut-remove`（网关不提供：仅支持用户身份）

This skill maps to shortcut: `im.+feed-shortcut-remove`（网关不提供：仅支持用户身份）. Underlying API: `POST /open-apis/im/v2/feed_shortcuts/remove`.

## What it does

Removes one or more chats from the **current user's** feed shortcuts.

- Only **CHAT-type** shortcuts are supported (`feed_card_id` must be an `oc_xxx`).
- Batch up to **10 chat IDs per call**.
- Removing a chat that is not currently in the shortcut list is idempotent success: the call returns `ok:true`, `failure_count=0`, and no `failed_shortcuts` entry for that chat.

## Commands

```bash

# Preview the request
```

## Parameters

| Parameter | Required | Description |
|------|------|------|
| `chat_id=<oc_xxx>` | yes | open_chat_id to remove from feed shortcuts; repeatable or comma-separated; max 10 per call |

## Response

The response uses the same batch ledger as `im.+feed-shortcut-create`（网关不提供：仅支持用户身份）（主题 `lark-im-feed-shortcut-create`）: `total`, `success_count`, `failure_count`, `succeeded_shortcuts`, and `failed_shortcuts`. A non-empty `failed_shortcuts` is a partial failure: stdout carries `ok:false` with the full ledger and the process exits non-zero (currently exit `1`).

## Permissions

- Required scope: `im:feed.shortcut:write`

## Note

- To see what is currently in the shortcut list before removing, run `im.+feed-shortcut-list`（网关不提供：仅支持用户身份）（主题 `lark-im-feed-shortcut-list`）. Use `--no-detail` when you only need the `feed_card_id` values.
