---
name: feishu-gateway
description: 通过 Feishu-MCP-Gateway 以飞书应用身份处理飞书/Lark 业务：云文档、电子表格、多维表格、云空间、知识库、消息与群聊、日历、任务、邮箱、OKR、幻灯片、视频会议、妙记、会议纪要、画板、Markdown 文件、思维笔记、通讯录、考勤、应用斜杠命令、妙搭应用、事件。已连接该 MCP、任务涉及飞书资源或飞书链接时使用。
---

# 飞书 MCP 网关

本 Skill 说明怎样选择操作和调用网关。参数以工具 schema 或 `feishu_tools_describe` 为准，权限由网关按接入和提问者检查。

## 开始

1. 不确定当前接入能做什么时，调用 `feishu_guide`（无参数），查看开放的领域、操作数量和入口操作。
2. 按下方路由表确定领域。复杂任务先用 `feishu_guide`（domain 填领域 ID）读领域指南；指南中写“主题 `x`”的地方，用 `feishu_guide`（topic="x"）读取。
3. 调用方式取决于接入的工具视图：
   - 具名视图：直接调用工具。operation_id 换成工具名的规则是加 `feishu_` 前缀、去掉 `+`、把 `.` 和 `-` 换成 `_`，例如 `docs.+fetch` 对应 `feishu_docs_fetch`。
   - 紧凑视图：已知 operation_id 和参数时直接调用 `feishu_execute_read` 或 `feishu_execute_write`；否则先用 `feishu_tools_search` 查找（中英文均可，可按 domain 过滤），再用 `feishu_tools_describe` 查看参数和用法。
4. 领域指南由 lark-cli 官方 Skill 转换而来。标注“网关不提供”的操作不能调用；CLI 的本地脚本、jq、stdin 和 dry-run 在网关中不可用。
5. 如果本 Skill 目录带有 `references/`，其中文件与 `feishu_guide` 返回的内容相同，可以直接读：docs、mindnotes 对应 `references/lark-doc/`，vc、minutes、note 对应 `references/lark-meeting/`，其余领域对应 `references/lark-<领域 ID>/`，入口文件为 `GUIDE.md`。

## 领域路由

| 领域 | 适用请求 | 入口操作 |
|---|---|---|
| 云文档 `docs` | 读取、创建、编辑云文档（Docx） | `docs.+fetch`、`docs.+create`、`docs.+update` |
| 电子表格 `sheets` | 读写单元格、工作表结构、图表、透视表与导入导出 | `sheets.+workbook-info`、`sheets.+csv-get`、`sheets.+cells-get`、`sheets.+csv-put`、`sheets.+table-put`、`sheets.+cells-set` |
| 多维表格 `base` | 数据表、字段、记录、视图与仪表盘 | `base.+table-list`、`base.+field-list`、`base.+record-list`、`base.+record-search`、`base.+record-get`、`base.+record-upsert`、`base.+record-batch-create` |
| 云空间 `drive` | 文件与文件夹、上传下载、导入导出、评论与权限 | `drive.+search`、`drive.+upload`、`drive.+download`、`drive.+export`、`drive.+import`、`drive.files.list` |
| 知识库 `wiki` | 知识空间、节点层级与成员 | `wiki.+node-get`、`wiki.+node-list`、`wiki.+space-list`、`wiki.+node-create` |
| 消息与群聊 `im` | 发送和回复消息、读取聊天记录、管理群聊、下载消息资源 | `im.+messages-send`、`im.+messages-reply`、`im.+chat-messages-list`、`im.+messages-mget`、`im.+chat-search`、`im.+messages-resources-download` |
| 日历 `calendar` | 日程、参会人、忙闲与会议室 | `calendar.+agenda`、`calendar.+create`、`calendar.+update`、`calendar.+freebusy`、`calendar.+room-find` |
| 任务 `task` | 任务、子任务、清单与提醒 | `task.+create`、`task.+update`、`task.+complete`、`task.+comment`、`task.+tasklist-create` |
| 邮箱 `mail` | 邮件、会话、草稿与收信规则 | `mail.+messages`、`mail.+message`、`mail.+thread`、`mail.+triage` |
| OKR `okr` | 周期、目标、关键结果与进展 | `okr.+cycle-list`、`okr.+cycle-detail`、`okr.+progress-create` |
| 幻灯片 `slides` | 创建和编辑演示文稿与页面 | `slides.+create`、`slides.+xml-get`、`slides.+add-slide`、`slides.+update-slide` |
| 视频会议 `vc` | 会议记录、会议产物与进行中的会议 | `vc.+search`、`vc.+detail`、`vc.+notes`、`vc.+recording` |
| 妙记 `minutes` | 妙记搜索、详情与媒体下载 | `minutes.+search`、`minutes.+detail`、`minutes.+download` |
| 会议纪要 `note` | 按 note_id 读取纪要信息 | `note.+detail` |
| 画板 `whiteboard` | 查询、导出和更新文档中的画板 | `whiteboard.+query`、`whiteboard.+update`、`whiteboard.+export` |
| Markdown 文件 `markdown` | 云空间中的原生 Markdown 文件 | `markdown.+fetch`、`markdown.+create`、`markdown.+patch` |
| 思维笔记 `mindnotes` | 思维笔记节点 | `mindnotes.nodes.list`、`mindnotes.nodes.create` |
| 通讯录 `contact` | 按 ID 查询用户信息 | `contact.+get-user` |
| 考勤 `attendance` | 考勤打卡结果 | `attendance.user_tasks.query` |
| 应用斜杠命令 `application` | 当前应用的 / 命令 | `application.+slash-command-list`、`application.+slash-command-create`、`application.+slash-command-update` |
| 妙搭应用 `apps` | 妙搭用户 ID 与飞书 ID 转换 | `apps.+user-id-convert` |
| 事件 `events` | 读取应用已接收的事件回调 | `events.read` |

