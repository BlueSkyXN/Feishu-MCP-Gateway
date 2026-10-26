> 由 lark-cli `7f70de0666a2` 的 `lark-wiki` 参考文档转换；调用约定见 `wiki` 领域指南（`feishu_guide` 的 domain 参数）。

# `wiki` 领域 `wiki.+member-add`

Add a member to a wiki space. OpenAPI: `POST /open-apis/wiki/v2/spaces/:space_id/members`. Shortcut over the raw `wiki.members.create` — adds enum hints, optional `need_notification`, `my_library` resolution, and a flattened single-member output envelope.

> The underlying `members.create` API is flagged `danger: true` in the schema browser, but adding a member is **not** confirmation-gated (no 管理员预授权（high_risk_operations）). To revert, call `wiki.+member-remove`（主题 `lark-wiki-member-remove`） with the same `(member_id, member_type, member_role)` tuple.

## Usage

```text

# Preview the call chain without writing
wiki.+member-add {"space_id": "<space_id>", "member_id": "<id>", "member_type": "openid", "member_role": "admin"}
```

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `space_id` | string | **Yes** | — | Wiki space ID; use `my_library` for the personal document library (user only) |
| `member_id` | string | **Yes** | — | Member ID; interpretation is decided by `member_type` |
| `member_type` | enum | **Yes** | — | `openchat` / `userid` / `email` / `opendepartmentid` / `openid` / `unionid` / `appid` |
| `member_role` | enum | **Yes** | — | `admin` (full space administration) / `member` (collaborator) |
| `need_notification` | bool | No | unset | Send an in-app notification after the grant. **Omitting the flag sends no `need_notification` query at all** — passing `need_notification=false` is the explicit opt-out |

## Output

```json
{
  "space_id": "7160145948494381236",
  "member_id": "ou_449b53ad6aee526f7ed311b216aabcef",
  "member_type": "openid",
  "member_role": "admin",
  "type": "user"
}
```

`type` is a read-only enum (`user` / `chat` / `department`) the server attaches; absent when the API omits it.

## Notes

- **App member uses `member_type=appid`.** The corresponding `member_id` is the app ID, commonly formatted as `cli_xxx`.
- Resolve `member_id` **before** calling: `contact.+search-user`（网关不提供：仅支持用户身份） for users, `im.+chat-search` for groups, `POST /open-apis/contact/v3/departments/search`（CLI 通用 API 调用，网关不提供） for departments. Do not call `wiki.+member-add` first and reverse-engineer the type from the error.
- The role switch (`admin` ⇄ `member`) is not a single update — call `wiki.+member-remove`（主题 `lark-wiki-member-remove`） for the old role first, then `wiki.+member-add` with the new one.
- dry-run（网关不提供，改为先读取核对） previews 2 steps when `space_id=my_library` (resolve → add), 1 step otherwise.

## Required Scope

`wiki:member:create`
