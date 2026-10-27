> 由 lark-cli `7f70de0666a2` 的 `lark-task` 参考文档转换；调用约定见 `task` 领域指南（`feishu_guide` 的 domain 参数）。

# `task.+update`

Update an existing task in Lark.

## Recommended Commands

```text
# Update task summary
task.+update {"task_id": "<task_guid>", "summary": "New Summary"}

# Update multiple tasks' due dates
task.+update {"task_id": "<task_guid>,<another_task_guid>", "due": "+2d"}

# A task applink is accepted directly; the CLI extracts its guid query value
task.+update {"task_id": "https://applink.larksuite.com/client/todo/task?guid=<task_guid>", "summary": "New Summary"}

# Update with JSON data
task.+update {"task_id": "<task_guid>", "data": "{\"description\": \"New description\"}"}
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `task_id=<guid-or-applink>` | Yes | Task OpenAPI GUID or a task applink containing `guid=`. Comma-separated GUIDs/applinks are supported for multiple tasks. Display task IDs such as `t104121` / `suite_entity_num` are rejected. |
| `summary=<text>` | No | New summary/title for the task. |
| `description=<text>` | No | New description for the task. |
| `due=<time>` | No | New due date (supports relative time). |
| `data=<json>` | No | JSON payload for fields to update. |

## Workflow

1. Confirm with the user the tasks to update and the fields.
2. Execute `task.+update` (task_id="...")
3. Read `data.updated_fields` and `data.tasks[].confirmed` from the result and report only the fields confirmed by the server.
4. Do not routinely call `task.tasks.get` after the update when `confirmed` already contains the required state. Query details only if a required field is absent or the user explicitly asks for a full verification.

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.
