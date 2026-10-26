> 由 lark-cli `7f70de0666a2` 的 `lark-task` 参考文档转换；调用约定见 `task` 领域指南（`feishu_guide` 的 domain 参数）。

# `task.+search`（网关不提供：仅支持用户身份）

>

Search tasks by keyword and optional filters.

## Recommended Commands

```text
# Search by keyword
task.+search  # 网关不提供：仅支持用户身份

# Search incomplete tasks assigned to specific users
task.+search  # 网关不提供：仅支持用户身份

# Search by due time range
task.+search  # 网关不提供：仅支持用户身份
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `--query <string>` | No | Search keyword. If omitted, at least one filter must be provided. |
| `--creator <ids>` | No | Creator open_ids, comma-separated. |
| `assignee=<ids>` | No | Assignee open_ids, comma-separated. |
| `follower=<ids>` | No | Follower open_ids, comma-separated. |
| `--completed=<bool>` | No | Filter by completion state. |
| `due=<range>` | No | Due time range in `start,end` form. Each side supports ISO/date/relative/ms input. |
| `--page-token <string>` | No | Page token for pagination. |
| `--page-all` | No | Automatically paginate through all pages (max 40). |
| `--page-limit <int>` | No | Max page limit (default 20). |

## Workflow

1. Build the keyword and filters from the user's request.
2. Execute `task.+search`（网关不提供：仅支持用户身份）
3. Report the matched tasks and include the next `page_token` if more results exist.
