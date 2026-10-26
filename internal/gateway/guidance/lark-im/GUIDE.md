> 由 Feishu-MCP-Gateway 从 lark-cli `7f70de0666a2` 的 `lark-im` Skill 自动转换。
> - `docs.+fetch` 这类标识是 operation_id：具名视图调用对应工具（如 `feishu_docs_fetch`），紧凑视图用 `feishu_execute_read` / `feishu_execute_write` 传 operation_id 和 arguments。
> - CLI 参数 `--foo-bar` 对应 JSON 参数 `foo_bar`；代码块里的 `operation_id {…}` 行是参数示例。
> - 文中的“主题 `x`”用 `feishu_guide`（topic="x"）读取。
> - 网关固定以应用（bot）身份执行，没有 dry-run；高风险操作需要管理员预授权。
> - 本地文件改用 `{"file_id": "…"}`（先 `feishu_file_create` 或文件传输），输出路径填受控工作区内的相对名称；文中的本地脚本、jq、stdin 在网关中不可用。
> - 标注“网关不提供”的操作不能调用；参数以 `feishu_tools_describe` 或具名工具 schema 为准。

# im (v1)

## Core Concepts

- **Message**: A single message in a chat, identified by `message_id` (om_xxx). Supports types: text, post, image, file, audio, video, sticker, interactive (card), share_chat, share_user, merge_forward, etc.
- **Chat**: A group chat or P2P conversation, identified by `chat_id` (oc_xxx).
- **Thread**: A reply thread under a message, identified by `thread_id` (om_xxx or omt_xxx).
- **Reaction**: An emoji reaction on a message.
- **Flag**: A bookmark on a message or thread.
- **Feed Shortcut**: A chat pinned to the current user's feed sidebar, identified by `feed_card_id` (an `oc_xxx` open_chat_id for CHAT type).
- **Feed Group**: A tag that groups feed cards in the feed list, identified by `feed_group_id` (ofg_xxx). Members are feed cards, each identified by `feed_id` + `feed_type`. Two types: `normal` (members managed explicitly) and `rule` (members auto-derived from rules).

## Resource Relationships

```
Chat (oc_xxx)
├── Message (om_xxx)
│   ├── Thread (reply thread)
│   ├── Reaction (emoji)
│   └── Resource (image / file / video / audio)
└── Member (user / bot)
```

## Important Notes

### AppLink and Share Links

Prefer CLI-returned links: use `chat_app_link` to open joined conversations, `message_app_link` to open messages, and `share_link` to invite others to groups. If manually building a joined-conversation AppLink, use `https://<applink_host>/client/chat/open?openChatId=<oc_xxx>`, never `chatId=<oc_xxx>` or `lark://...chat_id=<oc_xxx>`.

### Identity and Token Mapping

- If an IM API says it supports both `user` and `bot`, the token type changes who the operator is. The same API can succeed with one identity and fail with the other because owner/admin status, chat membership, tenant boundary, or app availability are checked against the current caller.

### Sender Name Resolution

When fetching messages (`im.+chat-messages-list`, `im.+threads-messages-list`, `im.+messages-mget`, `im.+messages-search`), the CLI shows a display name for both user and bot senders:

- **Server-provided name**: the read APIs return `sender_name` (plus the full-i18n `sender_i18n_names` map) on each message `sender`; the CLI surfaces it as the sender's `name` for users and bots alike. No name lookup and no extra permission are needed — **no contact scope** and no `application:bot.basic_info:read`.
- **Fallback to id**: when the server does not provide a name, the sender is shown by its id and the command still exits 0. There is no contact-directory fallback.

The raw `sender_name` is not duplicated in output (its value is in `name`); the full `sender_i18n_names` map (all locales) is preserved for consumers that need a specific language, alongside an optional `open_bot_id` (`ou_`) for bot senders aligned with the message-receive event channel. System messages (`msg_type: system`) have no sender name — that is normal, not an error.

### Default message enrichment (reactions / update_time)

