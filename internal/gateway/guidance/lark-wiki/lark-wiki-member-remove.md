> 由 lark-cli `7f70de0666a2` 的 `lark-wiki` 参考文档转换；调用约定见 `wiki` 领域指南（`feishu_guide` 的 domain 参数）。

# `wiki` domain `wiki.+member-remove`

Remove a member from a wiki space. OpenAPI: `DELETE /open-apis/wiki/v2/spaces/:space_id/members/:member_id`. Unlike most DELETEs, this endpoint **requires a body** carrying `member_type` and `member_role` — the `:member_id` path segment alone is ambiguous without both.

> The underlying `members.delete` API is flagged `danger: true` in the schema browser, but the operation is recoverable — call `wiki.+member-add` (topic `lark-wiki-member-add`) with the same `(member_id, member_type, member_role)` to restore. No administrator pre-authorization (high_risk_operations) gate.

## Usage

```text

# Preview the call chain without deleting
wiki.+member-remove {"space_id": "<id>", "member_id": "<id>", "member_type": "openid", "member_role": "admin"}
```

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `space_id` | string | **Yes** | — | Wiki space ID; use `my_library` for the personal document library (user only) |
| `member_id` | string | **Yes** | — | Member ID; interpretation is decided by `member_type` |
| `member_type` | enum | **Yes** | — | Must **match the original grant**: `openchat` / `userid` / `email` / `opendepartmentid` / `openid` / `unionid` / `appid` |
| `member_role` | enum | **Yes** | — | Must **match the original grant**: `admin` / `member` |

## Output

```json
{
  "space_id": "7160145948494381236",
  "member_id": "ou_449b53ad6aee526f7ed311b216aabcef",
  "member_type": "openid",
  "member_role": "admin"
}
```

If the API ever omits the member echo, the CLI falls back to surfacing the caller-supplied `(member_id, member_type, member_role)` so scripts still see what was removed.

## Notes

- **`member_type` and `member_role` must match the original grant.** Revoking a non-existent `(member_id, type, role)` tuple is a no-op error from the API. If you do not know the current role, run `wiki.+member-list` (topic `lark-wiki-member-list`) first.
- **Role switch is not a single update.** To move someone between `admin` and `member`, call `wiki.+member-remove` with the old role first, then `wiki.+member-add` (topic `lark-wiki-member-add`) with the new one.
- **Bot + `my_library` is rejected upfront.** Pass an explicit `space_id` when app identity.
- dry-run (not available in the gateway; read and verify first) previews 2 steps when `space_id=my_library` (resolve → delete), 1 step otherwise.

## Required Scope

`wiki:member:update`
