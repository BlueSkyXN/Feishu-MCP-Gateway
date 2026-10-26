> 由 lark-cli `7f70de0666a2` 的 `lark-wiki` 参考文档转换；调用约定见 `wiki` 领域指南（`feishu_guide` 的 domain 参数）。

# `wiki` 领域 `wiki.+member-list`

List the members of a wiki space. OpenAPI: `GET /open-apis/wiki/v2/spaces/:space_id/members`. **Default fetches a single page** (matches `wiki.+space-list` / `wiki.+node-list`); pass `page_all` to walk every page.

## Usage

```text
# Default: single page
wiki.+member-list {"space_id": "<space_id>"}

# Walk every page (capped by --page-limit, default 10)
wiki.+member-list {"space_id": "<space_id>", "page_all": true}

# Walk every page, no cap
wiki.+member-list {"space_id": "<space_id>", "page_all": true, "page_limit": 0}

# Resume from a specific cursor (single-page fetch regardless of --page-all)
wiki.+member-list {"space_id": "<space_id>", "page_token": "<TOKEN>"}

# Pretty / table / csv / ndjson output
wiki.+member-list {"space_id": "<space_id>"}
wiki.+member-list {"space_id": "<space_id>"}
```

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `space_id` | string | **Yes** | — | Wiki space ID; use `my_library` for the personal document library (user only) |
| `page_size` | int | No | 50 | Page size, 1-50 |
| `page_token` | string | No | — | Page cursor; implies single-page fetch (no auto-pagination) |
| `page_all` | bool | No | `false` | Automatically paginate through all pages (capped by `page_limit`) |
| `page_limit` | int | No | 10 | Max pages with `page_all` (0 = unlimited) |
|  | enum | No | `json` | `json` / `pretty` / `table` / `csv` / `ndjson` |

## Output

```json
{
  "ok": true,
  "data": {
    "space_id": "7160145948494381236",
    "members": [
      {
        "member_id": "ou_449b53ad6aee526f7ed311b216aabcef",
        "member_type": "openid",
        "member_role": "admin"
      },
      {
        "member_id": "ou_67e5ecb64ce1c0bd94612c17999db411",
        "member_type": "openid",
        "member_role": "member"
      }
    ],
    "has_more": false,
    "page_token": ""
  },
  "meta": { "count": 2 }
}
```

`type` (`user` / `chat` / `department`) is included when the server returns it. When the default single-page fetch (or `page_all` capped by `page_limit`) does not exhaust the upstream cursor, `has_more=true` and `page_token=<cursor>` so the caller can resume.

## Notes

- Use `member_id` from the output as `member_id` for `wiki.+member-remove`（主题 `lark-wiki-member-remove`）; `member_type` and `member_role` must be passed exactly as listed to remove a grant.
- dry-run（网关不提供，改为先读取核对） previews 2 steps when `space_id=my_library` (resolve → list), 1 step otherwise.

## Required Scope

`wiki:member:retrieve`
