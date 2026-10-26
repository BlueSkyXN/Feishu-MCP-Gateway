> 由 lark-cli `7f70de0666a2` 的 `lark-mail` 参考文档转换；调用约定见 `mail` 领域指南（`feishu_guide` 的 domain 参数）。

# `mail.+message-modify`（网关不提供：仅支持用户身份）

`mail.+message-modify`（网关不提供：仅支持用户身份） is the preferred shortcut for changing labels, read-state labels, or folder placement on existing messages.

Use it instead of raw `user_mailbox.messages batch_modify` when the operation targets concrete `message_id` values from `mail.+triage`, `mail.+message`, or `mail.+messages`.

## Common Commands

```text
mail.+message-modify  # 网关不提供：仅支持用户身份
mail.+message-modify  # 网关不提供：仅支持用户身份
mail.+message-modify  # 网关不提供：仅支持用户身份
mail.+message-modify  # 网关不提供：仅支持用户身份
mail.+message-modify  # 网关不提供：仅支持用户身份
```

## Flags

| Flag | Required | Notes |
| --- | --- | --- |
| `mailbox` | No | Mailbox that owns the messages. Defaults to `me`. |
| `message_ids` | Yes | `string_array`; supports comma-separated values and repeated flags. |
| `add_label_ids` | No | Adds labels. System labels `unread`, `important`, `other`, `flagged` normalize to upper case. |
| `remove_label_ids` | No | Removes labels. Cannot overlap with `add_label_ids`. |
| `add_folder` | No | Moves to one folder. `inbox`, `sent`, `spam`, `archive`, `archived` normalize to system folder IDs. |

`TRASH` is intentionally rejected by this shortcut. Use `mail.+message-trash`（网关不提供：仅支持用户身份） for soft deletion.

## Behavior

- Message IDs are locally validated, de-duplicated in first-seen order, and sent in batches of 20.
- Custom label IDs are checked with `labels.get`; custom folder IDs are checked with `folders.get`.
- If no label or folder operation is requested, the command succeeds locally, emits all message IDs as `success_message_ids`, and makes no POST request.
- Single batch POST failures mark every message in that batch with the same failure reason; later batches still run.
- JSON output is intentionally compact:

```json
{
  "success_message_ids": ["id1"],
  "failed_message_ids": [
    {"message_id": "id2", "reason": "api error"}
  ]
}
```

## When Raw API Is Still Appropriate

Use raw `mail.user_mailbox.messages.batch_modify` only when you need a request shape that the shortcut intentionally does not expose, or when reproducing backend/API behavior exactly for diagnostics.
