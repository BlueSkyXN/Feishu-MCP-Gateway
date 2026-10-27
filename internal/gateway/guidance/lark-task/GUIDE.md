> 由 Feishu-MCP-Gateway 从 lark-cli `7f70de0666a2` 的 `lark-task` Skill 自动转换。
> - `docs.+fetch` 这类标识是 operation_id：具名视图调用对应工具（如 `feishu_docs_fetch`），紧凑视图用 `feishu_execute_read` / `feishu_execute_write` 传 operation_id 和 arguments。
> - CLI 参数 `--foo-bar` 对应 JSON 参数 `foo_bar`；代码块里的 `operation_id {…}` 行是参数示例，`<…>` 是要替换的占位值。代码块中仍以 `lark-cli` 开头的命令无法等价转换，只作参考。
> - 文中的“主题 `x`”用 `feishu_guide`（topic="x"）读取。
> - 网关固定以应用（bot）身份执行，没有 dry-run；高风险操作需要管理员预授权。
> - 本地文件改用 `{"file_id": "…"}`（先 `feishu_file_create` 或文件传输），输出路径填受控工作区内的相对名称；文中的本地脚本、jq、stdin 在网关中不可用。
> - 标注“网关不提供”的操作不能调用；参数以 `feishu_tools_describe` 或具名工具 schema 为准。
> - 操作结果中的 `next_command` 等后续命令是 CLI 写法，按上述规则换成对应的 operation_id 和参数再调用。

# task (v2)

## 命令选择与渐进式发现（必读）

执行任何 Task 命令前，必须先确认能力真实存在，禁止根据用户意图自行拼接或猜测 `+<verb>`：

1. 先将用户意图与下方 Shortcut 表精确匹配。只有表中明确列出的 shortcut 才可直接选择；参数不确定时读取对应 reference 或运行该 shortcut 的 `feishu_tools_describe`。
2. 没有精确匹配、或无法确认当前版本是否支持时，先运行 `feishu_tools_search`（domain="task"），以当前 CLI 输出的命令列表为准。
3. help 中存在匹配 shortcut 时，使用 help 列出的完整 shortcut token（例如 `task.+create`）运行 `feishu_tools_describe`，再按真实 flag 执行。
4. help 中没有匹配 shortcut 时，不得尝试相似的 `+<verb>`；从 help 中选择原生 resource，运行 `feishu_tools_describe` 确认 method，再运行 `feishu_tools_describe（operation_id="task.<resource>.<method>"）` 获取参数结构，最后调用 `task.<resource>.<method> {参数}`。
5. 遇到 `unknown_subcommand` 时必须停止猜测或尝试变体，回到第 2 步重新发现能力。

shortcut 名称只能来自本 Skill 的 Shortcut 表或 `feishu_tools_search`（domain="task"）；原生 resource/method 以逐级 help 为准，参数名、类型和嵌套结构以 method schema 为准。

> **任务分组定位（必读）**：
> - 在飞书任务/任务中心语境中，用户说“分组 / groups”时，优先对应 `sections`；任务清单对应 `tasklists`。先确定分组归属，再选择查询路径。
> - 归属不明时，先结合上下文判断，仍不明确再询问属于“我的任务”还是哪个清单；不要为了定位分组先遍历所有任务清单。
> - 已知 `section_guid` 时，直接用 `sections.tasks` 查询组内任务；仅有分组名称时，先列取所属资源的分组并取得 `guid`。同名候选展示 GUID 和已知归属，不自动选择第一条。
> - 分组操作的流程和 schema-first 示例见 分组操作参考（主题 `lark-task-sections`）。仅查询分组信息时，无需继续列取组内任务。

