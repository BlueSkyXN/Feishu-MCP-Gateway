# 0.3.0-dev 开发验收

基线验证日期：2026-10-27；2026-10-28 的修复验证见下方增量记录。本机为 macOS arm64、Go 1.27.2；GitHub Actions 为 ubuntu linux/amd64、Go 1.27.3，另用 Go 1.26.9 核对模块最低版本 Go 1.26.0。MCP SDK v1.8.0、SQLite 驱动 v1.61.0。飞书上游使用模拟 HTTP transport，测试凭据均为 fixture。

## 操作覆盖与验证强度

固定来源 `larksuite/cli@7f70de0666a21ff77ab882c62b61aff44ae3734d`：

- 206 项静态 API schema 与文件字段匹配实际受控后端。
- 392 项 shortcuts 的 bot 身份、风险与 flag 映射匹配实际导出声明。
- 190 项 user-only 明确排除；旧 `event +subscribe` 由原生 `events.read` 替代入口。
- 599 个 Operation 均通过 schema 编译；包含 source 已公开的复杂 JSON 参数 schema。
- Wiki 查询、Drive 文件列表和创建目录的 3 个 dry-run 通过。

2026-10-27 重新导入后，目录的风险、执行映射和资源绑定与 0.2.0-dev 逐项一致。变化包括 65 个操作描述和 701 个参数描述（删除用户身份说明、CLI flag 改为参数名、英文描述中的网关说明改用英文），以及 13 个 sheets 操作的 18 个参数 schema：6 个枚举参数（`include` 2 个、坐标轴标签角度 4 个）原来拒绝任何取值，7 个 `properties` 与 5 个 `sheets`/`styles` 原来拒绝 lark-cli 接受的写法。受控后端的 dry-run 确认 lark-cli 接受部分 `properties`、外层对象或数组形式的 `sheets`/`styles`、逗号拼接的 `include` 与 -45 度角。`catalog_revision` 因此改变。

以上检查目录与适配合同，598 项操作没有逐项做真实飞书实测（见外部验收边界）。端到端测试使用实际构建的 wrapper，运行以下 11 项：

| 能力 | 实际 wrapper 调用 |
|---|---|
| 静态 JSON API | wiki.spaces.get |
| 文档 | docs.+fetch、docs.+create、docs.+update |
| 文件 | im.images.create、drive.+download、drive.+export |
| Base | base.+record-list、base.+record-batch-create |
| Sheets | sheets.+workbook-info、sheets.+cells-set |

链路包含 MCP SDK 客户端、Gateway、独立 CLI 子进程、应用取票代理、模拟官方 HTTPS origin、文件存储和回执。验证了从 file_id 读取文档输入、multipart 上传、下载/导出归档、JSON 对象转 shortcut 参数、Token 跨调用复用和 actor 文件隔离。

## 用法知识与发现

- 导入脚本从同一来源生成 228 个操作级指南、15 个领域指南与 412 个主题；对同一来源重复导入，生成文件逐字节一致。
- 指南加载时与目录交叉校验：引用的操作、示例、主题、领域入口操作都必须存在。operations.json 的 24 个示例与指南代码块中的 999 条调用示例全部通过对应操作的 schema 校验（`<…>` 占位值只放宽类型、枚举与格式），参数中没有 `$'...'`、`$VAR` 残留。59 处 dry-run 预览示例改为说明注释，测试检查预览注释后不跟写入调用。操作级指南中没有 CLI 登录、`--as` 身份切换或 `lark-cli` 命令残留；全部指南文件中没有 `--as` 参数、首格残缺的表格行和按 shell 区分写法的表格，无法等价转换而保留原文的 `lark-cli` 命令 48 行，测试检查不超过此数。
- 搜索按任务说法排序：17 条中英文说法的目标操作出现在前 3 或前 5 名，包括“读取文档”“send file”“多维表格 记录”“创建日程”“新建电子表格”“往表格追加一行”“append rows to sheet”“在知识库里找文档”“查看我明天的日程”；精确 ID 只返回该操作；分页不重复。
- 搜索、describe、指南总览与 `feishu_context` 按提问者的操作策略收窄，与执行授权使用同一个判断函数；无提问者的发现按接入范围展示。
- `feishu_guide` 只开放当前请求可见领域的指南与主题，分页边界正确；关闭领域的指南与主题返回 `domain_unavailable`。
- `instructions` 按接入列出领域与调用约定；具名工具描述带适用场景。
- 仓库中的 SKILL.md 与 `--render-skill` 输出一致，其中引用的 operation_id 均存在；`scripts/package_skill.py` 生成含 427 个指南文件的 Skill 目录与 ZIP。

