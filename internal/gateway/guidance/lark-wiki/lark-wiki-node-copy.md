> 由 lark-cli `7f70de0666a2` 的 `lark-wiki` 参考文档转换；调用约定见 `wiki` 领域指南（`feishu_guide` 的 domain 参数）。

# `wiki` domain `wiki.+node-copy`

Copy one Wiki node, including that node's content, to a target space or under a target parent node. Descendant nodes are not copied.

> ⚠️ **High-risk write** — the upstream API is flagged `danger: true`, so this shortcut requires explicit administrator pre-authorization (high_risk_operations) confirmation before issuing the request. Forgetting administrator pre-authorization (high_risk_operations) returns a `confirmation_required` error and the copy is **not** performed.

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `space_id` | **Yes** | Source wiki space ID |
| `node_token` | **Yes** | Source node token to copy |
| `target_space_id` | Conditional | Target space ID. Required if `target_parent_node_token` is not set |
| `target_parent_node_token` | Conditional | Target parent node token. Required if `target_space_id` is not set |
| `title` | No | New title for the copied node. Omit to keep the original title |

> At least one of `target_space_id` or `target_parent_node_token` must be provided.

## Output

```json
{
  "space_id": "target_space_id",
  "node_token": "wikcn_EXAMPLE_TOKEN",
  "obj_token": "doccn_EXAMPLE_TOKEN",
  "obj_type": "docx",
  "node_type": "origin",
  "title": "Getting Started (Copy)",
  "parent_node_token": "",
  "has_child": false
}
```

## Notes

- Copying is non-recursive: only the requested node and its content are copied.
- Descendant nodes must be copied separately.
- When the Wiki service returns `131009` lock contention, the CLI retries twice with bounded exponential backoff. If contention remains, wait before retrying again and avoid concurrent writes under the same target parent.
- To move an existing Wiki node without keeping the source, use `wiki.+move` (topic `lark-wiki-move`) instead of copy-then-delete.

## Required Scope

`wiki:node:copy`
