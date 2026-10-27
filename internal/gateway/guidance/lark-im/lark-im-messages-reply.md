> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+messages-reply`

This skill maps to the shortcut: `im.+messages-reply` (internally calls `POST /open-apis/im/v1/messages/:message_id/reply`).

## Safety Constraints

Replies sent by this tool are visible to other people. Before calling it, you **must** confirm with the user:

1. Which message to reply to
2. The reply content

**Do not** send a reply without explicit user approval.

When using app identity, the reply is sent in the app's name, so make sure the app has already been added to the target chat.

## Choose The Right Content Flag

### Default Selection Rule For Agents

- Prefer `markdown` for headings, lists, links, summaries, investigation notes, or Markdown-looking content.
- Use `text` for exact plain text: logs, code, indentation-sensitive text, or literal Markdown.
- Use `content` for exact `post` JSON, titles, multiple locales, cards, or unsupported structures.

| Need | Recommended flag | Why |
|------|------|------|
| Reply with headings, lists, links, summaries, or investigation notes | `markdown` | Best default for lightweight formatting; converted to Feishu `post` JSON |
| Reply with plain text exactly as written | `text` | Preserves literal text; no Markdown conversion |
| Precisely control the reply payload | `content` | You provide the exact JSON |

### `text` vs `markdown`

- Use `markdown` for lightweight formatted replies.
- Use `text` for exact plain text, especially logs, code, indentation, or literal Markdown characters.
- Use `content` when you need exact `post` JSON, a card, a title, multiple locales, or any structure that `markdown` cannot express reliably.

## What `markdown` Really Does

`markdown` accepts Markdown-like input and converts it to the Feishu `post` payload required by the reply API.

The shortcut:

1. Forces `msg_type=post`
2. Resolves remote Markdown images like `![x](https://...)`
3. Normalizes the Markdown for Feishu post rendering
4. Wraps the final content as:

```json
{"zh_cn":{"content":[[{"tag":"md","text":"..."}]]}}
```

This makes `markdown` the simplest path for lightweight formatted replies.

### Markdown Boundaries

- It does **not** promise full CommonMark / GitHub Flavored Markdown support.
- It always becomes a `post` payload with a single `zh_cn` locale.
- It does **not** let you set a `post` title.
- Headings are rewritten:
    - `# Title` becomes `#### Title`
    - `##` to `######` are normalized to `#####` when the content contains H1-H3
- Consecutive headings are separated with blank lines after heading normalization.
- Block spacing and line breaks may be normalized during conversion.
- Code blocks are preserved as code blocks.
- Excess blank lines are compressed.
- Already-uploaded `img_xxx` image keys are the most reliable Markdown image input.
- Local paths (e.g. `!x`) are **not** supported directly in `markdown` and will not be auto-uploaded.
- Remote URLs (`https://...`) will be auto-downloaded and uploaded at runtime; if the download or upload fails, the image is removed with a warning.

If you need a title, multiple locales, cards, unsupported rich structures, or byte-for-byte post JSON control, use `msg_type post content ...`.

### Image Constraint for `markdown`

When using `markdown` with images, prefer pre-uploading via `images.create` and referencing `!alt` for predictable results. Remote URLs may work but are not guaranteed.

**Steps:**

```text
# 1. Upload image to get image_key
im.images.create {"data": {"image_type": "message"}, "file": "./diagram.png"}

# 2. Use image_key in --markdown reply
im.+messages-reply {"message_id": "om_xxx", "markdown": "$## Result\\n\\n![diagram](img_v3_xxxx)\\n\\nSee above for details."}
```

## Preserving Formatting

If the reply contains multiple lines, code blocks, indentation, tabs, or a lot of escaping, prefer `$'...'` for either `markdown` or `text`.

### When formatting must be preserved

Use `text` plus `$'...'`:

```text
im.+messages-reply {"message_id": "om_xxx", "text": "$Received\\nI will check this today.\\nOwner: alice"}
```

```text
im.+messages-reply {"message_id": "om_xxx", "text": "$```sql\\nselect * from jobs;\\n```"}
```

This keeps the reply as plain text instead of converting it to a `post`.

## Commands

```text
# Reply with a formatted update
im.+messages-reply {"message_id": "om_xxx", "markdown": "$## Reply\\n\\n- item 1\\n- item 2"}

# Reply with a plain one-line message
im.+messages-reply {"message_id": "om_xxx", "text": "Received"}

# Read Markdown from a local file
im.+messages-reply {"message_id": "om_xxx", "markdown": {"file_id": "<file_id>"}}

# Equivalent manual JSON
im.+messages-reply {"message_id": "om_xxx", "content": "{\"text\":\"Received\"}"}

# Reply as a bot
im.+messages-reply {"message_id": "om_xxx", "text": "bot reply"}

# Reply with preserved multi-line text
im.+messages-reply {"message_id": "om_xxx", "text": "$Line 1\\nLine 2\\n  indented line"}

# Reply inside the thread (message appears in the target thread)
im.+messages-reply {"message_id": "om_xxx", "text": "Let's discuss this", "reply_in_thread": true}

# Reply with Markdown containing an image (must pre-upload via images.create)
im.images.create {"data": {"image_type": "message"}, "file": "./screenshot.png"}
# Use the returned image_key
im.+messages-reply {"message_id": "om_xxx", "markdown": "$## Screenshot\\n\\n![screenshot](img_v3_xxxx)\\n\\nConfirmed."}

# If you need exact post structure, send JSON directly
im.+messages-reply {"message_id": "om_xxx", "msg_type": "post", "content": "{\"zh_cn\":{\"title\":\"Reply\",\"content\":[[{\"tag\":\"text\",\"text\":\"Detailed content\"}]]}}"}

# Reply with a local image (uploaded automatically before sending)
im.+messages-reply {"message_id": "om_xxx", "image": {"file_id": "<file_id>"}}

# Reply with a local file (uploaded automatically before sending)
im.+messages-reply {"message_id": "om_xxx", "file": {"file_id": "<file_id>"}}

# Reply with a local video (--video-cover is required as the video cover)
im.+messages-reply {"message_id": "om_xxx", "video": {"file_id": "<file_id>"}, "video_cover": "./cover.png"}

# Reply with a voice message
im.+messages-reply {"message_id": "om_xxx", "audio": {"file_id": "<file_id>"}}

# With an idempotency key
im.+messages-reply {"message_id": "om_xxx", "text": "Received", "idempotency_key": "my-unique-id"}

# Preview the request without executing it
im.+messages-reply {"message_id": "om_xxx", "markdown": "$## Test\\n\\nhello"}

# Once the workflow has produced the card JSON, reply with it:
im.+messages-reply {"message_id": "om_xxx", "msg_type": "interactive", "content": "<card_json_from_workflow>"}
```

## Media Input Rules

- Media flags accept an existing key (`img_xxx` / `file_xxx`), an `http://` or `https://` URL, or a local file path.
- Local paths must be relative to the current working directory and stay within it after resolving `..` and symlinks.
- Absolute paths such as `/tmp/photo.png` are rejected. Run the command from the file's directory and pass `./photo.png`, or copy the file into the current directory first.
- `audio` sends a voice message and accepts only Opus audio (`.opus` or Ogg Opus `.ogg`) for local paths and URLs. For `mp3`, `wav`, or other non-Opus audio, convert to `.opus` before using `audio`, or use `file` to send the original audio as an attachment.

## Parameters

| Parameter | Required | Description                                                                                                                                                                                   |
|------|------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `message_id=<id>` | Yes | ID of the message being replied to (`om_xxx`)                                                                                                                                                 |
| `msg_type=<type>` | No | Message type (default `text`). If you use `text` / `markdown` / media flags, the effective type is inferred automatically. Explicitly setting a conflicting `msg_type` fails validation |
| `content=<json>` | One content option | Exact reply content as JSON. The JSON must match the effective `msg_type`                                                                                                                   |
| `text=<string>` | One content option | Plain text reply. Use when exact text and formatting preservation matter                                                                                                                      |
| `markdown=<string>` | One content option | Best default for lightweight formatted replies such as headings, lists, links, summaries, and investigation notes. Internally converted to `post` JSON with Feishu-specific normalization |
| `attachment=<key>` | One content option | Repeatable bare file/folder key (`file_xxx`); merges into the post message's attachment zone. Requires a post message (`markdown` or `msg_type=post`). Name/size/mime/is_folder are filled by the server from file service metadata, not taken from the client. Use this instead of `file` when the file should render inside a rich-text message's attachment area |
| `reply_in_thread` | No | Reply inside the thread. The reply appears in the target message's thread instead of the main chat stream                                                                                     |
| `idempotency_key=<key>` | No | Idempotency key, max 50 characters; the same key sends only one reply within 1 hour                                                                                                          |

> **Mutual exclusivity rule:** `text`, `markdown`, `content`, and `image`/`file`/`video`/`audio` cannot be used together. Media flags are also mutually exclusive with each other.
>
> **Video cover rule:** `video` **must** be accompanied by `video_cover`. Omitting `video_cover` when using `video` will fail validation. `video_cover` cannot be used without `video`.

## Common Mistakes

- Choosing `text` for headings, lists, links, summaries, or investigation notes. Use `markdown`.
- Choosing `markdown` when you actually need exact plain text. If exact line breaks, spacing, logs, code, or literal Markdown characters matter, use `text`, usually with `$'...'`.
- Assuming `markdown` supports every Markdown feature. It is converted into a Feishu `post` payload and normalized first.
- Putting local image paths inside Markdown like `!x`. `markdown` does not auto-upload those paths.
- **Using local file paths inside Markdown image syntax** (e.g. `!x`) with `markdown`. Local paths are not auto-uploaded and will not render as an image. Pre-upload via `images.create` to get an `image_key` instead.
- Using `content` without making the JSON match the effective `msg_type`.
- Explicitly setting `msg_type` to something that conflicts with `text`, `markdown`, or media flags.
- Mixing `text`, `markdown`, or `content` with media flags in one command.
- Using `attachment` with `text` or a media flag. The attachment zone only exists on `post` messages — pair `attachment` with `markdown` or `msg_type=post`.

## Return Value

```json
{
  "message_id": "om_xxx",
  "chat_id": "oc_xxx",
  "create_time": "1234567890"
}
```

## Usage Scenarios

### Scenario 1: Reply in the main chat stream

```text
im.+messages-reply {"message_id": "om_xxx", "text": "OK, I will handle it"}
```

The reply appears in the main chat stream and references the target message.

### Scenario 2: Reply inside a thread

```text
im.+messages-reply {"message_id": "om_xxx", "text": "Let me take a look at this", "reply_in_thread": true}
```

The reply appears in the target message's thread and does not show up in the main chat stream.

## @Mention Format

The `<at>` syntax differs by message type. The shortcut only normalizes mentions for `text` and `post`; `interactive` card content is passed through verbatim, so cards must use the card-native syntax below.

### `text`

- `<at user_id="ou_xxx">name</at>` — the inner text is the mentioned user's display name and is optional (`<at user_id="ou_xxx"></at>` also works)
- @all: `<at user_id="all"></at>`

### `post`

- Inside a `text` or `md` element, the same inline form as `text` works: `<at user_id="ou_xxx">name</at>`
- Or use a dedicated `at` element node: `{"tag":"at","user_id":"ou_xxx"}` (use `"all"` to mention everyone)

### `interactive` (card)

Card content is **not** normalized — use the card-native `<at>` syntax inside a `lark_md` / `markdown` element:

- single user by open_id: `<at id=ou_xxx></at>`
- multiple users: `<at ids=ou_xxx1,ou_xxx2></at>`
- by email: `<at email=user@example.com></at>`

## Notes

- `message_id` must be a valid message ID in `om_xxx` format
- `content` must be valid JSON
- When using `content`, you are responsible for making the JSON structure match the effective `msg_type`
- `reply_in_thread` adds `reply_in_thread=true` to the API request
- `reply_in_thread` is mainly meaningful in chats that support thread replies
- If the provided media value starts with `img_` or `file_`, it is treated as an existing key and used directly
- `markdown` always sends `msg_type=post`
- If you explicitly set `msg_type` and it conflicts with the chosen content flag, validation fails
- When using `video`, `video_cover` is required as the video cover
- dry-run (not available in the gateway; read and verify first) uses placeholder image keys for remote Markdown images and placeholder media keys for local uploads
- Failures return error codes and messages
- app identity uses a tenant access token (TAT), and requires the `im:message:send_as_bot` scope
- When using `markdown` with images, pre-uploading via `images.create` to obtain an `image_key` is recommended for reliability; remote URLs may be auto-resolved at runtime, but if download/upload fails the image is removed with a warning; local paths are not supported
- **Interactive cards are gated:** you MUST read and follow the topic `lark-im-card-create` workflow to produce the card JSON *before* replying. Do not hand-write or copy a card payload — the JSON given to `msg_type interactive content` must be the workflow's output. This applies every time, with no exception
