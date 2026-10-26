> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+feed-group-list-item`（网关不提供：仅支持用户身份）

> Shortcut for `im.+feed-group-list-item`（网关不提供：仅支持用户身份）. List the feed cards inside one feed group (tag), enriched with a readable `chat_name`.

`im.+feed-group-list-item`（网关不提供：仅支持用户身份） is the only CLI surface for the `feed.groups.list_item` read API — there is no raw `feed.groups list_item` command. It resolves a human-readable `chat_name` for every feed card it returns: a v1 feed card's `feed_id` is always a chat ID (`oc_xxx`), so the shortcut issues a follow-up `POST /open-apis/im/v1/chats/batch_query` and injects `chat_name` into each entry of both `items[]` and `deleted_items[]`.

## Scopes

Because chat-name resolution always runs, this shortcut needs **two** user scopes unconditionally:

- `im:feed_group_v1:read` — to read the items
- `im:chat:read` — to resolve names

`chat_name` resolution always runs, so there is no single-scope, un-enriched path. For the other raw `feed.groups.*` methods, see lark-im-feed-groups.md（主题 `lark-im-feed-groups`）.

## Usage

```bash

# Auto-paginate through everything within a time window
```

## Flags

| Flag | Required | Description |
|---|---|---|
| `--feed-group-id` | Yes | Feed group ID (`ofg_xxx`); path parameter |
| `page_size` | No | Records per page, 1–50 (default 50) |
| `page_token` | No | Starting cursor, normally returned by a previous response |
| `page_all` | No | Auto-paginate and merge all pages |
| `page_limit` | No | Max pages when `page_all` is set, 1–1000 (default 20) |
| `--start-time` | No | Update-time window start (Unix milliseconds as a decimal string) |
| `--end-time` | No | Update-time window end (Unix milliseconds as a decimal string) |

When `page_token` and `page_all` are supplied together, automatic pagination starts at that cursor and continues until exhaustion or `page_limit`.

## Output

JSON keeps the raw envelope and adds `chat_name` to each resolvable item:

```json
{
  "items": [
    { "feed_id": "oc_abc", "feed_type": "chat", "update_time": "1767196800000", "chat_name": "Release Team" }
  ],
  "deleted_items": [
    { "feed_id": "oc_def", "feed_type": "chat", "update_time": "1767196800000", "chat_name": "Old Channel" }
  ],
  "page_token": "",
  "has_more": false
}
```

A feed card whose chat cannot be resolved (soft-deleted or no permission) simply omits `chat_name` — the command still exits 0. p2p (direct) chats also omit `chat_name`: the server returns an empty `name` for them (the client UI shows the partner's display name instead); if a label is needed, fetch the chat via `chats/batch_query`, read `p2p_target_id`, and resolve it with a contact lookup.

## See also

- lark-im-feed-groups.md（主题 `lark-im-feed-groups`） — raw `feed.groups.*` APIs, enums, and rule guidance
- lark-im-feed-group-list.md（主题 `lark-im-feed-group-list`） — list your feed groups
- lark-im-feed-group-query-item.md（主题 `lark-im-feed-group-query-item`） — look up specific feed cards by ID
