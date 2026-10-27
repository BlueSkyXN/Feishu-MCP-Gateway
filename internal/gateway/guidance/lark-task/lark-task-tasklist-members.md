> 由 lark-cli `7f70de0666a2` 的 `lark-task` 参考文档转换；调用约定见 `task` 领域指南（`feishu_guide` 的 domain 参数）。

# `task.+tasklist-members`

Manage tasklist members (editors/owners).

## Recommended Commands

```text
# Add a member
task.+tasklist-members {"tasklist_id": "tl_xxx", "add": "ou_aaa"}

# Remove a member
task.+tasklist-members {"tasklist_id": "tl_xxx", "remove": "ou_aaa"}

# Replace all members exactly
task.+tasklist-members {"tasklist_id": "tl_xxx", "set": "ou_aaa,ou_bbb"}
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `tasklist_id=<id>` | Yes | The GUID of the tasklist, or a full AppLink URL. |
| `add=<ids>` | No | Comma-separated list of user `open_id`s to add as members. |
| `remove=<ids>` | No | Comma-separated list of user `open_id`s to remove from members. |
| `set=<ids>` | No | Comma-separated list of user `open_id`s to exactly set as members (replaces all existing). |

## Workflow

1. Confirm the tasklist and members to add/remove/set.
2. Execute the command.
3. Report success.

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.
