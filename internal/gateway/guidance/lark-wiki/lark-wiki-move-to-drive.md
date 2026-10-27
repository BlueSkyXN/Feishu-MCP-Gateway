> 由 lark-cli `7f70de0666a2` 的 `lark-wiki` 参考文档转换；调用约定见 `wiki` 领域指南（`feishu_guide` 的 domain 参数）。

# `wiki.+move-to-drive`

将已有 Wiki 节点移出知识库，并放到指定 Drive 文件夹；省略目标文件夹时放到当前调用身份的“我的空间”根目录。该操作始终创建异步任务，shortcut 会自动有限轮询。

## 何时使用

| 源对象 | 目标位置 | 命令 |
|--------|----------|------|
| Wiki 节点 | Wiki 空间或 Wiki 父节点 | `wiki.+move` |
| Drive 文档 | Wiki 空间或 Wiki 父节点 | `wiki.+move` |
| Wiki 节点 | Drive 文件夹或“我的空间”根目录 | `wiki.+move-to-drive` |
| Drive 文件 / 文件夹 | Drive 文件夹或根目录 | `drive.+move` |

`node_token` 必须是 Wiki 节点 token，不是底层文档的 `obj_token`。无法判断时，先执行 `wiki.+node-get`（node_token="<URL_OR_TOKEN>"）。

## 命令

```text
# 移到指定 Drive 文件夹

# 移到当前调用身份的“我的空间”根目录

# 预览提交任务和轮询任务两步请求
# 网关不提供 dry-run 预览，此示例省略
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `node_token` | 是 | 要移出知识库的 Wiki 节点 token |
| `folder_token` | 否 | 目标 Drive 文件夹 token；省略时移动到当前调用身份的“我的空间”根目录 |

## 异步协议与续跑

shortcut 会按以下协议执行：

1. `POST /open-apis/wiki/v2/nodes/{node_token}/move_wiki_to_docs`，取得完整、不可拆分的 `task_id`。
2. `GET /open-apis/wiki/v2/tasks/{task_id}?task_type=move_wiki_to_docs`。
3. 读取 `data.task.move_wiki_to_docs_result`：`status=1` 表示处理中，`status=0` 表示成功，`status=-1` 表示失败。

任务查询必须使用 `task_type=move_wiki_to_docs`、`move_wiki_to_docs_result` 和数值状态；不要回退到其他 task type、result 字段或字符串状态。

- 最多轮询 30 次，每次间隔 2 秒。
- 轮询窗口内成功时返回 `ready=true`，并尽可能返回 `obj_token`、`obj_type` 和 `url`。
- 仍在处理中时返回 `ready=false`、`timed_out=true`、完整 `task_id` 和 `next_command`；超时不代表任务失败。
- 任务进入失败态时返回结构化错误。
- `task_id` 是服务端签名的 opaque ID，可能包含多个连字符；必须原样保存，不能自行切分。

手动续跑命令：

## 典型返回

成功：

```json
{
  "node_token": "wikcnXXX",
  "folder_token": "fldcnXXX",
  "task_id": "<OPAQUE_TASK_ID>",
  "ready": true,
  "failed": false,
  "status": 0,
  "status_msg": "success",
  "obj_token": "doxcnXXX",
  "obj_type": "docx",
  "url": "https://example.feishu.cn/docx/doxcnXXX"
}
```

轮询窗口超时：

```json
{
  "node_token": "wikcnXXX",
  "folder_token": "",
  "task_id": "<OPAQUE_TASK_ID>",
  "ready": false,
  "failed": false,
  "status": 1,
  "status_msg": "processing",
  "timed_out": true,
  "next_command": "lark-cli drive +task_result --scenario wiki_move_to_drive --task-id <OPAQUE_TASK_ID>"
}
```

## 权限与影响

- CLI 写操作预检查使用 `space:document:move`，任务轮询使用 `wiki:space:read`。
- 调用方必须能移动源 Wiki 节点并写入目标 Drive 文件夹。
- 成功后源节点会从 Wiki 树中消失，目标文档改用 Drive 目标位置的权限模型；原 Wiki 层级继承权限不再保留。

> [!CAUTION]
> 这是会改变文档归属和权限继承的**写入操作**。执行前必须确认源 Wiki 节点、目标 Drive 位置和调用身份。

## 参考

- `wiki` 领域指南 -- 知识库全部命令
- `wiki.+move`（主题 `lark-wiki-move`） -- Wiki 内移动与 Drive 文档迁入 Wiki
- drive +task_result（主题 `lark-drive-task-result`） -- 超时后的任务续跑