> **任务搜索技巧**：先区分用户是否**特地指定使用搜索 skill**，以及是否真的提供了**查询关键字**（例如任务名称、关键词、片段描述）。如果用户特地指定使用搜索 skill，或明确给出了任务查询关键字，则目标是**任务**时优先使用 `task.+search`（网关不提供：仅支持用户身份）。如果用户没有特地指定使用搜索 skill，且意图里没有查询关键字，只有范围条件（例如“今年以来”“已完成”“由我创建”“我关注的”），并且使用 `task.+search`（网关不提供：仅支持用户身份） 与 `task.+get-related-tasks`（网关不提供：仅支持用户身份） / `task.+get-my-tasks`（网关不提供：仅支持用户身份） 都能达到目的时，应优先使用列表型能力，而不是搜索型能力。其中，“与我相关 / 我关注的 / 由我创建”等优先考虑 `task.+get-related-tasks`（网关不提供：仅支持用户身份）；“我负责的 / 分配给我”的列表优先考虑 `task.+get-my-tasks`（网关不提供：仅支持用户身份）。不要把时间范围词（例如“今年以来”）本身误当成 `query` 去走搜索。
> **任务搜索相关性提示**：`task.+search`（网关不提供：仅支持用户身份） 当前不会自动判断搜索结果与搜索发起人的相关性。如果用户明确要求搜索“与我相关”的任务，必须先识别具体关系，获取当前用户的 `open_id`，并显式传入对应的 `assignee`（负责人）、`--creator`（创建人）或 `follower`（关注人）过滤条件；不能只依赖 `query` 期待自动返回与当前用户相关的任务。
> **意图区分补充**：像“搜索飞书中今年以来我关注的任务”这类表达，虽然字面带有“搜索”，但如果没有真正的查询关键字，且本质是在限定“与我相关 + 时间范围”，则应优先走 `task.+get-related-tasks`（网关不提供：仅支持用户身份）；像“搜索飞书中由我创建的任务清单”这类表达，如果没有清单关键字，且本质是在限定“清单范围 + 创建者”，则应优先走原生 `tasklists.list` 后筛选，而不是直接走搜索型 shortcut。
> **术语理解 — 待办 disambiguation（必读）**：
> - 用户提到「待办 / todo / 任务」时，**先判断归属**，不要默认走本 skill。
> - **走 `vc` 领域指南 的 `minutes.+todo`（网关不提供：仅支持用户身份）**（禁止本 skill）：上下文含 **妙记 / 会议纪要 / minute_token / 妙记 URL**（`/minutes/`）；或「在某某妙记里新建/修改待办」「妙记 AI 待办」「会议录制里的待办」。
> - **走本 skill（`task` 领域）**：任务清单、分配给我、项目待办、截止日期/提醒、子任务、任务清单成员；或 applink 含 `client/todo/task?guid=`；或明确说「飞书任务」「任务中心」「我的任务清单」。
> - **禁止**：用户要在妙记里加待办时，**不要**调用 `task.tasklists.list`、`task.+create` 或任何 task 命令去「找清单再放任务」。
> **友好输出**：在输出任务（或清单）的执行结果给用户时，建议同时提取并输出命令返回结果中的 `url` 字段（任务链接），以便用户可以直接点击跳转查看详情。

> **创建/更新注意**：
> 1. 只有在设置了 `due`（截止时间）的情况下，才能设置 `repeat_rule`（重复规则）和 `reminder`（提醒时间）。
> 2. 若同时设置了 `start`（开始时间）和 `due`（截止时间），开始时间必须小于或等于截止时间。
> 3. 使用 tenant_access_token（应用身份）时，无法跨租户添加任务成员。

> **查询注意**：
> 1. 在输出任务详情时，如果需要渲染负责人、创建人等人员字段，除了展示 `id` (例如 open_id) 外，还必须通过其他方式（例如调用通讯录技能）尝试获取并展示这个人的真实名字，以便用户更容易识别。
> 2. 在输出清单详情时，如果需要渲染 owner、member、角色成员等人员字段，也必须像任务成员展示一样，除了展示 `id` 外，尽量解析并展示对应人员的真实名字。
> 3. 在输出任务或清单详情时，如果需要渲染创建时间、截止时间等字段，需要使用本地时区来渲染（格式为2006-01-02 15:04:05）。

> **Task GUID 定义**：
> Task OpenAPI 中用于更新/操作任务的 `guid` 是任务的全局唯一标识（GUID），不是客户端展示的任务编号（例如 `t104121` / `suite_entity_num`）。
> 对于 Feishu 的任务 applink（例如 `.../client/todo/task?guid=...`），必须使用 URL query 里的 `guid` 参数作为 task guid。

> **从任务清单定位并修改任务的最短路径**：
> 1. 已知任务清单 GUID 时直接使用，不要先搜索；已知任务清单 applink 时，取 URL query 中的 `guid` 作为 `tasklist_guid`。
> 2. 只有清单名称或关键词、没有 GUID/applink 时，才调用一次 `task.+tasklist-search`（网关不提供：仅支持用户身份） 解析目标清单。
> 4. 从清单任务结果中取任务的 `guid`，直接传给 `task.+update` 或 `task.+complete`；禁止传客户端展示编号（例如 `t104121`）。这两个 shortcut 也可直接接收包含 `guid=` 的任务 applink。
> 5. `task.+update` 返回 `updated_fields` 和每个任务的服务端 `confirmed` 字段；`task.+complete` 返回 `status`、`completed_at`、`already_completed`。这些字段已确认目标状态时，不要例行追加 `tasks get`；仅在服务端未返回所需字段或用户明确要求完整复核时再查询详情。

