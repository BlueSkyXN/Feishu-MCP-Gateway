> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+chat-messages-list`

Fetch the message list for a conversation. Supports both group chats and direct messages.

By default the response carries a `reactions` block (counts + details from `im.reactions.batch_query`) on every message that has reactions, and `update_time` on messages that were actually edited. Thread replies expanded via auto-`thread_replies` participate in the same batched enrichment. Pass `no_reactions` to skip the extra round-trip. Pass `download_resources` to additionally download message resources (image/file/audio/video/media + post-embedded, excluding stickers) into `./lark-im-resources/` and attach a `resources` block — off by default. See message enrichment（主题 `lark-im-message-enrichment`） for the full contract.

This skill maps to the shortcut: `im.+chat-messages-list` (internally calls `GET /open-apis/im/v1/messages`, and automatically resolves the p2p chat_id when needed).

## Commands

```text
# Get group chat messages (json output by default)
im.+chat-messages-list {"chat_id": "oc_xxx"}

# Get direct messages with a user (pass open_id and resolve p2p chat_id automatically)
im.+chat-messages-list {"user_id": "ou_xxx"}

# Read message context as compact Markdown
im.+chat-messages-list {"chat_id": "oc_xxx", "concise": true}

# Specify a time range (ISO 8601)
im.+chat-messages-list {"chat_id": "oc_xxx", "start": "2026-03-10T00:00:00+08:00", "end": "2026-03-11T00:00:00+08:00"}

# Specify a time range (date only)
im.+chat-messages-list {"chat_id": "oc_xxx", "start": "2026-03-10", "end": "2026-03-11"}

# Control sort order and page size (max 50)
im.+chat-messages-list {"chat_id": "oc_xxx", "order": "asc", "page_size": "20"}

# Pagination
im.+chat-messages-list {"chat_id": "oc_xxx", "page_token": "xxx"}

# Fetch multiple pages automatically, up to 10 pages by default
im.+chat-messages-list {"chat_id": "oc_xxx", "page_all": true}

