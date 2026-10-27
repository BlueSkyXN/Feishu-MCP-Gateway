> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# IM message read status

Use two focused shortcuts for message read-status queries:

- `im.+messages-read-status` (not available in the gateway: user identity only) queries whether the current user has read 1–50 messages.
- `im.+message-read-users` lists users who have read one message and supports automatic pagination.

## Identity and scopes

| Shortcut | Identity | Scope |
|---|---|---|
| `im.+messages-read-status` (not available in the gateway: user identity only) | user only | `im:message:readonly` (recommended), `im:message`, or `im:message:get_as_user` |
| `im.+message-read-users` | user | `im:message:readonly` (recommended), `im:message`, `im:message:basic`, or `im:message:get_as_user` |
| `im.+message-read-users` | bot | `im:message:readonly` |

For `im.+message-read-users`, the caller must still be in the chat. A user can query only messages they sent within the last seven days, while a bot can query only messages sent by that bot within the last seven days.
The user scopes in the table are alternatives; the CLI preflight uses `im:message:readonly` because it is the least-privileged regular OAuth scope supported by this endpoint.

## Batch query the current user's read status

```bash
# Preview one request

# Execute with a user access token
```

The command accepts 1–50 comma-separated `om_` message IDs. The three scopes above are alternatives; any one is sufficient, and the CLI recommends the least-privileged OAuth scope `im:message:readonly`. The response keeps the OpenAPI response unchanged:

- `items[].message_id` and `items[].is_read` contain statuses the server could determine.
- `invalid_message_ids` contains messages that do not exist, are not visible to the current user, or do not support this query. The API deliberately does not expose a more specific reason.

## List users who read one message

```text
# Fetch one page as the current user

# Fetch every page as a bot, bounded to ten pages by default
im.+message-read-users {"message_id": "om_xxx", "user_id_type": "open_id", "page_all": true}
```

Pagination flags:

- `page_size`: 1–100, default 100.
- `page_token`: start from a known cursor.
- `page_all`: continue until the endpoint is exhausted.
- `page_limit`: maximum pages with `page_all`; default 10, range 1–1000.
- `page_delay`: delay in milliseconds between pages; default 200, and 0 disables the delay.

The command preserves each server item, including `user_id_type`, `user_id`, `timestamp`, and `tenant_key`. Pagination metadata reports whether the endpoint was exhausted and retains the next token when a bounded run can be resumed.

## Raw API commands

When Registry MR !128 is published, the corresponding raw commands remain available:

Prefer the shortcuts for flag validation, identity-specific scope hints, and read-users auto-pagination.

## Troubleshooting

| Symptom | Meaning | Action |
|---|---|---|
| Missing `im:message:readonly` or `im:message` | A regular OAuth scope has not been granted | Follow the CLI authorization hint to grant one supported scope |
| Missing a user read scope | No supported regular OAuth scope has been granted | Grant `im:message:readonly` and retry |
| Bot permission denied | The application lacks a bot scope | Open the `console_url` from the typed error and enable the requested scope |
| Empty read-user list | No user has read the message, or sender/time constraints are not met | Verify chat membership, the message sender, and the seven-day window |

## References

- `im` domain指南