## 行为检查

- 2026-07-28 和 2025-11-25 官方 SDK 客户端互操作、同一连接 A→B→A、发现时无 actor、实际调用缺失 actor 拒绝。
- 具名与紧凑视图；动态参数 schema 校验、读写入口类型检查、当前 actor 策略和资源检查。
- 应用取票并发合并、连接/凭据隔离、失效处理、失败冷却、DPoP claims 和 ES256 签名验证。
- 代理签名、身份拒绝、nonce 防重放、资源路径和查询参数检查、配置撤权、出站节流与 Retry-After。
- 高风险操作需要逐操作预授权；catalog_revision 固定、应用连接禁止改绑、热加载替换工具视图、管理凭据隔离。
- 并发同键幂等、异参冲突、重启去重、unknown 不重放、Job 取消与恢复、同步调用退出前保存回执。
- 应用/客户端/actor 限额、后台队列上限、文件容量、文件读写权限、来源权限撤销、输入引用保留及 TTL。
- 快捷操作与静态 API 的分页句柄均绑定主体、操作和查询。
- HTTP 上传下载、Resources、分段文件工具、单次票据的重放/撤权/跨主体拒绝，以及 Python 文件适配器的实际上传下载。
- 加密事件 URL 验证、签名、应用和事件类型检查、去重、分页、actor 过滤、TTL 清理。
- wrapper 的原子保存、失败覆盖保护、单文件/工作区限制、路径越界及符号链接拒绝、外部请求经过代理。
- data、meta 和文件清单组成的超限回执保存在 result.json，保留原始内容。
- `/admin/status` 返回版本、指南摘要与各接入的视图和开放范围。
- 超大分页参数（搜索 `after` 接近 int64 上限、指南 `offset` 超出 int 范围）返回空页。工具与资源处理函数中的 panic 转为 `internal` 错误；执行中的 panic 写入回执（写入为 unknown，读取为 failed），同键重试返回原回执，服务继续处理请求。

## 执行结果（2026-10-27 基线）

| 检查 | macOS arm64（本机） | darwin/amd64（Rosetta 2） | linux/amd64（GitHub Actions） |
|---|---|---|---|
| `go vet`、`go test`、`go test -race`：42 个顶层测试，含实际 wrapper 的集成测试 | 通过，无跳过，无数据竞争 | `go test` 通过 | 通过 |
| Go 1.26 运行 `go vet`、`go test`（未构建 wrapper，相应集成测试跳过） | — | — | 通过 |
| wrapper 的 race 测试、vet 与构建 | 通过 | — | 通过 |
| `scripts/refresh_catalog.py` 重新生成的目录、覆盖台账与指南与提交一致 | 通过，连续两次逐字节一致 | — | 通过 |
| `scripts/verify_backend.py`：598 项定义、3 个 dry-run，无真实请求 | 通过 | — | 通过 |
| `scripts/smoke_gateway.py`：独立 Gateway 进程 | 通过 | — | 通过 |
| `build_release.py` 与 `verify_release.py`：4 个二进制的包内、PROVENANCE 记录与离线重建结果一致 | — | — | 通过 |
| 交叉构建 | linux/amd64 静态 ELF 通过 | — | darwin/arm64 通过 |

- 独立进程检查覆盖配置检查、健康检查、旧版 MCP 初始化与 instructions、工具发现、应用身份上下文与可见领域、领域指南分页、文件创建、Resources 读取、认证下载、无认证拒绝及正常退出。
- 超大 `offset` 的回归测试在修复前的代码上只在 amd64 失败，arm64 上通过。
- macOS arm64 的 Gateway 二进制约 27 MB，比 0.2.0-dev 增加约 3.3 MB，主要是内嵌指南。
- 本机原始结果保存在忽略的 `local/evidence/`：`test-0.3-review2.jsonl`、`race-0.3-review2.jsonl`、`test-0.3-review2-amd64.jsonl`、`backend-validation-review2.json`、`process-smoke-review2.json` 和 `refresh-review2-*.log`；GitHub Actions 的结果见 PR #1 的检查记录。

