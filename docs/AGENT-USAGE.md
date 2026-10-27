# Agent 使用层

Gateway 用固定的操作目录执行飞书请求，用一层“用法知识”帮助 Agent 选对操作、按正确顺序完成任务。两层来自同一个固定版本的 lark-cli，随二进制一起发布。

## 组成

| 部分 | 内容 | 版本标识 |
|---|---|---|
| 操作目录 `internal/gateway/catalog.json` | 599 个 Operation 的参数 schema、风险、执行映射和资源绑定 | `catalog_revision` |
| 用法指南 `internal/gateway/guidance/` | 228 个操作级指南（适用场景、不适用场景、前置条件、提示、示例），15 个领域指南与 412 个主题 | `guidance_revision` |
| 领域表 `internal/gateway/domains.go` | 22 个领域的名称、别名和入口操作 | 随代码版本 |
| Skill `skills/feishu-gateway/SKILL.md` | 入口说明与领域路由表，由领域表渲染 | 随代码版本 |

指南只影响 Agent 看到的说明，不改变可执行的操作、权限和资源检查，因此单独计算 `guidance_revision`，不计入 `catalog_revision`。两个摘要都可以从 `--version`、`feishu_context` 和 `/admin/status` 读取。

## Agent 怎样使用

所有接入都提供：

- MCP `instructions`：按接入生成，列出开放的领域、操作数量和调用约定。支持该字段的宿主会在会话开始时交给模型。
- `feishu_guide`：无参数返回当前请求可见的领域、操作数量和入口操作；`domain` 返回领域指南；`topic` 返回指南中引用的主题。长内容通过 `offset` / `next_offset` 分页。
- `feishu_context`：应用身份、工具视图、可见领域与操作数量、目录与指南版本。

具名视图（`tool_view=named`）中，每个开放操作是一个工具，工具描述附带指南里的适用与不适用场景。operation_id 与工具名的对应规则是加 `feishu_` 前缀、去掉 `+`、把 `.` 和 `-` 换成 `_`。

紧凑视图（`tool_view=compact`）固定提供少量入口：

- `feishu_tools_search`：按任务说法查找操作，中英文均可（如“读取文档”“send file”“多维表格 记录”），可用 `domain`、`risk` 过滤；`query` 为空时按领域浏览。精确的 operation_id 或工具名只返回该操作。没有结果时返回可用领域和改写建议。
- `feishu_tools_describe`：参数 schema、返回结构、应用 scopes、风险，以及该操作的指南。
- `feishu_execute_read` / `feishu_execute_write`：已知 operation_id 与参数时直接调用，不必每次先 search 和 describe。

搜索的排序规则：查询拆成若干条件（中文按词表最长匹配，英文按词拆分并做同义归一，英文虚词不计入条件；“明天”“本周”等时间词只为日程视图加分），优先返回全部条件都命中的操作；命中位置按 operation_id 与工具名、领域别名、能力包、参数名、描述与适用场景依次降权；领域入口操作和 shortcut 在同等命中时靠前，已隐藏的旧命令靠后。没有操作满足全部条件时，返回部分命中结果并标记 `match: partial`。

## 提问者级发现

搜索、describe、指南总览和 `feishu_context` 使用与执行授权相同的判断：

- 接入通过 `packs` 与 `operations` 的并集开放操作；`resource_scoped` 接入只开放有资源绑定的操作；高风险操作需要出现在 `high_risk_operations` 中。
- 请求带有 Wake 提问者时，再按该提问者的 `operations` 收窄；提问者配置了 `resources` 时，隐藏无法校验资源范围的操作。
- 发现阶段可以没有提问者，此时按接入范围展示；实际调用、文件、Job 访问仍要求提问者身份。

因此，同一个 Wake 连接下，不同提问者在搜索结果和指南总览中看到的是各自能执行的操作。

## 选择工具视图

| 接入用途 | 建议配置 |
|---|---|
| 职责固定的 Waker，例如周报助手、文档助手 | `named` + `operations` 白名单（`packs` 留空）+ Skill |
| 通用飞书助手，需要跨多个领域 | `compact` + Skill |
| 宿主会延迟加载 MCP 工具（按需搜索后才载入完整工具定义） | `named` 可以开放较大范围 |

`tool_view` 未填写时为 `named`。具名视图在业务工具之外固定附带 9 个通用工具（指南、上下文、Job 2 个、文件 5 个），启用文件传输票据时再加 1 个。

## Skill

`skills/feishu-gateway/SKILL.md` 说明调用约定、写入与回执、文件、分页、错误处理和 Wake 身份，并给出领域路由表。宿主支持 Skill 时与 MCP 连接一起安装；不支持时，`instructions` 与 `feishu_guide` 提供同样的入口。

需要离线指南时，用打包脚本生成带 `references/` 的 Skill 目录或 ZIP（ZIP 内为 `feishu-gateway/SKILL.md` 与 `feishu-gateway/references/`），按 Wake 的“包含 SKILL.md 和必要资源的压缩包”方式上传：

