> 由 lark-cli `7f70de0666a2` 的 `lark-task` 参考文档转换；调用约定见 `task` 领域指南（`feishu_guide` 的 domain 参数）。

# `task.+tasklist-task-add`

Add existing tasks to a tasklist.

## Recommended Commands

```text
# Add a single task to a tasklist
task.+tasklist-task-add {"tasklist_id": "<tasklist_guid>", "task_id": "<task_guid>"}

# Add multiple tasks to a tasklist
task.+tasklist-task-add {"tasklist_id": "<tasklist_guid>", "task_id": "<task_guid>,<another_task_guid>,<third_task_guid>"}

# Add a task to a specific section in the tasklist
task.+tasklist-task-add {"tasklist_id": "<tasklist_guid>", "task_id": "<task_guid>", "section_guid": "<section_guid>"}
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `tasklist_id=<guid>` | Yes | The GUID of the tasklist, or a full AppLink URL. |
| `task_id=<guids>` | Yes | Comma-separated list of task GUIDs to add to the tasklist. For Feishu task applinks, use each task's `guid` query parameter, not the `suite_entity_num` / display task ID like `t104121`. |
| `section_guid=<guid>` | No | The GUID of the custom section to add the tasks to. If omitted, tasks will be added to the default section. |

## Workflow

1. Confirm the tasklist and the tasks to add.
2. Execute the command `task.+tasklist-task-add`.
3. Report the result (successful vs failed tasks).

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.
