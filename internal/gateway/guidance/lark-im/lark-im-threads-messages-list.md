> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+threads-messages-list`

Fetch the reply message list inside a thread. When `im.+chat-messages-list` returns messages that include a `thread_id` field, use this command to inspect all replies in that thread.

By default each reply also carries a `reactions` block (counts + details from `im.reactions.batch_query`) when the server has reactions for it, and `update_time` for messages that were actually edited. Pass `no_reactions` to skip the extra round-trip. Pass `download_resources` to additionally download message resources (image/file/audio/video/media + post-embedded, excluding stickers) into `./lark-im-resources/` and attach a `resources` block — off by default, no extra requests when omitted. See message enrichment (topic `lark-im-message-enrichment`) for the full contract.

This skill maps to the shortcut: `im.+threads-messages-list` (internally calls `GET /open-apis/im/v1/messages` with `container_id_type=thread` to fetch thread messages).

## Commands

```text
# Get thread replies (ascending by time by default, table output)
im.+threads-messages-list {"thread": "omt_xxx"}

# Reverse chronological order (latest first)
im.+threads-messages-list {"thread": "omt_xxx", "order": "desc"}

# Control page size
im.+threads-messages-list {"thread": "omt_xxx", "page_size": "20"}

# Pagination
im.+threads-messages-list {"thread": "omt_xxx", "page_token": "<PAGE_TOKEN>"}

# Fetch multiple pages automatically, up to 10 pages by default
im.+threads-messages-list {"thread": "omt_xxx", "page_all": true}

# Output format options
im.+threads-messages-list {"thread": "omt_xxx"}
im.+threads-messages-list {"thread": "omt_xxx"}
im.+threads-messages-list {"thread": "omt_xxx"}

# Read thread context as compact Markdown
im.+threads-messages-list {"thread": "omt_xxx", "concise": true}

# View as a bot
im.+threads-messages-list {"thread": "omt_xxx"}

# Preview the request without executing it
im.+threads-messages-list {"thread": "omt_xxx"}
```

## Parameters

| Parameter | Required | Description |
|------|------|------|
| `thread=<id>` | Yes | Thread ID (`om_xxx` or `omt_xxx` format) |
| `no_reactions` | No | Skip auto-fetching the `reactions` block |
| `download_resources` | No | Download message resources (image/file/audio/video/media + post-embedded, excluding stickers) into `./lark-im-resources/` and attach a `resources` block. Off by default |
| `order=<order>` | No | Sort order: `asc` (default) / `desc` |
| `page_size=<n>` | No | Number of items per page (default 50, range 1-50) |
| `page_token=<token>` | No | Starting cursor, normally returned by a previous response |
| `page_all` | No | Automatically fetch and merge subsequent pages; capped by `page_limit` |
| `page_limit=<n>` | No | Maximum pages fetched by `page_all` (default 10, range 1-1000) |
| `concise` | No | Render compact Markdown for thread context |

## Core Constraints

### 1. Source of `thread_id`

`thread_id` (`omt_xxx` or `om_xxx`) comes from the `thread_id` field in results returned by `im.+chat-messages-list` or `im.+messages-search`. Do not guess a thread ID. Fetch messages first and use the returned value.

### 2. No time filtering support

Thread messages do not support `start_time` / `end_time` filtering because of Feishu API limitations. Use pagination and sort order to control the scope.

### 3. Pagination (`has_more` / `page_token`)

Default is one page. With `page_all`, `page_token` sets the starting cursor; if `meta.pagination.complete=false`, resume from `meta.pagination.next_token` or raise `page_limit`.

### 4. Recommended expansion strategy

| Scenario | Recommended Parameters |
|------|---------|
| Quickly inspect recent replies | `order desc page_size 10` |
| Read the full thread in chronological order | `order asc page_size 50`, then paginate as needed |
| Just confirm whether replies exist | `order desc page_size 1` |

## Usage Scenarios

### Scenario 1: Expand a thread discovered in group messages

```text
# Step 1: Fetch group messages and find one that contains thread_id
im.+chat-messages-list {"chat_id": "oc_xxx"}

# Step 2: Extract thread_id from the JSON output and fetch thread replies
im.+threads-messages-list {"thread": "omt_xxx"}
```

### Scenario 2: Paginate through a long thread

```text
# First page
im.+threads-messages-list {"thread": "omt_xxx"}

# If has_more=true is returned, continue with page_token
im.+threads-messages-list {"thread": "omt_xxx", "page_token": "<PAGE_TOKEN>"}
```

## Resource Rendering

Thread replies are rendered into human-readable text. Image messages appear as placeholders such as `!Image`; `folder` replies are expanded one level (children rendered inside a `<folder ...>` tag); by default resource binaries are **not** downloaded.

Pass `download_resources` to download every eligible resource (image/file/audio/video/media + post-embedded, excluding stickers) into `./lark-im-resources/` in one pass and attach a `resources` block to each reply (see message enrichment (topic `lark-im-message-enrichment`)). Otherwise download individual resources manually through `im.+messages-resources-download` (see topic `lark-im-messages-resources-download`).

## Common Errors and Troubleshooting

| Symptom | Root Cause | Solution |
|---------|---------|---------|
| "Invalid thread ID format" | `thread_id` does not start with `om_` or `omt_` | Use a valid `om_xxx` or `omt_xxx` value |
| Empty thread result | Wrong thread_id or no replies in the thread | Confirm the thread_id came from `im.+chat-messages-list` output |
| Permission denied | The user is not authorized or is not a conversation member | Make sure OAuth authorization is complete and the identity is a chat member |

## References

- `im` domain指南 - all message-related commands
- topic `lark-im-chat-messages-list` - fetch conversation messages (source of `thread_id`)