## 2026-10-28 修复验证

本轮在 macOS arm64、Go 1.27.2 验证，使用当前源码构建的受控 wrapper 与模拟飞书上游。目录新增 3 个 Base Record 操作的 NDJSON 产物映射；操作数量、风险和资源绑定保持不变。配套后端声明 `artifact_envelope`，缺少该能力的旧 wrapper 会在启动检查时被拒绝。

- 多行 JSON 错误及前后的提示行可以解析，保留错误类型、错误码、scopes 和 log_id；flag 解析阶段缺少 identity 的纯错误可返回，声明用户身份的回执仍被拒绝。
- `base.+record-list`、`base.+record-get`、`base.+record-search` 的 `output` 会产生可复用的 NDJSON 与 manifest，回执中的产物路径改为文件 URI；跨 actor 读取被拒绝。另覆盖 `jq_records` 的多值、尾部注释、空结果，以及 `minimal_stdout`、`overwrite`。
- 子进程 stdout/stderr 通过实际管道超过限额时返回 `backend_output_limit`；`io.Copy` 的零字节、恰好达到上限及超限输入均验证，超限内容截断后继续排空管道。
- 归档 manifest 内部的 `record_file`、`manifest_file` 使用文件 URI，并与回执一致；最终文件大小和 SHA-256 与实际存储字节匹配。检查了大整数与业务字符串保留、重复导出未变化的输入、普通 JSON 产物保持原样，以及越界/缺失引用、文件大小、工作区容量和接入配额失败时不写入无效 manifest。
- 实际 wrapper 接受 `offset=1000000`，产物分页回执保持正确的 next_offset。
- 客户端、应用、actor 三种并发额度分别达到上限时，其他客户端的任务仍执行；等待额度的任务保持 queued，可取消，撤权后记录失败。
- 用本地 Git fixture 验证来源固定：默认拒绝不同提交；显式完整提交可进入升级构建；不匹配、脏工作区和短摘要仍拒绝。构建过程由 stub 替代，不下载新来源或依赖。

| 检查 | 本轮本地结果 |
|---|---|
| `go test`、`go test -race` | 47 个顶层测试、含子测试共 77 项通过，无测试跳过，无数据竞争 |
| `go vet`、Gateway 构建 | 通过 |
| wrapper race 测试、vet、构建 | 通过，4 个顶层测试 |
| Python 回归测试 | 16 项通过：发布核对、来源固定/升级、NDJSON 指南转换 |
| 后端契约与 dry-run | 598 项定义、3 个 dry-run 通过 |
| 独立进程 smoke | 通过 |
| 目录与指南重复生成 | 430 个文件逐字节一致；Skill 已重新渲染 |

第一轮原始日志保存在忽略的 `local/evidence/20261028-review-fixes/`，补充修复后的完整测试、race 和 smoke 结果在 `local/evidence/20261028-followup-fixes/`。远端 CI、跨平台运行和完整发布包重建尚未针对本轮修改执行；上表只记录本地结果。

## 外部验收边界

本地验收未执行生产部署、飞书权限修改或真实飞书业务写入。发布与构建验证见对应 GitHub Release；尚未验证：

1. 目标应用的真实 Token/DPoP 服务行为、应用 scopes、资源 ACL、限流及 598 项操作的线上数据差异。
2. 目标 Wake 版本实际发送的 Sender、Resources/Job 请求是否保留身份、文件适配器运行目录和聊天产物展示；Wake 是否把 MCP `instructions` 交给模型，以及上传带 `references/` 的 Skill 后的读取方式。
3. 真实模型在具名与紧凑视图下的任务完成情况：选对操作的比例、参数一次填对的比例、调用轮次与耗时。
4. 指南内容与真实飞书行为的一致性；指南来自上游文档，逐项实测不在本轮范围。
5. 真实飞书事件回调与客户端事件订阅配置；官方完整 MCP conformance harness 与其他客户端。

MCP 接入采用预配 Bearer key，未声明 OAuth 授权发现兼容；没有启用标准 Tasks 扩展、旧式 SSE endpoint 或用户身份授权。