| Shortcut | 说明 |
|----------|------|
| `task.+create` (topic `lark-task-create`) | create a task |
| `task.+update` (topic `lark-task-update`) | update task attributes |
| `task.+set-ancestor` (topic `lark-task-set-ancestor`) | set or clear a task ancestor |
| `task.+comment` (topic `lark-task-comment`) | add a comment to a task |
| `task.+complete` (topic `lark-task-complete`) | mark a task as complete |
| `task.+reopen` (topic `lark-task-reopen`) | reopen a completed task |
| `task.+assign` (topic `lark-task-assign`) | assign or remove task members |
| `task.+followers` (topic `lark-task-followers`) | manage task followers |
| `task.+reminder` (topic `lark-task-reminder`) | manage task reminders |
| `task.+get-my-tasks` (not available in the gateway: user identity only) (topic `lark-task-get-my-tasks`) | List tasks assigned to me |
| `task.+get-related-tasks` (not available in the gateway: user identity only) (topic `lark-task-get-related-tasks`) | list tasks related to me |
| `task.+search` (not available in the gateway: user identity only) (topic `lark-task-search`) | search tasks |
| `task.+upload-attachment` (topic `lark-task-upload-attachment`) | upload a local file as an attachment to a task |
| `task.+tasklist-create` (topic `lark-task-tasklist-create`) | create a tasklist and optionally add tasks |
| `task.+tasklist-search` (not available in the gateway: user identity only) (topic `lark-task-tasklist-search`) | search tasklists |
| `task.+tasklist-task-add` (topic `lark-task-tasklist-task-add`) | add tasks to a tasklist |
| `task.+tasklist-members` (topic `lark-task-tasklist-members`) | manage tasklist members |

## API Resources

```bash
feishu_tools_describe（operation_id="task.<resource>.<method>"）   # 调用 API 前必须先查看参数结构
task.<resource>.<method> {参数} # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `data` / `params` 参数结构，不要猜测字段格式。

### tasks

  - `create` — 创建任务
  - `delete` — 删除任务
  - `get` — 获取任务详情
  - `list` — 列取任务列表
  - `patch` — 更新任务

### tasklists

  - `add_members` — 添加清单成员
  - `create` — 创建清单
  - `delete` — 删除清单
  - `get` — 获取清单详情
  - `list` — 获取清单列表
  - `patch` — 更新清单
  - `remove_members` — 移除清单成员
  - `tasks` — 获取清单任务列表

### subtasks

  - `create` — 创建子任务
  - `list` — 获取任务的子任务列表

### members

  - `add` — 添加任务成员
  - `remove` — 移除任务成员

### sections

  - `create` — 创建自定义分组
  - `delete` — 删除自定义分组
  - `get` — 获取自定义分组详情
  - `list` — 获取自定义分组列表
  - `patch` — 更新自定义分组
  - `tasks` — 获取自定义分组任务列表

### custom_fields

  - `create` — 创建自定义字段
  - `get` — 获取自定义字段详情
  - `patch` — 更新自定义字段
  - `list` — 获取自定义字段列表
  - `add` — 将自定义字段加入资源
  - `remove` — 将自定义字段移出资源

### custom_field_options

  - `create` — 创建自定义字段选项
  - `patch` — 更新自定义字段选项

### agent

  - `update_agent_profile` — 更新任务代理的主页内容数据。
  - `register_agent` — 注册AI 智能体

### agent_task_step_info

  - `append_task_steps` — 写入任务记录。

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `tasks.create` | `task:task:write` |
| `tasks.delete` | `task:task:write` |
| `tasks.get` | `task:task:read` |
| `tasks.list` | `task:task:read` |
| `tasks.patch` | `task:task:write` |
| `tasklists.add_members` | `task:tasklist:write` |
| `tasklists.create` | `task:tasklist:write` |
| `tasklists.delete` | `task:tasklist:write` |
| `tasklists.get` | `task:tasklist:read` |
| `tasklists.list` | `task:tasklist:read` |
| `tasklists.patch` | `task:tasklist:write` |
| `tasklists.remove_members` | `task:tasklist:write` |
| `tasklists.tasks` | `task:tasklist:read` |
| `subtasks.create` | `task:task:write` |
| `subtasks.list` | `task:task:read` |
| `members.add` | `task:task:write` |
| `members.remove` | `task:task:write` |
| `sections.create` | `task:section:write` |
| `sections.delete` | `task:section:write` |
| `sections.get` | `task:section:read` |
| `sections.list` | `task:section:read` |
| `sections.patch` | `task:section:write` |
| `sections.tasks` | `task:section:read` |
| `custom_fields.create` | `task:custom_field:write` |
| `custom_fields.get` | `task:custom_field:read` |
| `custom_fields.patch` | `task:custom_field:write` |
| `custom_fields.list` | `task:custom_field:read` |
| `custom_fields.add` | `task:custom_field:write` |
| `custom_fields.remove` | `task:custom_field:write` |
| `custom_field_options.create` | `task:custom_field:write` |
| `custom_field_options.patch` | `task:custom_field:write` |
| `agent.update_agent_profile` | `task:task:write` |
| `agent.register_agent` | `task:task:write` |
| `agent_task_step_info.append_task_steps` | `task:task:write` |
