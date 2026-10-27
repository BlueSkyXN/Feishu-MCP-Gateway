> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+messages-edit`

Edit an already-sent message's content. **Bot identity only** — the edit API does not accept user tokens. Only messages the bot sent can be edited.

This skill maps to the shortcut: `im.+messages-edit` (PUT on the message edit endpoint).

## Safety Constraints

Editing rewrites a message visible to other people. Before calling it, you **must** confirm with the user:

1. Which message to edit (its `message_id`)
2. The new content

**Do not** edit a message without explicit user approval.

## Choose The Right Content Flag

| Need | Recommended flag | Why |
|------|------|------|
| Edit to headings, lists, links, summaries, or Markdown-looking content | `markdown` | Best default for lightweight formatting; converted to Feishu `post` JSON |
| Edit to exact plain text | `text` | Preserves literal text; no Markdown conversion |
| Precisely control the new payload | `content` | You provide the exact JSON for `text` / `post` |
| Attach files/folders to the edited message's attachment zone | `set_attachments` | Repeatable, as bare `file_key` (`file_xxx`); **replaces** the post content's `files` array (flag values are the final list, discarding any `files` in `content`). Requires a post message (`markdown` or `msg_type=post`). Name/metadata are filled by the server, not the client |
| Clear the edited message's attachment zone | `clear_attachments` | Sets `files:[]` on the post content. Requires a post message; mutually exclusive with `set_attachments` |
| Keep the existing attachment zone while rewriting the body | *(no attachment flag)* | **Default.** Editing with only `markdown` / `text` / `content` leaves the current `files` array untouched — a body-only edit never drops attachments |

```text
# Read the replacement Markdown from a local file
im.+messages-edit {"message_id": "om_xxx", "markdown": {"file_id": "<file_id>"}}
```

## Editing the Attachment Zone

`post` messages can carry an attachment zone — a top-level `files` array that renders files/folders under the rich-text body.

**Default: no attachment flag preserves the attachment zone.** Editing with only `markdown` / `text` / `content` (i.e. passing neither `set_attachments` nor `clear_attachments`) rewrites the body and keeps the existing `files` array unchanged. This is the safe default — fixing a typo must not drop the files you attached. Only pass `set_attachments` to replace the zone, or `clear_attachments` to remove it.

To edit a message so it attaches (or re-attaches) files:

```text
im.+messages-edit {"message_id": "om_xxx", "markdown": "Updated content", "set_attachments": ["file_xxx", "file_yyy"]}
```

- `set_attachments` accepts a bare file/folder key (`file_xxx`), and may be repeated.
- **`set_attachments` is a replace, not an append:** the flag values become the final `files` array. Send/reply's `attachment` merges; edit's `set_attachments` replaces.
- **Mutually exclusive with `content` carrying files:** when `content` already contains a `files` array, `set_attachments` and `clear_attachments` are rejected — declare the attachment zone either via `content` or via the attachment flags, not both. Use `markdown` (which never emits a `files` array) or a `content` without `files` together with the attachment flags.
- The server fills name/size/mime/is_folder from file service metadata; the client does not (and cannot) override the display name.
- When `set_attachments` is present the effective `msg_type` is forced to `post`. Pair it with `markdown` (or `content` with post JSON plus `msg_type=post`); `text` cannot carry an attachment zone.
- The edited content replaces the whole message content, so include every file you want to keep in the new attachment zone.

To **clear** the attachment zone entirely, pass `clear_attachments` instead of `set_attachments`:

```text
im.+messages-edit {"message_id": "om_xxx", "markdown": "Updated content", "clear_attachments": true}
```

- `clear_attachments` sets the post content's `files` array to `[]`, telling the server to remove all file/folder attachments.
- It cannot be used together with `set_attachments`.
- Like `set_attachments`, it forces the effective `msg_type` to `post`, so pair it with `markdown` or `msg_type post content <post-json>`.

## Parameters

| Parameter | Required | Description |
|------|------|------|
| `message_id=<id>` | Yes | Message ID (`om_xxx`) to edit |
| `text=<string>` | One content option | Plain text content |
| `markdown=<string>` | One content option | Markdown text, converted to `post` JSON |
| `content=<json>` | One content option | Exact message content JSON; must match the effective `msg_type` |
| `set_attachments=<key>` | One content option | Repeatable bare file/folder key (`file_xxx`); **replaces** the post attachment zone — the flag values become the final `files` array, discarding any `files` written in `content`, and duplicate keys are sent once. Name/size/mime/is_folder are filled by the server |
| `clear_attachments` | One content option | Clear the post attachment zone by setting `files:[]` |
| `msg_type=<type>` | No | Message type (default `text`). When `markdown`/`set_attachments`/`clear_attachments` is used the effective type is inferred automatically |

## Return Value

```json
{
  "message_id": "om_xxx",
  "chat_id": "oc_xxx",
  "update_time": "1234567890"
}
```

## Common Mistakes

- Editing a message the calling identity did not send — the API rejects it.
- Using `set_attachments` with `text`. The attachment zone only exists on `post` messages; use `markdown` or `msg_type=post`.
- Supplying only the files you want to keep, then losing the text. Editing replaces the entire content; pass the full new content (text + attachments) in one call.
- Assuming a body-only edit clears the attachment zone. It does not — without `set_attachments` / `clear_attachments` the existing attachments are preserved.