The four message-pulling shortcuts (`im.+messages-mget`, `im.+chat-messages-list`, `im.+messages-search`, `im.+threads-messages-list`) automatically attach a `reactions` block and (for edited messages) `update_time` to each returned message — no separate `im.reactions.batch_query` call is needed. Pass `no_reactions` to opt out. For the full contract (output shape, the `im:message.reactions:read` scope requirement, and the "missing field ≠ fetch failure" data rules), read 主题 `lark-im-message-enrichment`（主题 `lark-im-message-enrichment`）.

### Compact message output (`concise`)

Some message-listing shortcuts support `concise` for compact Markdown output. Use it when the user asks for concise output or a smaller result/file; check `feishu_tools_describe` for availability and do not combine it with an explicit , an enabled `--json`, or a non-empty jq（网关不提供）.

### Opt-in resource auto-download (`download_resources`)

`im.+chat-messages-list`, `im.+messages-mget`, and `im.+threads-messages-list` accept `download_resources` to save eligible attachments into `./lark-im-resources/` and add a `resources` array to each message. It is off by default; stickers are not downloadable. A failed attachment is reported on that resource without aborting the message pull. Use `im.+messages-resources-download`（主题 `lark-im-messages-resources-download`） for one attachment. See 主题 `lark-im-message-enrichment`（主题 `lark-im-message-enrichment`） for the output contract.

**Folder resources** are containers, not files — a folder `file_key` cannot be downloaded directly. Expand it first with `im.files.folder`（file_key="<folder_key>"，srctype="message"，srcid="<message_id>"）, then download the files inside with `im.+messages-resources-download`（主题 `lark-im-messages-resources-download`）.

### Card Messages (Interactive)

**Before sending, replying with, or updating any `interactive` card (`im.+messages-send` / `im.+messages-reply` / `messages.patch`), you MUST read 主题 `lark-im-card-create`（主题 `lark-im-card-create`） and follow its workflow.** The card JSON passed to `msg_type interactive content` (send/reply) or `messages.patch data` (update) must be the output of that workflow — never hand-write or copy a card payload.

Card messages (`interactive` type) are not yet supported for compact conversion in event subscriptions. The raw event data will be returned instead, with a hint printed to stderr.

`interactive` cards support callback events (`card.action.trigger`) — see 主题 `lark-im-card-action-reply`（主题 `lark-im-card-action-reply`）.

### Audio Messages

`audio` sends a voice message and supports only Opus audio files, for example `.opus` files or Ogg Opus (`.ogg`) files. For `mp3`, `wav`, or other non-Opus audio, either convert to `.opus` first and keep using `audio`, or send the original file as an attachment with `file`.

### Sending Doc Content as a Message

When sending content fetched from a Lark doc as a message, fetch the doc with --doc-format im-markdown, then send it as a message using the `markdown` format. The fetched content is already in markdown; in any content-forwarding scenario, keep the fetched original text and send it in the `markdown` format. Note: if the doc contains a cite tag with type="user", keep it as-is and do not strip the tag.

### Flag Types

Flags support two layers:

- **Message-layer flag**: `(ItemTypeDefault, FlagTypeMessage)` — regular message bookmark
- **Feed-layer flag**: `(ItemTypeThread/ItemTypeMsgThread, FlagTypeFeed)` — thread as feed-layer bookmark

Item types for feed-layer flags:
- **ItemTypeThread** (4) = thread in a topic-style chat
- **ItemTypeMsgThread** (11) = thread in a regular chat

### Feed Shortcut

Feed shortcuts add chats to the current user's feed sidebar. They are distinct from flags:

- **Flag** = bookmark on a message/thread, scoped to the user's bookmark list.
- **Feed shortcut** = entry in the user's feed sidebar (currently only chats).