入口操作覆盖常见任务；当前接入实际开放的操作以 `feishu_guide`、`feishu_tools_search` 或工具列表为准。

## 写入与回执

- 每次写入在参数中提供稳定的 `idempotency_key`。同一次写入重试时复用原键和原参数；改变参数时换新键。
- 结果为 `unknown` 时，先用 `feishu_job_get` 查询 invocation_id 并核对飞书中的实际对象，再决定是否重试。
- 结果为 `partial` 时，按逐项回执只处理确定失败的项目。
- 耗时操作可设 `async=true`，再用 `feishu_job_get` 查询；`feishu_job_cancel` 只停止剩余执行，已产生的飞书对象仍按回执处理。
- 高风险操作需要管理员逐项预授权后才能使用。

## 文件

- 文件输入使用 `{"file_id": "…"}`。小文本先用 `feishu_file_create` 保存；多个文件可通过 `workspace_files` 映射到受控工作区的 `inputs/` 下，保持 Markdown/XML 中的相对引用。
- 输出路径填受控工作区内的相对名称；结果中的 `files` 可以直接作为下一次操作的输入。
- 结果超过内联上限时保存为 `result.json`，用 `feishu_file_read` 按 `next_offset` 读到 `eof`。
- 需要在本机上传或下载字节时，使用宿主提供的文件适配器。接入启用了 `feishu_file_transfer` 时，先取得方向匹配的单次票据，再交给适配器传输；票据只用于这一次传输。

## 分页

沿用返回的 `page_token`、`next_page_token` 或 `cursor`。这些句柄绑定当前主体、操作和查询；改变过滤条件时从第一页开始。offset 分页沿用 `next_offset`。

## 错误与权限

- `operation_denied`：当前接入或提问者未开放该操作。换用已开放的操作，或请管理员调整接入。
- `resource_denied`：目标资源不在授权范围内。确认资源 ID，或请管理员授权该资源。
- 应用权限错误按 `error.kind`、`upstream_code`、`missing_scopes` 处理：把需要管理员开通的 scope 或资源告诉用户。

## Wake

Wake 通过已认证的连接传递提问者身份。返回 `actor_required` 时，检查该连接是否开启了 IM 提问者身份传递；工具参数和 Skill 文本不能代替身份 Header。
