> 由 lark-cli `7f70de0666a2` 的 `lark-task` 参考文档转换；调用约定见 `task` 领域指南（`feishu_guide` 的 domain 参数）。

# `task.+create`

Create a new task in Lark.

## Recommended Commands

```text
# Create a task with all details
task.+create {"summary": "Quarterly Sales Review", "description": "Review the sales performance for the last quarter.", "assignee": "ou_xxx", "due": "2026-03-25", "tasklist_id": "https://applink.larkoffice.com/client/todo/task_list?guid=a4b00000-000-000-000-00000000036c"}

# Create a task assigned to an app
task.+create {"summary": "Nightly Sync", "assignee": "cli_xxx"}

# Create a simple task
task.+create {"summary": "Buy milk"}

# Create a milestone by passing an API field without a named flag
task.+create {"summary": "Release v2.0", "due": "2026-08-15", "data": "{\"is_milestone\":true}"}

# Preview the API call without executing
task.+create {"summary": "Test Task"}
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `summary=<text>` | Yes | The title or summary of the task |
| `description=<text>` | No | Detailed description of the task |
| `assignee=<id>` | No | Assignee ID. Use user `open_id` like `ou_xxx` for people, or app ID like `cli_xxx` for apps. |
| `follower=<id>` | No | Follower ID. Use user `open_id` like `ou_xxx` for people, or app ID like `cli_xxx` for apps. |
| `due=<time>` | No | Due date. Supports ISO 8601, `YYYY-MM-DD`, relative time (e.g., `+2d`), or ms timestamp. `YYYY-MM-DD` and relative time will automatically set it as an all-day task. |
| `tasklist_id=<id>` | No | The GUID of the tasklist, or a full AppLink URL (the CLI will automatically extract the `guid` parameter from the URL). |
| `idempotency_key=<key>` | No | Client token to ensure idempotency of the request. |
| `data=<json>` | No | JSON object merged into the task create request for API fields without dedicated flags, such as `{"is_milestone":true}`. Explicit named flags override same-named fields in this object. |

> **Required:** If `task.+create` has no dedicated flag for a field requested by the user, first inspect `feishu_tools_describe (operation_id="task.tasks.create")`, then add that field to `data` using the exact field name, type, and nesting from the Meta API request-body schema. Do not omit requested fields or guess their JSON shape. Keep fields already supplied through dedicated flags out of `data`.

Prefer this shortcut over the raw `tasks create` command when `data` can express the request. Do not assume that other shortcuts support `data`; check each shortcut's `feishu_tools_describe` output first.

## Workflow

1. Confirm with the user: task summary, due date, assignee, and tasklist if necessary.
2. Execute `task.+create` (summary="...")
3. Judge success by `ok == true` in the stdout JSON (the success envelope has no `code` field — do not test `code == 0`), then report the result: task ID (`data.guid`) and summary.

Example success response:

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "guid": "e297d3d0-4b60-4a5f-a4d4-xxxxxxxxxxxx",
    "url": "https://applink.larkoffice.com/client/todo/detail?guid=e297d3d0-4b60-4a5f-a4d4-xxxxxxxxxxxx"
  }
}
```

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.

## References

- `task` domain指南 -- All task commands
