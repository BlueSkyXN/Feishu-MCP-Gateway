> 由 lark-cli `7f70de0666a2` 的 `lark-task` 参考文档转换；调用约定见 `task` 领域指南（`feishu_guide` 的 domain 参数）。

# `task.+comment`

Add a comment to an existing task.

## Recommended Commands

```text
# Add a comment
task.+comment {"task_id": "<task_guid>", "content": "Looks good!"}
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `task_id=<guid>` | Yes | The task GUID to comment on. For Feishu task applinks, use the `guid` query parameter, not the `suite_entity_num` / display task ID like `t104121`. |
| `content=<text>` | Yes | The text content of the comment. |

## Workflow

1. Confirm the task and comment content.
2. Execute `task.+comment`（task_id="..."，content="..."）
3. Report success and comment ID.

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.
