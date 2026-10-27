> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+feed-group-query-item` (not available in the gateway: user identity only)

> Shortcut for `im.+feed-group-query-item` (not available in the gateway: user identity only). Look up specific feed cards inside one feed group (tag) by ID, enriched with a readable `chat_name`.

`im.+feed-group-query-item` (not available in the gateway: user identity only) is the only CLI surface for the `feed.groups.batch_query_item` read API — there is no raw `feed.groups batch_query_item` command. It resolves a human-readable `chat_name` for every feed card it returns: a v1 feed card's `feed_id` is always a chat ID (`oc_xxx`), so the shortcut issues a follow-up `POST /open-apis/im/v1/chats/batch_query` and injects `chat_name` into each entry of both `items[]` and `deleted_items[]`.

## Scopes

Because chat-name resolution always runs, this shortcut needs **two** user scopes unconditionally:

- `im:feed_group_v1:read` — to read the items
- `im:chat:read` — to resolve names

`chat_name` resolution always runs, so there is no single-scope, un-enriched path. For the other raw `feed.groups.*` methods, see topic `lark-im-feed-groups`.

## Flags

| Flag | Required | Description |
|---|---|---|
| `--feed-group-id` | Yes | Feed group ID (`ofg_xxx`); path parameter |
| `--feed-id` | Yes | Comma-separated chat IDs (`oc_xxx`); `feed_type` is fixed to `chat` |

## Output

The command sends `{"items":[{"feed_id":"oc_a","feed_type":"chat"},{"feed_id":"oc_b","feed_type":"chat"}]}`, then enriches the response (`items[]` and `deleted_items[]`) with `chat_name` exactly as `im.+feed-group-list-item` (not available in the gateway: user identity only) does. There is no pagination for this method.

A feed card whose chat cannot be resolved (soft-deleted or no permission) simply omits `chat_name` — the command still exits 0. p2p (direct) chats also omit `chat_name`: the server returns an empty `name` for them (the client UI shows the partner's display name instead); if a label is needed, fetch the chat via `chats/batch_query`, read `p2p_target_id`, and resolve it with a contact lookup.

## See also

- topic `lark-im-feed-groups` — raw `feed.groups.*` APIs, enums, and rule guidance
- topic `lark-im-feed-group-list` — list your feed groups
- topic `lark-im-feed-group-list-item` — list all feed cards in a group (paginated)
