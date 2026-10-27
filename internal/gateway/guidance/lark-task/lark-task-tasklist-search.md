> 由 lark-cli `7f70de0666a2` 的 `lark-task` 参考文档转换；调用约定见 `task` 领域指南（`feishu_guide` 的 domain 参数）。

# `task.+tasklist-search` (not available in the gateway: user identity only)

>
> **⚠️ Note:** This shortcut uses tasklist search followed by tasklist detail queries to render the final output.

Search tasklists by keyword and optional filters.

## Recommended Commands

```text
# Search by keyword
task.+tasklist-search  # 网关不提供：仅支持用户身份

# Search tasklists created by specific users
task.+tasklist-search  # 网关不提供：仅支持用户身份

# Search by creation time range
task.+tasklist-search  # 网关不提供：仅支持用户身份
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `--query <string>` | No | Search keyword. If omitted, at least one filter must be provided. |
| `--creator <ids>` | No | Creator open_ids, comma-separated. |
| `--create-time <range>` | No | Creation time range in `start,end` form. Each side supports ISO/date/relative/ms input. |
| `--page-token <string>` | No | Page token for pagination. |
| `--page-all` | No | Automatically paginate through all pages (max 40). |
| `--page-limit <int>` | No | Max page limit (default 20). |

## Workflow

1. Build the search keyword and filters from the user's request.
2. Execute `task.+tasklist-search` (not available in the gateway: user identity only)
3. Report the matched tasklists and the next `page_token` if more results exist.
