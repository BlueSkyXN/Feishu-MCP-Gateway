> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+chat-search`

This skill maps to the shortcut: `im.+chat-search` (internally calls `POST /open-apis/im/v2/chats/search`).

## Commands

```text
# Search chats by keyword
im.+chat-search {"query": "project"}

# Restrict by search types
im.+chat-search {"query": "project", "search_types": "private,public_joined"}

# Filter by chat mode (group = regular group, topic = topic/thread group)
im.+chat-search {"query": "project", "chat_modes": "topic"}

# Filter by member open_ids (with keyword)
im.+chat-search {"query": "project", "member_ids": "ou_xxx,ou_yyy"}

# Search by member open_ids only
im.+chat-search {"member_ids": "ou_xxx,ou_yyy"}

# Only show chats you created or manage
im.+chat-search {"query": "project", "is_manager": true}

# Set page size
im.+chat-search {"query": "project", "page_size": 10}

# Pagination
im.+chat-search {"query": "project", "page_token": "xxx"}

# Fetch multiple pages automatically, up to 10 pages by default
im.+chat-search {"query": "project", "page_all": true}

# JSON output
im.+chat-search {"query": "project"}

# Preview the request without executing it
im.+chat-search {"query": "project"}
```

## Parameters

| Parameter | Required | Limits | Description |
|------|------|------|------|
| `query=<keyword>` | No (at least one of `query` / `member_ids` required) | Max 64 characters | Search keyword. Supports matching localized chat names, member names, multilingual search, pinyin, and prefix fuzzy search. If the query contains `-`, it is automatically wrapped in quotes |
| `search_types=<types>` | No | Comma-separated: `private`, `external`, `public_joined`, `public_not_joined` | Restrict the visible chat types returned by search |
| `chat_modes=<modes>` | No | Comma-separated: `group`, `topic` | Filter by chat mode (server-side): `group` = regular group, `topic` = topic/thread group |
| `member_ids=<ids>` | No (at least one of `query` / `member_ids` required) | Up to 50, format `ou_xxx` | Filter by member open_ids; can be used alone or combined with `query` |
| `is_manager` | No | - | Only show chats you created or manage |
| `disable_search_by_user` | No | - | Disable member-name-based matching and search by group name only |
| `sort=<field>` | No | `create_time`, `update_time`, `member_count` | Sort field (always descending) |
| `page_size=<n>` | No | 1-100, default 20 | Number of results per page |
| `page_token=<token>` | No | - | Starting cursor, normally returned by a previous response |
| `page_all` | No | - | Automatically fetch and merge subsequent pages; capped by `page_limit` |
| `page_limit=<n>` | No | 1-1000, default 10 | Maximum pages fetched by `page_all` |
|  | No | - | Output as JSON |
| dry-run（网关不提供，改为先读取核对） | No | - | Preview the request without executing it |

With `page_all`, `page_token` sets the starting cursor. If `meta.pagination.complete=false`, resume from `meta.pagination.next_token` or raise `page_limit`.

> **CAUTION:** `sort` is **always descending** — the search API only ranks the chosen field high-to-low (e.g. `member_count` = most members first). There is no ascending option. If the user asks for "fewest first / ascending / 从少到多", tell them the search API does not support ascending order; any low-to-high view requires re-sorting the fetched page client-side and is not an upstream sort. Do **not** invent values like `member_count_asc` or pass `asc` (they are rejected).

## Output Fields

| Field | Description |
|------|------|
| `chat_id` | Chat ID (`oc_xxx` format) |
| `name` | Chat name |
| `description` | Chat description |
| `owner_id` | Owner ID |
| `external` | Whether the chat is external |
| `chat_status` | Chat status (`normal` / `dissolved` / `dissolved_save`) |

## Filtering muted chats

When the flag is set, the JSON envelope gains a `filter` sub-object (absent otherwise, so existing consumers are unaffected); `fetched_count == returned_count + filtered_count` always holds:

```json
{
  "chats": [...],
  "filter": {
    "applied": "exclude_muted",
    "fetched_count": 20,
    "returned_count": 19,
    "filtered_count": 1,
    "hint": "Filtered out 1 muted chat(s) on this page (19 remaining, including 2 non-member public group(s)); use --page-token to fetch more."
  }
}
```

Note: only confirmed-muted chats count toward `filtered_count`; non-member public groups are retained and surfaced in `hint`. For strict member-only results, combine with `search_types=private,public_joined,external`.

## Usage Scenarios

### Scenario 1: Search chats that contain a keyword

```text
im.+chat-search {"query": "design review"}
```

### Scenario 2: Search a chat and list recent messages

```bash
CHAT_ID=$(lark-cli im +chat-search --query "project" --format json | jq -r '.data.chats[0].chat_id')
im.+chat-messages-list {"chat_id": "$CHAT_ID"}
```

### Scenario 3: Search a chat and send a message

```bash
CHAT_ID=$(lark-cli im +chat-search --query "daily report" --format json | jq -r '.data.chats[0].chat_id')
im.+messages-send {"chat_id": "$CHAT_ID", "text": "Today's progress update"}
```

## Common Errors and Troubleshooting

| Symptom | Root Cause | Solution |
|---------|---------|---------|
| `query and member_ids cannot both be empty` | Both were omitted | Provide at least `query` or `member_ids` |
| Empty results | No visible chats matched the keyword or filters | Relax the keyword or filters and try again |
| `invalid page_size 101: must be between 1 and 100` | page-size is out of range | Use an integer between 1 and 100 |
| Permission denied (99991672) | The bot app does not have `im:chat:read` TAT permission enabled | Enable the permission for the app in the Open Platform console |
| `Bot ability is not activated` (232025) | The app does not have bot capability enabled | Enable bot capability in the Open Platform console |

## AI Usage Guidance

When the user asks to search chats, follow these rules:

1. **At least one filter required:** `query` and `member_ids` cannot both be empty. Either alone or combined together are valid.
3. **Control result volume:** the result set may be large. Use `page_size` deliberately.
4. **Suggest follow-up actions:** after finding a chat, common next steps include listing recent messages (`im.+chat-messages-list`) or sending a message (`im.+messages-send`).
5. **NEVER fall back to chats list:** If `im.+chat-search` returns empty results, do NOT attempt to use `im.+chat-list` or `GET /open-apis/im/v1/chats` as a fallback. The list API is not a search API — it returns all chats without keyword filtering and will not help locate the target chat. Instead, ask the user to refine the keyword or check whether the chat is visible to the current identity.

## References

- `im` 领域指南 - all IM commands
