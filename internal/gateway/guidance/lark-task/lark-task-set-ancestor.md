> 由 lark-cli `7f70de0666a2` 的 `lark-task` 参考文档转换；调用约定见 `task` 领域指南（`feishu_guide` 的 domain 参数）。

# `task.+set-ancestor`

Set a parent task for a task, or clear the parent to make it independent.

## Recommended Commands

```text
# Set a parent task
task.+set-ancestor {"task_id": "guid_1", "ancestor_id": "guid_2"}

# Clear the parent task
task.+set-ancestor {"task_id": "guid_1"}
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `task_id=<guid>` | Yes | The task GUID to update. |
| `ancestor_id=<guid>` | No | The parent task GUID. Omit it to clear the ancestor. |

## Workflow

1. Confirm the child task and, if applicable, the ancestor task.
2. Execute `task.+set-ancestor`
3. Report the updated task GUID and whether the ancestor was set or cleared.

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.
