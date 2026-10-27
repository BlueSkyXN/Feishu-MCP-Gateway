> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+chat-list`

List chats the current user (or bot, with app identity) is a member of. **Not a search API — there is no `query` parameter; the call always returns the full member list, paginated.** For keyword-based lookup (e.g. find a group by name or by member), use `im.+chat-search` (topic `lark-im-chat-search`) instead.

This skill maps to the shortcut: `im.+chat-list` (internally calls `GET /open-apis/im/v1/chats`).

## Commands

```text
# List the user's chats (default sort: create_time, ascending)
im.+chat-list

# Sort by recent activity (most recently active first)
im.+chat-list {"sort": "active_time"}

# Limit page size
im.+chat-list {"page_size": 50}

# Pagination
im.+chat-list {"page_token": "xxx"}

# Fetch multiple pages automatically, up to 10 pages by default
im.+chat-list {"page_all": true}

im.+chat-list {"exclude_muted": true}

# JSON output
im.+chat-list

# Preview the request without executing it
# 网关不提供 dry-run 预览，此示例省略

# Same, using repeat flag instead of CSV

```

## Parameters

| Parameter | Required | Limits | Description |
|------|------|------|------|
| `user_id_type=<type>` | No | `open_id` (default), `union_id`, `user_id` | ID type used for `owner_id` in the response |
| `sort=<field>` | No | `create_time` (default, ascending), `active_time` (descending) | Result ordering |
| `page_size=<n>` | No | 1-100, default 20 | Number of results per page |
| `page_token=<token>` | No | - | Starting cursor, normally returned by a previous response |
| `page_all` | No | - | Automatically fetch and merge subsequent pages; capped by `page_limit` |
| `page_limit=<n>` | No | 1-1000, default 10 | Maximum pages fetched by `page_all` |

With `page_all`, `page_token` sets the starting cursor. If `meta.pagination.complete=false`, resume from `meta.pagination.next_token` or raise `page_limit`.

## Output Fields

| Field | Description |
|------|------|
| `chat_id` | Chat ID (`oc_xxx` format) |
| `name` | Chat name |
| `description` | Chat description |
| `owner_id` | Owner ID (type controlled by `user_id_type`) |
| `external` | Whether the chat is external |
| `chat_status` | Chat status (`normal` / `dissolved` / `dissolved_save`) |
| `chat_mode` | Chat mode discriminator: `group` (regular) / `topic` (topic group) / `p2p` (single chat) |
| `p2p_target_type` | Peer type, e.g., `user` |
| `p2p_target_id` | Peer ID (type controlled by `user_id_type`) |

## Including p2p single chats

Default behavior lists groups only — same as before this feature. To include p2p, pass `types`:

| User intent | Call | Identity |
|---|---|---|
| "list my p2p chats" / 我的单聊 / 我跟谁有 1v1 | `types=p2p` | **user only** |
| "all my chats" / 全部聊天 / 所有会话 (ambiguous) | `types=p2p,group` | **user only** |

For p2p rows in the response: `name` is the peer's display name, `owner_id` follows group semantics, `chat_mode = "p2p"`, and `p2p_target_type` / `p2p_target_id` identify the peer.

## Bot identity and p2p

`tenant_access_token` cannot list p2p chats — to protect user privacy, bot identity is not permitted to enumerate p2p single chats. Behavior under app identity:

- `types=p2p` → rejected at validation time with an actionable error; no request is sent.
- `types=p2p,group` → CLI strips `p2p` and sends `types=group`. Request proceeds; only groups are returned. The strip is a **request-level adjustment**, surfaced two ways so neither humans nor agents miss it:
    - **stdout JSON**: a top-level `notices` array gains a structured entry:
```json
      {
        "chats": [...],
        "notices": [
          { "code": "bot_strip_p2p", "message": "To protect user privacy, bot identity cannot list p2p chats; …" }
        ]
      }
```
    - The `filter` slot stays scoped to `exclude_muted`; `notices` is a separate top-level key, so the two never collide and no priority is needed when both fire.
    - DryRun emits the same stderr warning so a previewed request truthfully reflects what Execute will send (parity with `shortcuts/drive/drive_search.go`).
- `types=group` → accepted, returns groups normally.
- app identity (no `types`) → unchanged, returns groups.

## Filtering muted chats

When the flag is set, the JSON envelope gains a `filter` sub-object (absent otherwise, so existing consumers are unaffected); `fetched_count == returned_count + filtered_count` always holds:

```json
{
  "chats": [...],
  "filter": {
    "applied": "exclude_muted",
    "fetched_count": 20,
    "returned_count": 17,
    "filtered_count": 3,
    "hint": "Filtered out 3 muted chat(s) on this page (17 remaining); use --page-token to fetch more."
  }
}
```

## Usage Scenarios

### Scenario 1: List my recent chats

```text
im.+chat-list {"sort": "active_time", "page_size": 10}
```

### Scenario 2: List my non-muted chats sorted by activity

```text
im.+chat-list {"sort": "active_time", "exclude_muted": true}
```

### Scenario 3: Iterate all my chats programmatically

```bash
TOKEN=""
while :; do
  RESP=$(lark-cli im +chat-list --page-size 100 --page-token "$TOKEN" --format json)
  echo "$RESP" | jq -r '.data.chats[].chat_id'
  HAS_MORE=$(echo "$RESP" | jq -r '.data.has_more')
  [ "$HAS_MORE" = "true" ] || break
  TOKEN=$(echo "$RESP" | jq -r '.data.page_token')
done
```

## Common Errors and Troubleshooting

| Symptom | Root Cause | Solution |
|---------|---------|---------|
| `invalid page_size 101: must be between 1 and 100` | page-size is out of range | Use an integer between 1 and 100 |
| Permission denied (99991672) | The bot app does not have `im:chat:read` TAT permission enabled | Enable the permission for the app in the Open Platform console |
| `Bot ability is not activated` (232025) | The app does not have bot capability enabled | Enable bot capability in the Open Platform console |
