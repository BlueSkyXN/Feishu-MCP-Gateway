> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+messages-send`

This skill maps to the shortcut: `im.+messages-send` (internally calls `POST /open-apis/im/v1/messages`).

## Safety Constraints

Messages sent by this tool are visible to other people. Before calling it, you **must** confirm with the user:

1. The recipient (which person or which group)
2. The message content

**Do not** send messages without explicit user approval.

When using app identity, the message is sent in the app's name, so make sure the app has already been added to the target chat.

## Choose The Right Content Flag

### Default Selection Rule For Agents

- Prefer `markdown` for headings, lists, links, summaries, reports, or Markdown-looking content.
- Use `text` for exact plain text: logs, code, indentation-sensitive text, or literal Markdown.
- Use `content` for exact `post` JSON, titles, multiple locales, cards, or unsupported structures.

| Need | Recommended flag | Why |
|------|------|------|
| Send headings, lists, links, summaries, or reports | `markdown` | Best default for lightweight formatting; converted to Feishu `post` JSON |
| Send plain text exactly as written | `text` | Preserves literal text; no Markdown conversion |
| Precisely control the final payload | `content` | You provide the exact JSON for `text` / `post` / `interactive` / `share_*` / media payloads |
| Attach files/folders to a post message's attachment zone | `attachment` | Repeatable, as bare `file_key` (`file_xxx`); merges into the post content's `files` array. Requires a post message (`markdown` or `msg_type=post`). Name/metadata are filled by the server, not the client |

### `text` vs `markdown`

- Use `markdown` for lightweight formatted messages.
- Use `text` for exact plain text, especially logs, code, indentation, or Markdown characters that should **not** render.
- Use `content` when `markdown` is not enough, especially if you need exact `post` JSON, a title, multiple locales, cards, or unsupported rich structures.

## What `markdown` Really Does

`markdown` accepts Markdown-like input and converts it to the Feishu `post` payload required by the message API.

The shortcut does all of the following before sending:

1. Forces `msg_type=post`
2. Resolves remote Markdown images like `![x](https://...)` by downloading and uploading them first
3. Normalizes the Markdown for Feishu post rendering
4. Wraps the result as:

```json
{"zh_cn":{"content":[[{"tag":"md","text":"..."}]]}}
```

This makes `markdown` the simplest path for lightweight formatted messages.

### Markdown Boundaries

- It does **not** promise full CommonMark / GitHub Flavored Markdown support.
- It always becomes a `post` payload with a single `zh_cn` locale.
- It does **not** let you set a `post` title. If you need a title, use `msg_type post content ...`.
- Headings are rewritten:
    - `# Title` becomes `#### Title`
    - `##` to `######` are normalized to `#####` when the content contains H1-H3
- Consecutive headings are separated with blank lines after heading normalization.
- Block spacing and line breaks may be normalized during conversion.
- Code blocks are preserved as code blocks.
- Excess blank lines are compressed.
- Already-uploaded `img_xxx` image keys are the most reliable Markdown image input.
- Local paths in Markdown image syntax like `![x](./a.png)` are **not** supported and will not be auto-uploaded.
- Remote URLs (`https://...`) will be auto-downloaded and uploaded at runtime; if the download or upload fails, the image is removed with a warning.

If you need a title, multiple locales, cards, unsupported rich structures, or byte-for-byte post JSON control, use `content` and provide the final JSON yourself.

### Image Constraint for `markdown`

When using `markdown` with images, prefer pre-uploading via `images.create` and referencing `![alt](img_xxx)` for predictable results. Remote URLs may work but are not guaranteed.

**Steps:**

```text
# 1. Upload image to get image_key
im.images.create {"data": {"image_type": "message"}, "files": {"image": {"file_id": "<file_id>"}}}
# Returns: {"image_key":"img_v3_xxxx"}

# 2. Use image_key in --markdown
im.+messages-send {"chat_id": "oc_xxx", "markdown": "## Report\n\n![diagram](img_v3_xxxx)\n\nSee above for details."}
```

## Preserving Formatting

If the message has multiple lines, indentation, code blocks, tabs, or many quotes/backslashes, write line breaks as `\n` inside the JSON string for either `markdown` or `text`.

### When formatting must be preserved

Use `text`:

```text
im.+messages-send {"chat_id": "oc_xxx", "text": "Build failed\nBranch: feature/im-docs\nAction: please check logs"}
```

```text
im.+messages-send {"chat_id": "oc_xxx", "text": "```bash\nmake test\nmake lint\n```"}
```

Use this path when you want the receiver to see the text exactly as entered, not a converted Markdown post.

## Commands

```text
# Send a formatted update
im.+messages-send {"chat_id": "oc_xxx", "markdown": "## Update\n\n- item 1\n- item 2"}

# Send a plain one-line message
im.+messages-send {"chat_id": "oc_xxx", "text": "Hello"}

# Read Markdown from a local file
im.+messages-send {"chat_id": "oc_xxx", "markdown": {"file_id": "<file_id>"}}

# Equivalent manual JSON
im.+messages-send {"chat_id": "oc_xxx", "content": "{\"text\":\"Hello\"}"}

# Send to a direct message (pass open_id)
im.+messages-send {"user_id": "ou_xxx", "text": "Hello"}

# Send multi-line text while preserving formatting
im.+messages-send {"chat_id": "oc_xxx", "text": "Line 1\nLine 2\n  indented line"}

# Send Markdown with an image (must pre-upload via images.create)
im.images.create {"data": {"image_type": "message"}, "files": {"image": {"file_id": "<file_id>"}}}
# Use the returned image_key in the markdown content
im.+messages-send {"chat_id": "oc_xxx", "markdown": "## Status\n\n![screenshot](img_v3_xxxx)\n\nDone."}

# If you need exact post structure, send JSON directly
im.+messages-send {"chat_id": "oc_xxx", "msg_type": "post", "content": "{\"zh_cn\":{\"title\":\"Title\",\"content\":[[{\"tag\":\"text\",\"text\":\"Body\"}]]}}"}

# Send a local image (uploaded automatically before sending)
im.+messages-send {"chat_id": "oc_xxx", "image": {"file_id": "<file_id>"}}

# Or send directly with an existing image_key
im.+messages-send {"chat_id": "oc_xxx", "image": "img_xxx"}

# Send a local file (uploaded automatically before sending)
im.+messages-send {"chat_id": "oc_xxx", "file": {"file_id": "<file_id>"}}

# Send a video (--video-cover is required as the cover)
im.+messages-send {"chat_id": "oc_xxx", "video": {"file_id": "<file_id>"}, "video_cover": "./cover.png"}
im.+messages-send {"chat_id": "oc_xxx", "video": {"file_id": "<file_id>"}, "video_cover": "img_xxx"}

# Send a voice message
im.+messages-send {"chat_id": "oc_xxx", "audio": {"file_id": "<file_id>"}}

# Use an idempotency key (same key sends only once within 1 hour)
im.+messages-send {"chat_id": "oc_xxx", "text": "Hello", "idempotency_key": "my-unique-id"}

# Preview the request without executing it
# 网关不提供 dry-run 预览，此示例省略

# ===== Interactive Card =====
# 🚫 STOP — before constructing ANY interactive card JSON, you MUST read
#    card/lark-im-card-create.md and follow its workflow. Do NOT
#    hand-write or copy a card payload from the examples below. The JSON passed
#    to --content must be the OUTPUT of that workflow. This is non-negotiable.

# Once the workflow has produced the card JSON, send it:
im.+messages-send {"chat_id": "oc_xxx", "msg_type": "interactive", "content": "<card_json_from_workflow>"}
```

## Media Input Rules

- Media flags accept an existing key (`img_xxx` / `file_xxx`), an `http://` or `https://` URL, or a local file path.
- Local paths must be relative to the current working directory and stay within it after resolving `..` and symlinks.
- Absolute paths such as `/tmp/photo.png` are rejected. Run the command from the file's directory and pass `./photo.png`, or copy the file into the current directory first.
- `audio` sends a voice message and accepts only Opus audio (`.opus` or Ogg Opus `.ogg`) for local paths and URLs. For `mp3`, `wav`, or other non-Opus audio, convert to `.opus` before using `audio`, or use `file` to send the original audio as an attachment.

## Parameters

| Parameter | Required | Description                                                                                                                                                                                   |
|------|------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `chat_id=<id>` | One of two | Group chat ID (`oc_xxx`)                                                                                                                                                                      |
| `user_id=<id>` | One of two | User open_id (`ou_xxx`) for direct messages                                                                                                                                                   |
| `text=<string>` | One content option | Plain text message. Use when exact text and formatting preservation matter. Automatically wrapped as `{"text":"..."}`                                                                         |
| `markdown=<string>` | One content option | Best default for lightweight formatted messages such as headings, lists, links, summaries, and reports. Internally converted to `post` JSON with Feishu-specific normalization               |
| `content=<json>` | One content option | Exact message content JSON string; use this when you need full control over `msg_type` and payload. The JSON must match the effective `msg_type`                                            |
| `attachment=<key>` | One content option | Repeatable bare file/folder key (`file_xxx`); merges into the post message's attachment zone. Requires a post message (`markdown` or `msg_type=post`). Name/size/mime/is_folder are filled by the server from file service metadata, not taken from the client. Use this instead of `file` when the file should render inside a rich-text message's attachment area |
| `msg_type=<type>` | No | Message type (default `text`). If you use `text` / `markdown` / media flags, the effective type is inferred automatically. Explicitly setting a conflicting `msg_type` fails validation |
| `idempotency_key=<key>` | No | Idempotency key, max 50 characters; the same key sends only one message within 1 hour                                                                                                        |

> **Mutual exclusivity rule:** `text`, `markdown`, `content`, and `image`/`file`/`video`/`audio` cannot be used together. Media flags are also mutually exclusive with each other. `attachment` cannot be combined with a `content` that already contains a `files` array (the attachment zone is declared either via `content` or via `attachment`, not both).
>
> **Video cover rule:** `video` **must** be accompanied by `video_cover`. Omitting `video_cover` when using `video` will fail validation. `video_cover` cannot be used without `video`.

## Common Mistakes

- Choosing `text` for headings, lists, links, summaries, or reports. Use `markdown`.
- Choosing `markdown` when you actually need exact plain text. If exact line breaks, spacing, logs, code, or literal Markdown characters matter, use `text`.
- Assuming `markdown` supports every Markdown feature. It is converted into a Feishu `post` payload and normalized first.
- Putting local image paths inside Markdown like `![x](./a.png)`. `markdown` does not auto-upload those paths.
- **Using local file paths inside Markdown image syntax** (e.g. `![x](./a.png)`) with `markdown`. Local paths are not auto-uploaded and will not render as an image. Pre-upload via `images.create` to get an `image_key` instead.
- Using `content` without making the JSON match the effective `msg_type`.
- Explicitly setting `msg_type` to something that conflicts with `text`, `markdown`, or media flags.
- Mixing `text`, `markdown`, or `content` with media flags in one command.
- Using `attachment` with `text` or a media flag. The attachment zone only exists on `post` messages — pair `attachment` with `markdown` or `msg_type=post`.
- Using `file` when the file should sit inside a rich-text message's attachment area. `file` sends a standalone `file`-type message; use `attachment` (with `markdown` or post `content`) to attach files/folders inside a post message.

## `content` Format Reference

| `msg_type` | Example `content` |
|----------|-------------|
| `text` | `{"text":"Hello <at user_id=\"ou_xxx\">name</at>"}` |
| `post` | `{"zh_cn":{"title":"Title","content":[[{"tag":"text","text":"Body"}]]},"files":[{"key":"file_xxx"}]}` — the top-level `files` array is the attachment zone; each entry carries a file/folder `key` (name/metadata are backfilled by the server from file service metadata — a client-supplied `name` has no effect) |
| `image` | `{"image_key":"img_xxx"}` |
| `file` | `{"file_key":"file_xxx"}` |
| `audio` | `{"file_key":"file_xxx"}` |
| `media` | `{"file_key":"file_xxx","image_key":"img_xxx"}` (video; `image_key` is the cover from `video_cover` — **required**) |
| `share_chat` | `{"chat_id":"oc_xxx"}` |
| `share_user` | `{"user_id":"ou_xxx"}` |
| `interactive` | Card JSON — **MUST** be produced by the topic `lark-im-card-create` workflow. Read it before writing any card; never hand-craft the JSON here |

> **`post` vs `interactive`:** `post` is a static rich-text message (title, paragraphs, @mentions, links, inline images) — content is fixed once sent. `interactive` is a card with structured layout and UI components (buttons, forms, selects, date pickers, charts) — content can be updated after sending and supports user-action callbacks. Use `post` for read-only content; use `interactive` when the message needs user interaction or dynamic updates.

`interactive` cards support callback events (`card.action.trigger`) — see topic `lark-im-card-action-reply`.

## Return Value

```json
{
  "message_id": "om_xxx",
  "chat_id": "oc_xxx",
  "create_time": "1234567890"
}
```

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

- `chat_id` and `user_id` are mutually exclusive; you must provide exactly one
- `content` must be valid JSON
- When using `content`, you are responsible for making the JSON structure match the effective `msg_type`
- If the provided media value starts with `img_` or `file_`, it is treated as an existing key and used directly
- `markdown` always sends `msg_type=post`, even if you do not explicitly set `msg_type=post`
- If you explicitly set `msg_type` and it conflicts with the chosen content flag, validation fails
- When using `video`, `video_cover` is required as the video cover
- dry-run (not available in the gateway; read and verify first) uses placeholder image keys for remote Markdown images and placeholder media keys for local uploads
- Failures return an error code and message
- app identity uses a tenant access token (TAT) and requires the `im:message:send_as_bot` scope
- When sending as a bot, the app must already be in the target group or already have a direct-message relationship with the target user
- When using `markdown` with images, pre-uploading via `images.create` to obtain an `image_key` is recommended for reliability; remote URLs may be auto-resolved at runtime, but if download/upload fails the image is removed with a warning; local paths are not supported
- **Interactive cards are gated:** you MUST read and follow the topic `lark-im-card-create` workflow to produce the card JSON *before* sending. Do not hand-write or copy a card payload — the JSON given to `msg_type interactive content` must be the workflow's output. This applies every time, with no exception
