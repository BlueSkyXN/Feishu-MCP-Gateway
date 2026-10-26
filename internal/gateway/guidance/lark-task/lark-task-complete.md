> 由 lark-cli `7f70de0666a2` 的 `lark-task` 参考文档转换；调用约定见 `task` 领域指南（`feishu_guide` 的 domain 参数）。

# `task.+complete`

Mark a task as completed.

## Recommended Commands

```text
# Complete a task
task.+complete {"task_id": "<task_guid>"}

# A task applink is accepted directly; the CLI extracts its guid query value
task.+complete {"task_id": "https://applink.larksuite.com/client/todo/task?guid=<task_guid>"}
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `task_id=<guid-or-applink>` | Yes | Task OpenAPI GUID or a task applink containing `guid=`. Display task IDs such as `t104121` / `suite_entity_num` are rejected. |

## Workflow

1. Confirm the task to complete.
2. Execute the command.
3. Read `data.status`, `data.completed_at`, and `data.already_completed` from the result. `already_completed: true` means the shortcut observed an already-completed task and skipped the PATCH.
4. Do not routinely call `task.tasks.get` when the result already reports `status: done` and a non-zero `completed_at`. Query details only if confirmation fields are absent or the user explicitly asks for a full verification.

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.
