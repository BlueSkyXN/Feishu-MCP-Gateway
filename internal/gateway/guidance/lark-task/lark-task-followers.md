> 由 lark-cli `7f70de0666a2` 的 `lark-task` 参考文档转换；调用约定见 `task` 领域指南（`feishu_guide` 的 domain 参数）。

# `task.+followers`

Manage task followers. Add or remove followers from an existing task.

## Recommended Commands

```text
# Add a follower
task.+followers {"task_id": "<task_guid>", "add": "ou_aaa"}

# Add an app follower
task.+followers {"task_id": "<task_guid>", "add": "cli_xxx"}

# Remove a follower
task.+followers {"task_id": "<task_guid>", "remove": "ou_aaa"}
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `task_id=<guid>` | Yes | The task GUID to modify. For Feishu task applinks, use the `guid` query parameter, not the `suite_entity_num` / display task ID like `t104121`. |
| `add=<ids>` | No | Comma-separated follower IDs. Use user `open_id`s like `ou_xxx` for people, or app IDs like `cli_xxx` for apps. |
| `remove=<ids>` | No | Comma-separated follower IDs. Use user `open_id`s like `ou_xxx` for people, or app IDs like `cli_xxx` for apps. |

## Workflow

1. Confirm the task and followers to add/remove.
2. Execute the command.
3. Report success.

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.