Key limits:
- Only **CHAT-type** (`feed_card_id` is `oc_xxx`) is exposed via OpenAPI; doc/app/subscription shortcuts exist internally but are not yet whitelisted.
- Batch size is **10 per call** for create/remove; list is a one-page wrapper with opaque `page_token` pagination.

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli im +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| `im.+chat-create`（主题 `lark-im-chat-create`） | Create a group chat or topic chat; user/bot; `chat_mode` group|topic; private/public; invites users/bots; optionally sets bot manager |
| `im.+chat-list`（主题 `lark-im-chat-list`） | List chats the current user/bot is a member of; defaults to groups; pass `types=p2p,group` to include p2p single chats (user-only); user/bot; supports sorting, auto-pagination, `exclude_muted` (user-only) |
| `im.+chat-messages-list`（主题 `lark-im-chat-messages-list`） | List messages in a chat or P2P conversation; user/bot; accepts `chat_id` or `user_id`, resolves P2P chat_id, supports time range, `order` asc/desc sorting, auto-pagination |
| `im.+chat-update`（主题 `lark-im-chat-update`） | Update group chat name or description; user/bot; updates a chat's name or description |
| `im.+message-read-users`（主题 `lark-im-message-read-status`） | List users who read one message; user/bot; identity-specific scopes; supports bounded auto-pagination |
| `im.+messages-mget`（主题 `lark-im-messages-mget`） | Batch get messages by IDs; user/bot; fetches up to 50 om_ message IDs, formats sender names, expands thread replies |
| `im.+messages-read-status`（网关不提供：仅支持用户身份）（主题 `lark-im-message-read-status`） | Batch query whether the current user read 1–50 messages; user-only; returns readable items and invalid message IDs |
| `im.+messages-reply`（主题 `lark-im-messages-reply`） | Reply to a message (supports thread replies); user/bot; supports text/markdown/post/media replies, reply-in-thread, idempotency key |
| `im.+messages-resources-download`（主题 `lark-im-messages-resources-download`） | Download an image/file from a message; folders are not directly downloadable — expand with `im.files.folder` first, then download the files inside; user/bot |
| `im.+messages-send`（主题 `lark-im-messages-send`） | Send a message to a chat or direct message; user/bot; sends to chat-id or user-id with text/markdown/post/media, supports idempotency key |
| `im.+threads-messages-list`（主题 `lark-im-threads-messages-list`） | List messages in a thread; user/bot; accepts om_/omt_ input, resolves message IDs to thread_id, supports `order` asc/desc sorting, auto-pagination |
| `im.+flag-create`（网关不提供：仅支持用户身份）（主题 `lark-im-flag-create`） | Create a bookmark on a message; user-only; defaults to message-layer flag; use --flag-type feed for feed-layer flag (item_type auto-detected from chat mode) |
| `im.+flag-cancel`（网关不提供：仅支持用户身份）（主题 `lark-im-flag-cancel`） | Cancel (remove) a bookmark. When no --flag-type is given, best-effort double-cancel: removes message layer and (when chat_type is determinable) feed layer |
| `im.+flag-list`（网关不提供：仅支持用户身份）（主题 `lark-im-flag-list`） | List bookmarks; user-only; auto-enriches feed-type thread entries with message content; `page_all` is capped by `page_limit` (default 20, max 1000), and `has_more=true` means the result is incomplete |
| `im.+feed-shortcut-create`（网关不提供：仅支持用户身份）（主题 `lark-im-feed-shortcut-create`） | Add chats to the user's feed shortcuts; user-only; oc_xxx chat IDs only; batch up to 10 per call; `--head`/`--tail` controls insertion order; partial failures return an `ok:false` ledger |
| `im.+feed-shortcut-remove`（网关不提供：仅支持用户身份）（主题 `lark-im-feed-shortcut-remove`） | Remove chats from the user's feed shortcuts; user-only; batch up to 10 per call; removing an absent shortcut is idempotent success; real per-item failures return an `ok:false` ledger |
| `im.+feed-shortcut-list`（网关不提供：仅支持用户身份）（主题 `lark-im-feed-shortcut-list`） | List one page of the user's feed shortcuts; user-only; omit `page_token` for the first page; default output enriches CHAT entries under `detail`; pass `--no-detail` to skip the extra lookup and `im:chat:read` scope |
| `im.+feed-group-list`（网关不提供：仅支持用户身份）（主题 `lark-im-feed-group-list`） | List the caller's feed groups (tags); user-only; supports `page_all` auto-pagination |
| `im.+feed-group-list-item`（网关不提供：仅支持用户身份）（主题 `lark-im-feed-group-list-item`） | List feed cards in a feed group (tag); user-only; enriches each item with chat_name resolved from feed_id; supports `page_all` auto-pagination |
| `im.+feed-group-query-item`（网关不提供：仅支持用户身份）（主题 `lark-im-feed-group-query-item`） | Look up specific feed cards in a feed group (tag) by ID; user-only; enriches each item with chat_name resolved from feed_id |