```sh
python3 scripts/package_skill.py local/skill --zip
```

`references/<上游 Skill 名>/GUIDE.md` 与主题文件的内容与 `feishu_guide` 返回的相同。发布包中的 `skills/feishu-gateway/` 已包含这些文件。

修改 `domains.go` 或 `internal/gateway/skill_template.md` 后，重新渲染：

```sh
go run ./cmd/gateway --render-skill > skills/feishu-gateway/SKILL.md
```

`go test` 会检查仓库中的 SKILL.md 与渲染结果一致，并检查其中引用的 operation_id 都存在。

## 指南如何生成

`scripts/import_catalog.py` 调用 `scripts/lark_guidance.py`，从固定 lark-cli 的三类内容生成指南：

1. `affordance/*.md`：按上游 `internal/affordance/mdparse.go` 的固定 markdown 子集解析，得到适用场景、不适用场景、前置条件、提示、示例和相关主题。
2. shortcut 的 Go Tips：由受控后端 `--gateway-export-shortcuts` 导出；同一操作在 affordance 中写了 Tips 时以 affordance 为准，与上游规则一致。
3. 15 个官方 Skill 的 SKILL.md 与参考文档：转换为领域指南和主题。

导入的 Skill 为 lark-doc、lark-sheets、lark-base、lark-drive、lark-wiki、lark-im、lark-calendar、lark-task、lark-mail、lark-okr、lark-slides、lark-meeting、lark-whiteboard、lark-markdown、lark-contact。lark-minutes、lark-vc、lark-vc-agent、lark-note、lark-event、lark-attendance 只把引用改写为对应领域，内容由 lark-meeting 或网关工具覆盖。以下 Skill 不导入：

| Skill | 原因 |
|---|---|
| lark-approval | 审批操作仅支持用户身份 |
| lark-apps | 妙搭应用的操作几乎都仅支持用户身份 |
| lark-shared | CLI 的登录、身份切换与配置 |
| lark-skill-maker、lark-openapi-explorer | 制作 CLI Skill、通过 CLI 调用未封装的 OpenAPI |
| lark-workflow-standup-report、lark-workflow-meeting-summary | 汇总“我的”日程、待办和会议，依赖用户身份操作 |

转换规则：

- `lark-cli docs +fetch --detail with-ids`、`docs +fetch`、`+fetch`、`[[+fetch]]` 等写法改为 operation_id，参数改为 JSON 参数名，例如 `docs.+fetch`（detail="with-ids"）；代码块中的命令改为 `operation_id {参数}` 行。
- 网关目录中没有的操作标注“网关不提供”及原因（例如仅支持用户身份）。
- `--print-schema`、`--help` 和 `lark-cli schema` 改为 `feishu_tools_describe`；`--yes` 改为管理员预授权；`--dry-run`、jq 标注网关不提供。说明 CLI 语法的模板（如 `lark-cli im +<verb> [flags]`）改为 operation_id 写法。
- 网关固定以应用身份执行：只提到 `--as bot` 的内容保留，改写为“应用身份”，命令中的该参数去掉；涉及用户身份的内容删除。
- 只适用于 CLI 登录授权、本机路径或 CLI 自身管理的行和条目会被删除；描述 CLI 全局参数的表格行、按 shell 区分写法的表格整行或整表删除。删除部分约占上游 Markdown 行数的 2%。表格续行的空首格写“同上”。
- affordance 与 Go Tips 是英文，插入其中的网关说明也用英文。
- Skill 内的相对链接改为“主题 `x`”或领域指南引用。

同一次导入还会清理目录中的操作与参数描述：删除用户身份说明，把 CLI flag 写法改为本操作的参数名。目录的参数、风险和执行映射不变。

已知限制：32 行无法安全转换的 `lark-cli` 写法保持原文，多为带管道、heredoc 或变量替换的 shell 片段和 CLI 输出示例；每篇指南开头说明这些 CLI 专用能力在网关中不可用，以及结果中的 `next_command` 怎样换成 operation_id。`go test` 检查残留不超过这个数，并检查指南中没有身份切换参数和残缺的表格行。指南内容来自上游文档，没有逐项做真实飞书验证。

## 升级 lark-cli

1. 把 lark-cli 源码检出到目标提交，保持工作区干净。
2. 重新生成目录、覆盖台账和指南：

   ```sh
   python3 scripts/refresh_catalog.py /path/to/lark-cli
   ```

3. 核对 `git diff`：`docs/coverage.json` 中新增或排除的操作、能力包变化、指南的新增与删除。
4. 运行 `go test ./...`。领域入口操作失效或指南引用了不存在的操作时，测试会指出具体项；需要时更新 `domains.go`。
5. 重新渲染 Skill，完成四项验证、`scripts/verify_backend.py` 与 `scripts/smoke_gateway.py`。
6. 部署新版本时，用 `--catalog-revision` 取得新摘要并更新配置中的 `catalog_revision`。
