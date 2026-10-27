> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+flag-cancel` (not available in the gateway: user identity only)

This skill maps to shortcut: `im.+flag-cancel` (not available in the gateway: user identity only). Underlying API: `POST /open-apis/im/v1/flags/cancel`.

## Double-Cancel Behavior (Important)

A message can have flags on both layers simultaneously:
- Message layer: `(default, message)`
- Feed layer: `(thread, feed)` or `(msg_thread, feed)` depending on chat type

**When no `--flag-type` is specified, the shortcut performs best-effort double-cancel**: the message-layer flag is always removed; the feed-layer flag is also removed when the chat type can be determined (otherwise a warning is printed on stderr and the feed layer is skipped). The server handles cancel requests for non-existent flags idempotently, so this is safe.

**Feed layer item_type is determined by chat_mode**:
- Topic-style chat (`chat_mode=topic`) → `item_type=thread`
- Regular chat (`chat_mode=group`) → `item_type=msg_thread`

## Commands

```bash

# Preview request
```

## Parameters

| Parameter | Required | Description |
|------|------|------|
| `message_id=<om_xxx>` | Required | Message ID |
| `--flag-type <name>` | No | `message` or `feed`; **when omitted, best-effort double-cancel of both layers** |
| `--item-type <name>` | No | `default\|thread\|msg_thread`; required when `--flag-type feed` |

## Idempotency

The server doesn't return an error for cancel requests when the flag doesn't exist, so repeated `+cancel` calls are idempotent.

## Permissions

- Required scopes: `im:feed.flag:write`, `im:message.group_msg:get_as_user`, `im:message.p2p_msg:get_as_user`, `im:chat:read`
- The message/chat read scopes are used by the default double-cancel path to auto-detect the feed-layer item type.

## Note

- **Do not call `im.+flag-list` (not available in the gateway: user identity only) for verification**: If the cancel API returns success, the flag is removed. Calling `im.+flag-list` (not available in the gateway: user identity only) to verify is expensive (requires full pagination) and unnecessary.

## Finding Message ID Efficiently

If you have message content but not the message ID:

1. **Use `im.+messages-search`** to find the message by content, then extract `message_id` from the result
2. **Do NOT use `im.+flag-list` (not available in the gateway: user identity only)** to find the message — it requires full pagination and is very inefficient

```bash
# Search by message content to find message_id
```