## API Resources

```bash
# feishu_tools_describe
lark-cli im <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `data` / `params` 参数结构，不要猜测字段格式。

### chats

  - `create` — 创建群。Identity: `bot` only (`tenant_access_token`).
  - `get` — 获取群信息。Identity: supports `user` and `bot`; the caller must be in the target chat to get full details, and must belong to the same tenant for internal chats.
  - `link` — 获取群分享链接。Identity: supports `user` and `bot`; the caller must be in the target chat, must be an owner or admin when chat sharing is restricted to owners/admins, and must belong to the same tenant for internal chats.
  - `update` — 更新群信息。Identity: supports `user` and `bot`.

### chat.members

  - `create` — 将用户或机器人拉入群聊。Identity: supports `user` and `bot`; the caller must be in the target chat; for `bot` calls, added users must be within the app's availability; for internal chats the operator must belong to the same tenant; if only owners/admins can add members, the caller must be an owner/admin, or a chat-creator bot with `im:chat:operate_as_owner`.
  - `delete` — 将用户或机器人移出群聊。Identity: supports `user` and `bot`; only group owner, admin, or creator bot can remove others; max 50 users or 5 bots per request.

### chat.managers

  - `add_managers` — 指定群管理员。Identity: supports `user` and `bot`; only the group owner can add managers; max 10 managers per chat (20 for super-large chats), and at most 5 bots per request.
  - `delete_managers` — 删除群管理员。Identity: supports `user` and `bot`; only the group owner can remove managers; max 50 users or 5 bots per request.

### chat.moderation

  - `get` — 获取群成员发言权限。Identity: supports `user` and `bot`; the caller must be in the target chat and belong to the same tenant.
  - `update` — 更新群发言权限。Identity: supports `user` and `bot`; only the group owner (or creator bot with `im:chat:operate_as_owner`) can update; the caller must be in the chat.

### messages

  - `delete` — 撤回消息。Identity: supports `user` and `bot`; for `bot` calls, the bot must be in the chat to revoke group messages; to revoke another user's group message, the bot must be the owner, an admin, or the creator; for user P2P recalls, the target user must be within the bot's availability.
  - `forward` — 转发消息。Identity: supports `user` and `bot`.
  - `merge_forward` — 合并转发消息。Identity: `bot` only (`tenant_access_token`).
  - `read_users` — 查询消息已读信息。Identity: supports `user` and `bot`; the caller must still be in the chat. A user can query messages they sent within the last 7 days, while a bot can query only messages sent by that bot within the last 7 days.Must-read（主题 `lark-im-message-read-status`）
  - `patch` — 更新已发送的消息卡片。Update an interactive message card sent by the app. Identity: supports `user` and `bot`; the message must have been sent within the last 14 days, and `content` must be a JSON-serialized string no larger than 30 KB.Must-read（主题 `lark-im-card-create`）
  - `urgent_app` — 发送应用内加急。Identity: `bot` only (`tenant_access_token`); the bot must be the message sender and must be in the conversation that contains the message.
  - `urgent_phone` — 发送电话加急。Identity: `bot` only (`tenant_access_token`); the bot must be the message sender and must be in the conversation that contains the message.
  - `urgent_sms` — 发送短信加急。Identity: `bot` only (`tenant_access_token`); the bot must be the message sender and must be in the conversation that contains the message.

### reactions

  - `batch_query` — 批量获取消息表情。Identity: supports `user` and `bot`.Must-read（主题 `lark-im-reactions`）
  - `create` — 添加消息表情回复。Identity: supports `user` and `bot`; the caller must be in the conversation that contains the message.Must-read（主题 `lark-im-reactions`）
  - `delete` — 删除消息表情回复。Identity: supports `user` and `bot`; the caller must be in the conversation that contains the message, and can only delete reactions added by itself.Must-read（主题 `lark-im-reactions`）
  - `list` — 获取消息表情回复。Identity: supports `user` and `bot`; the caller must be in the conversation that contains the message.Must-read（主题 `lark-im-reactions`）

### threads

  - `forward` — 转发话题。Identity: supports `user` and `bot`.

### pins

  - `create` — Pin 消息。Identity: supports `user` and `bot`.
  - `delete` — 移除 Pin 消息。Identity: supports `user` and `bot`.
  - `list` — 获取群内 Pin 消息。Identity: supports `user` and `bot`.

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `chats.create` | `im:chat:create` |
| `chats.get` | `im:chat:read` |
| `chats.link` | `im:chat:read` |
| `chats.update` | `im:chat:update` |
| `chat.members.create` | `im:chat.members:write_only` |
| `chat.members.delete` | `im:chat.members:write_only` |
| `chat.members.get` | `im:chat.members:read` |
| `im.+chat-members-list` | `im:chat.members:read` |
| `chat.user_setting.batch_query` | `im:chat.user_setting:read` |
| `chat.user_setting.batch_update` | `im:chat.user_setting:write` |
| `chat.managers.add_managers` | `im:chat.managers:write_only` |
| `chat.managers.delete_managers` | `im:chat.managers:write_only` |
| `chat.moderation.get` | `im:chat.moderation:read` |
| `chat.moderation.update` | `im:chat:moderation:write_only` |
| `chat.join_requests.list` | `im:chat.membership_application:read` |
| `chat.join_requests.handle` | `im:chat.membership_application:write` |
| `im.+messages-read-status`（网关不提供：仅支持用户身份） | user: `im:message:readonly` (recommended), `im:message`, or `im:message:get_as_user` |
| `im.+message-read-users` | user: `im:message:readonly` (recommended), `im:message`, `im:message:basic`, or `im:message:get_as_user`; bot: `im:message:readonly` |
| `messages.read_status` | `im:message:readonly` (recommended), `im:message`, or `im:message:get_as_user` |
| `messages.delete` | `im:message:recall` |
| `messages.forward` | `im:message` |
| `messages.merge_forward` | `im:message` |
| `messages.read_users` | user: `im:message:readonly` (recommended), `im:message`, `im:message:basic`, or `im:message:get_as_user`; bot: `im:message:readonly` |
| `messages.patch` | `im:message:update` |
| `messages.urgent_app` | `im:message.urgent` |
| `messages.urgent_phone` | `im:message.urgent:phone` |
| `messages.urgent_sms` | `im:message.urgent:sms` |
| `reactions.batch_query` | `im:message.reactions:read` |
| `reactions.create` | `im:message.reactions:write_only` |
| `reactions.delete` | `im:message.reactions:write_only` |
| `reactions.list` | `im:message.reactions:read` |
| `threads.forward` | `im:message` |
| `images.create` | `im:resource` |
| `files.create` | `im:resource` |
| `pins.create` | `im:message.pins:write_only` |
| `pins.delete` | `im:message.pins:write_only` |
| `pins.list` | `im:message.pins:read` |
| `feed.groups.batch_add_item` | `im:feed_group_v1:write` |
| `feed.groups.batch_query` | `im:feed_group_v1:read` |
| `feed.groups.batch_remove_item` | `im:feed_group_v1:write` |
| `feed.groups.create` | `im:feed_group_v1:write` |
| `feed.groups.delete` | `im:feed_group_v1:write` |
| `feed.groups.update` | `im:feed_group_v1:write` |
