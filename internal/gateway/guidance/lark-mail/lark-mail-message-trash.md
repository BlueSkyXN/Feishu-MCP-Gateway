> 由 lark-cli `7f70de0666a2` 的 `lark-mail` 参考文档转换；调用约定见 `mail` 领域指南（`feishu_guide` 的 domain 参数）。

# `mail.+message-trash` (not available in the gateway: user identity only)

`mail.+message-trash` (not available in the gateway: user identity only) is the preferred shortcut for soft-deleting existing messages.

Use it after obtaining real `message_id` values from `mail.+triage`, `mail.+message`, or `mail.+messages`, and after the user has confirmed the deletion preview.

## Common Commands

```text
mail.+message-trash  # 网关不提供：仅支持用户身份
mail.+message-trash  # 网关不提供：仅支持用户身份
mail.+message-trash  # 网关不提供：仅支持用户身份
```

## Flags

| Flag | Required | Notes |
| --- | --- | --- |
| `mailbox` | No | Mailbox that owns the messages. Defaults to `me`. |
| `message_ids` | Yes | `string_array`; supports comma-separated values and repeated flags. |

## Behavior

- Message IDs are locally validated, de-duplicated in first-seen order, and sent in batches of 20.
- The shortcut calls `POST /open-apis/mail/v1/user_mailboxes/<mailbox>/messages/batch_trash` sequentially.
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

Use raw `mail.user_mailbox.messages.batch_trash` only when reproducing backend/API behavior exactly for diagnostics. For normal soft deletion, prefer this shortcut because it handles validation, batching, compact output, and administrator pre-authorization (high_risk_operations) confirmation consistently.
