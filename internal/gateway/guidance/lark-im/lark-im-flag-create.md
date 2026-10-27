> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+flag-create` (not available in the gateway: user identity only)

This skill maps to shortcut: `im.+flag-create` (not available in the gateway: user identity only). Underlying API: `POST /open-apis/im/v1/flags`.

## Default Behavior

- **Message-layer flag** (default): `item_type=default, flag_type=message`
- **Feed-layer flag**: Use `--flag-type feed` — automatically detects chat type to determine `item_type`:
  - Topic-style chat (`chat_mode=topic`) → `item_type=thread`
  - Regular chat (`chat_mode=group`) → `item_type=msg_thread`

## Commands

```bash

# Preview request (dry-run, doesn't send)
```

## Parameters

| Parameter | Required | Description |
|------|------|------|
| `message_id=<om_xxx>` | Required | Message ID |
| `--flag-type <name>` | No | `message` (default) or `feed` |
| `--item-type <name>` | No | Override auto-detection: `default\|thread\|msg_thread` (rarely needed) |

## Valid Combinations

The server only accepts these `(item_type, flag_type)` pairs:

- `(default, message)` — regular message flag
- `(thread, feed)` — feed flag in topic-style chat
- `(msg_thread, feed)` — feed flag in regular chat

## Permissions

- Required scopes: `im:feed.flag:write`, `im:message.group_msg:get_as_user`, `im:message.p2p_msg:get_as_user`, `im:chat:read`
- The message/chat read scopes are used when `--flag-type feed` is used without explicit `--item-type` so the CLI can auto-detect chat type.

## Note

- **Do not call `im.+flag-list` (not available in the gateway: user identity only) for verification**: If the create API returns success, the flag is created. Calling `im.+flag-list` (not available in the gateway: user identity only) to verify is expensive (requires full pagination) and unnecessary.

## Finding Message ID Efficiently

If you have message content but not the message ID:

1. **Use `im.+messages-search`** to find the message by content, then extract `message_id` from the result
2. **Do NOT use `im.+flag-list` (not available in the gateway: user identity only)** to find the message — it requires full pagination and is very inefficient

```bash
# Search by message content to find message_id
```
