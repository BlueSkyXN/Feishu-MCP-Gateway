> 由 lark-cli `7f70de0666a2` 的 `lark-wiki` 参考文档转换；调用约定见 `wiki` 领域指南（`feishu_guide` 的 domain 参数）。

# `wiki` domain `wiki.+space-create` (not available in the gateway: user identity only)

Create a wiki space. OpenAPI: `POST /open-apis/wiki/v2/spaces`. This is the project-initialization entry point — the alternative is hand-writing `wiki.spaces.create` (not available in the gateway: user identity only).

> The underlying `spaces.create` API is flagged `danger: true` in the schema browser, but it is **not** confirmation-gated (no administrator pre-authorization (high_risk_operations)). A space created by mistake is recoverable via `wiki.+delete-space`.

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--name` | string | **Yes** | — | Wiki space name. Blank/whitespace is rejected (an unnamed space is almost always an accident) |
| `--description` | string | No | — | Wiki space description |

## Output

```json
{
  "space_id": "7160145948494381236",
  "name": "Engineering Wiki",
  "description": "team docs",
  "space_type": "team",
  "visibility": "private",
  "open_sharing": "closed"
}
```

There is no `url` field — the create API does not return one.

## Notes

- dry-run (not available in the gateway; read and verify first) previews the `POST /open-apis/wiki/v2/spaces` request (and surfaces the blank-name validation error early).

## Required Scope

`wiki:space:write_only`