# JSON output
im.+chat-messages-list {"chat_id": "oc_xxx"}
```

## Parameters

| Parameter | Required | Description |
|------|------|------|
| `chat_id=<id>` | One of two | Specify the conversation by its chat_id directly (e.g., group chat `oc_xxx`) |
| `start=<time>` | No | Start time (ISO 8601 or date only) |
| `end=<time>` | No | End time (ISO 8601 or date only) |
| `order=<order>` | No | Sort order: `asc` / `desc` (default `desc`) |
| `page_size=<n>` | No | Page size (default 50, max 50) |
| `page_token=<token>` | No | Starting cursor, normally returned by a previous response |
| `page_all` | No | Automatically fetch and merge subsequent pages; capped by `page_limit` |
| `page_limit=<n>` | No | Maximum pages fetched by `page_all` (default 10, range 1-1000) |
| `no_reactions` | No | Skip auto-fetching the `reactions` block |
| `download_resources` | No | Download message resources (image/file/audio/video/media + post-embedded, excluding stickers) into `./lark-im-resources/` and attach a `resources` block. Off by default; no extra requests when omitted |
| `concise` | No | Render compact Markdown for message context |

> Rule: `chat_id` and `user_id` are mutually exclusive. You must provide exactly one of them.

> **CAUTION:** `order` is the only sort axis — messages are always ordered by creation time, `asc` or `desc`. There is no field axis: the command cannot sort by sender or any other field, so do **not** attempt `sort=sender` or similar (it is rejected). If the user asks to group or sort by sender, fetch with `order` and aggregate client-side, and tell them this is local post-processing, not a CLI/API sort capability.

## Resource Rendering

Messages are rendered into human-readable text for inspection. Image messages are shown as placeholders such as `!Image`; files, audio, and videos are rendered with resource keys in the content (e.g. `<audio key="file_xxx" duration="Xs"/>`). `folder` messages are expanded one level (children rendered inside the tag, see the row below). By default resource binaries are **not** downloaded.

Two ways to get the binaries:
- **In one pass:** add `download_resources` to this command — every eligible resource (image/file/audio/video/media + post-embedded, excluding stickers) is downloaded into `./lark-im-resources/` and a `resources` block (`{message_id, key, type, local_path, size_bytes}`) is attached to each message. See message enrichment（主题 `lark-im-message-enrichment`）.
- **One at a time:** use lark-im-messages-resources-download（主题 `lark-im-messages-resources-download`）.

| Resource Type | Marker in Content | Behavior |
|---------|-------------|------|
| Image | `!Image` | `download_resources`, or manually `im.+messages-resources-download`（type="image"） |
| File | `<file key="file_xxx" .../>` | `download_resources`, or manually `im.+messages-resources-download`（type="file"） |
| Folder (message) | `<folder key="file_xxx" name="assets" child_count="N"><file key="..." .../>…</folder>` (first-level children rendered inside; `has_more="true"` past the 10-item cap) | Folder itself is not a single-file resource; children are real files — download one with explicit `im.+messages-resources-download`（message_id="<id>"，file_key="<child_key>"，type="file"） (`download_resources` auto-collection does not include folder children) |
| Audio | `<audio key="file_xxx" duration="Xs"/>` | `download_resources`, or manually `im.+messages-resources-download`（type="file"） |
| Video | `<video key="file_xxx" .../>` | `download_resources`, or manually `im.+messages-resources-download`（type="file"） |
| Sticker | `[Sticker]` | Not downloadable (Feishu does not support fetching sticker resources) |

## Thread Expansion (`thread_id`)

In JSON output, a message may contain a `thread_id` (`omt_xxx`) field, which means the message has replies in a thread. Use `im.+threads-messages-list`（主题 `lark-im-threads-messages-list`） to inspect replies in that thread:

```text
im.+threads-messages-list {"thread": "omt_xxx"}
```

| Scenario | Recommendation |
|------|------|
| You need context | Call `im.+threads-messages-list`（order="desc"，page_size="10"） for the discovered thread_id to inspect recent replies |
| The user asks for the "full discussion" | Use `im.+threads-messages-list`（order="asc"，page_size="50"）, then paginate if needed |
| You only need an overview | Skip thread expansion |

## Output Fields

| Field | Description |
|------|------|
| `messages` | Message array |
| `total` | Number of messages in the current page |
| `has_more` | Whether additional pages are available |
| `page_token` | Pagination token for the next page |

Each message contains:

| Field | Description |
|------|------|
| `message_id` | Message ID |
| `msg_type` | Message type: `text`, `image`, `file`, `interactive`, `post`, `audio`, `video`, `system`, etc. |
| `create_time` | Creation time |
| `sender` | Sender information (includes `name` for user senders) |
| `content` | Message content |
| `deleted` | Whether the message has been recalled (always present, `true` = recalled) |
| `updated` | Whether the message has been edited after sending |
| `mentions` | Array of @mentions in the message; each item contains `{id, key, name}`. Present only when the message contains @mentions |
| `thread_id` | Thread ID (`omt_xxx`) if the message has replies in a thread. Present only when replies exist |

## Pagination (`has_more` / `page_token`)

By default, `im.+chat-messages-list` fetches one page. It returns `has_more` and `page_token` when more data is available. Use `page_token` to continue:

```text
im.+chat-messages-list {"chat_id": "oc_xxx", "page_token": "<PAGE_TOKEN>"}
```

With `page_all`, `page_token` sets the starting cursor. If `meta.pagination.complete=false`, resume from `meta.pagination.next_token` or raise `page_limit`.

You can also fall back to the generic API:

```text
# GET /open-apis/im/v1/messages（CLI 通用 API 调用，网关不提供）
```

## Common Errors and Troubleshooting

| Symptom | Root Cause | Solution |
|---------|---------|---------|
| `specify chat_id <chat_id> or user_id <open_id>` | Neither `chat_id` nor `user_id` was provided | You must provide exactly one |
| `chat_id and user_id cannot be specified together` | Both parameters were provided | Use only one |
| `P2P chat not found for this user` | `user_id` was used but no p2p chat exists for the current identity and that user | Confirm the target direct-message relationship exists for the current identity |
| `start: invalid time format` | Invalid time format | Use ISO 8601 or date-only format such as `2026-03-10` |
| Permission denied | Message read permissions are missing | Ensure the app has `im:message:readonly` and `im:chat:read` enabled |

## AI Usage Guidance

1. **Resolving chat_id from a chat name:** When the user refers to a chat by name and you don't have the `chat_id`, use `im.+chat-search`（主题 `lark-im-chat-search`） first:
```text
   # Find chat_id by name, then list messages
   im.+chat-search {"query": "<chat name keyword>"}
   im.+chat-messages-list {"chat_id": "<chat_id>"}
```
   **Do not use `im chats search` or `im.+chat-list` — always use the `im.+chat-search` shortcut.**
2. **Prefer `chat_id` when available:** if the chat_id is already known, use it directly to avoid extra API calls.
4. **For time ranges:** both ISO 8601 and date-only inputs are supported. Date-only is usually simpler.
5. **For full content:** table output truncates content. Use  when you need the complete message body.
6. **For sender info:** the command already resolves sender names, so you do not need a separate lookup.
7. **Application/bot identity + named group history:** If the user says "使用应用身份/以 bot 身份" and asks to list or read historical messages for a named group, use bot identity for both steps:

## References

- `im` 领域指南 - all IM commands
