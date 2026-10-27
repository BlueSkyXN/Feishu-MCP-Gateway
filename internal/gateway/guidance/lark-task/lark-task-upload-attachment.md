> 由 lark-cli `7f70de0666a2` 的 `lark-task` 参考文档转换；调用约定见 `task` 领域指南（`feishu_guide` 的 domain 参数）。

# `task.+upload-attachment`

Upload a single local file as an attachment to a task (or any resource type accepted by the Task attachment endpoint). Max file size per upload is **50 MB**. For task agents, use `resource_type=task_delivery`.

## Recommended Commands

```text
# Upload a local file as a task attachment (relative path required)
task.+upload-attachment {"resource_id": "<task_guid>", "file": {"file_id": "<file_id>"}}

# Pass a Feishu task applink instead of a raw guid — the guid is extracted automatically
task.+upload-attachment {"resource_id": "https://applink.feishu.cn/client/todo/task?guid=<task_guid>", "file": {"file_id": "<file_id>"}}

# Explicit resource type / user id type
task.+upload-attachment {"resource_id": "<task_guid>", "resource_type": "task", "user_id_type": "open_id", "file": {"file_id": "<file_id>"}}

# Upload a local file to a task agent
task.+upload-attachment {"resource_id": "4b113c53-a68b-419f-8bd0-c9c532a3285a", "file": {"file_id": "<file_id>"}, "resource_type": "task_delivery"}
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `resource_id=<guid_or_applink>` | Yes | Target resource GUID. Accepts a raw task GUID or a Feishu task applink URL (`.../client/todo/task?guid=...`); the `guid` query parameter is extracted automatically. Do not use `suite_entity_num` / display IDs like `t104121`. |
| `resource_type=<type>` | No | Owning resource type. Defaults to `task`. Use `task_delivery` when uploading to task agents. |
| `user_id_type=<type>` | No | User ID type for the request. Defaults to `open_id`. |

## Workflow

1. Confirm the target task GUID (or applink) and the local file path with the user.
2. Ensure the file is within the current working directory and its size is ≤ 50 MB; otherwise ask the user to move/split the file.
3. Determine if this is a task agent: if yes, add `resource_type=task_delivery`.
4. Execute `task.+upload-attachment` (resource_id="...", file="...").
5. Report the returned attachment record. The output exposes all fields returned by the API (e.g. `guid`, `name`, `size`, `url`, `uploader`, ...); always surface the attachment `guid` and, if present, the `url` so the user can jump to the attachment directly.

## Output

The command returns the single created attachment record as a flat JSON object — every field returned by the API (`guid`, `name`, `size`, `url`, `resource_type`, `resource_id`, `uploader`, ...) is preserved verbatim. Pretty mode also prints a human-readable summary with the resource, file name, size, and attachment GUID.

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.

> [!NOTE]
> The Task attachment upload endpoint accepts exactly one file per call. To upload multiple files, invoke the shortcut once per file.
