# 0.3.0-dev 开发验收

验证日期：2026-10-26。开发环境为 macOS arm64、Go 1.27.2；模块最低版本 Go 1.26.0。MCP SDK v1.8.0、SQLite 驱动 v1.61.0。飞书上游使用模拟 HTTP transport，测试凭据均为 fixture。

## 操作覆盖与验证强度

固定来源 `larksuite/cli@7f70de0666a21ff77ab882c62b61aff44ae3734d`：

- 206 项静态 API schema 与文件字段匹配实际受控后端。
- 392 项 shortcuts 的 bot 身份、风险与 flag 映射匹配实际导出声明。
- 190 项 user-only 明确排除；旧 `event +subscribe` 由原生 `events.read` 替代入口。
- 599 个 Operation 均通过 schema 编译；包含 source 已公开的复杂 JSON 参数 schema。
- Wiki 查询、Drive 文件列表和创建目录的 3 个 dry-run 通过。

0.3.0-dev 重新导入后，目录的参数、风险、执行映射和资源绑定与 0.2.0-dev 逐项一致；变化只在 63 个操作描述和 700 个参数描述（删除用户身份说明、CLI flag 改为参数名）。`catalog_revision` 因此改变。

以上是目录与适配合同检查，不能当作 598 项真实飞书逐项实测。端到端测试使用实际构建的 wrapper，运行以下 11 项：

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
- 指南加载时与目录交叉校验：引用的操作、示例、主题、领域入口操作都必须存在。23 个示例的参数通过对应操作的 schema 校验。操作级指南中没有 CLI 登录、`--as` 身份切换或 `lark-cli` 命令残留。
- 搜索按任务说法排序：“读取文档”“read document”“send file”“发送文件”“上传文件”“spreadsheet read”“读取表格”“bitable record”“多维表格 记录”“知识库节点”“创建日程”的目标操作出现在前 3 或前 5 名；精确 ID 只返回该操作；分页不重复。
- 搜索、describe、指南总览与 `feishu_context` 按提问者的操作策略收窄；无提问者的发现按接入范围展示。
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

## 执行结果

测试原始结果保存在忽略的 `local/evidence/`：`test-0.3.jsonl`、`race-0.3.jsonl`、`backend-validation.json` 和 `process-smoke.json`。

- Go 常规测试与 race 回归：35 个顶层测试、41 个测试通过事件，无测试跳过，无数据竞争；vet 通过。
- wrapper 的 race 测试、vet 与构建：通过。
- `scripts/verify_backend.py`：598 项定义核对无差异，3 个 dry-run 通过，无真实请求。
- 独立 Gateway 进程：配置检查、健康检查、旧版 MCP 初始化与 instructions、工具发现、应用身份上下文与可见领域、领域指南分页、文件创建、Resources 读取、认证下载、无认证拒绝及正常退出通过。
- macOS arm64：Gateway 与 wrapper 已构建并用于上述检查；Gateway 二进制约 27 MB，比 0.2.0-dev 增加约 3.3 MB，主要是内嵌指南。
- Linux amd64：Gateway 已交叉构建为静态 ELF；未在 Linux 本机运行。

## 外部验收边界

当前没有生产部署、发布、飞书权限修改或真实飞书业务写入。尚未验证：

1. 目标应用的真实 Token/DPoP 服务行为、应用 scopes、资源 ACL、限流及 598 项操作的线上数据差异。
2. 目标 Wake 版本实际发送的 Sender、Resources/Job 请求是否保留身份、文件适配器运行目录和聊天产物展示；Wake 是否把 MCP `instructions` 交给模型，以及上传带 `references/` 的 Skill 后的读取方式。
3. 真实模型在具名与紧凑视图下的任务完成情况：选对操作的比例、参数一次填对的比例、调用轮次与耗时。
4. 指南内容与真实飞书行为的一致性；指南来自上游文档，逐项实测不在本轮范围。
5. 真实飞书事件回调与客户端事件订阅配置；官方完整 MCP conformance harness、其他客户端与 Linux 原生运行。

MCP 接入采用预配 Bearer key，未声明 OAuth 授权发现兼容；没有启用标准 Tasks 扩展、旧式 SSE endpoint 或用户身份授权。
