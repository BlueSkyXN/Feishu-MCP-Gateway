# Feishu-MCP-Gateway

通用飞书 MCP Server，替代 Agent 直接使用 `lark-cli` 的能力入口。服务提供应用身份操作、可复用文件和后台调用；多应用、多个客户端与 Wake 提问者使用独立的授权范围。

飞书执行身份固定为 **bot / TAT**。App Secret、Token 和 DPoP 私钥保留在 Gateway 进程内。服务使用受控的 `lark-cli-gateway` 后端，后端只能通过本次调用的临时代理访问已授权接口。

项目采用 [GPL-3.0-only](LICENSE)（GNU GPL 第 3 版）。第三方代码保留各自的版权与许可，见 [第三方声明](THIRD_PARTY_NOTICES.md)。

当前版本 `0.3.0-dev`，使用官方 MCP Go SDK v1.8.0，支持 `2026-07-28` 与 `2025-11-25` Streamable HTTP。认证采用管理员预配的 Bearer key。没有用户 OAuth、UAT 或个人 profile 登录流程。

## 能力范围

来源固定为 lark-cli `7f70de0666a21ff77ab882c62b61aff44ae3734d`。源目录的 789 项包括 251 个静态 API 与 538 个 shortcuts：

| 来源 | 应用身份适配 | 排除 |
|---|---:|---:|
| 静态 API | 206 | 45 项仅用户身份 |
| shortcuts | 392 | 145 项仅用户身份、1 项旧事件入口 |

598 项来源适配加上原生 `events.read`，共 **599 个 Operation**。文件、上下文、Job、指南和紧凑发现入口另外提供固定工具。目录逐项记录见 [coverage.json](docs/coverage.json)；`adapter_ready` 表示有 schema 和执行映射，真实飞书验收范围见 [VALIDATION.md](docs/VALIDATION.md)。

同一固定版本的用法知识也随目录导入：228 个操作级指南来自 affordance 与 shortcut Tips，15 个领域指南与 412 个主题来自官方 Skill 文档，均已改写为网关的调用方式。说明见 [Agent 使用层](docs/AGENT-USAGE.md)。

主要能力：

- 默认按能力包生成具名工具；`tool_view=compact` 提供 search、describe、execute_read、execute_write，使用同一目录、校验器和执行器。
- `feishu_guide` 提供领域总览、领域指南与主题；紧凑视图的搜索支持中英文任务说法和领域过滤。发现结果按接入和提问者的权限收窄。
- 应用取票缓存、并发刷新合并、DPoP、出站节流、Retry-After、临时调用凭据和代理防重放。
- 客户端及 actor 的操作/资源权限，应用/客户端/actor 并发与调用频率限制，配置热加载与撤权。
- 写入幂等、SQLite 回执、后台 Job、取消和重启恢复。不确定写入保存为 `unknown`。
- 文件输入、产物归档、跨操作复用、Resources、有界读取、HTTP 传输和单次传输票据。
- 加密事件回调接入、去重和有界事件读取。

## 下载

源码：[BlueSkyXN/Feishu-MCP-Gateway](https://github.com/BlueSkyXN/Feishu-MCP-Gateway)。

[GitHub Releases](https://github.com/BlueSkyXN/Feishu-MCP-Gateway/releases) 提供 macOS arm64、Linux amd64 的 Gateway 与配套后端、校验和、对应源码包。下载后核对 SHA256SUMS，并按示例配置应用凭据和接入权限。

## 构建

需要 Go 1.26 或更新版本、Python 3，以及上述固定提交的已有 lark-cli 源码。构建脚本验证来源 Git 状态，不修改来源工作区。受控 wrapper 单独构建，避免把 Cobra 的进程全局状态放进多用户服务。

```sh
python3 scripts/build_backend.py /path/to/lark-cli --test
go test ./...
go test -race ./...
go vet ./...
go build -trimpath -buildvcs=false -o bin/feishu-mcp-gateway ./cmd/gateway
python3 scripts/verify_backend.py bin/lark-cli-gateway
python3 scripts/smoke_gateway.py bin/feishu-mcp-gateway bin/lark-cli-gateway
```

`build_backend.py` 生成忽略的构建配置并下载所需 Go 模块；后端固定到来源提交。`--test` 运行 wrapper 的 race 测试与 vet。`verify_backend.py` 核对静态 schema、shortcut 声明和 dry-run；使用占位凭据与临时目录。根测试中，实际 wrapper 接模拟飞书上游；若未构建 wrapper，相应测试会明确 skip。

Linux 构建可使用：

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o bin/feishu-mcp-gateway-linux-amd64 ./cmd/gateway
python3 scripts/build_backend.py /path/to/lark-cli --goos linux --goarch amd64 --output bin/lark-cli-gateway-linux-amd64
```

升级 lark-cli 或调整转换规则后，重新生成目录、覆盖台账与指南，再渲染 Skill：

```sh
python3 scripts/refresh_catalog.py /path/to/lark-cli
go run ./cmd/gateway --render-skill > skills/feishu-gateway/SKILL.md
```

核对步骤见 [Agent 使用层](docs/AGENT-USAGE.md#升级-lark-cli)。维护者可以从干净提交生成完整发布包：

```sh
python3 scripts/build_release.py /path/to/lark-cli --tag v0.3.0-dev --output local/releases/v0.3.0-dev
```

对应源码包中的 `rebuild.py` 使用随包 vendor 依赖构建，关闭网络模块解析；需要预先安装 Go 1.26 或更新版本。

## 配置与管理

复制 [示例配置](examples/config.json) 到 `local/config.json`。相对 `data_dir` 相对于配置文件目录解析。`cli_binary` 填受控 wrapper 的绝对路径，`cli_sha256` 填其 SHA-256。普通 lark-cli 二进制无法通过受控后端契约检查。

`catalog_revision` 固定实际工具目录的摘要。升级目录时，先核对覆盖台账和已有 packs，再更新此值，以免能力包静默扩张。0.3.0-dev 清理了目录中的描述文字，并修正了 18 个 sheets 参数的校验，升级时需要更新此值。`--version` 同时输出版本、目录摘要和指南摘要：

```sh
./bin/feishu-mcp-gateway --version
./bin/feishu-mcp-gateway --catalog-revision
mkdir -p local
umask 077
./bin/feishu-mcp-gateway --generate-key > local/access-key.txt
./bin/feishu-mcp-gateway --config local/config.json --check
./bin/feishu-mcp-gateway --config local/config.json
```

`secret_env` 指向启动环境中的 App Secret。每个接入绑定一个 `app_connection_id`，只存接入 key 的 SHA-256。实际 key 配置到 MCP 客户端的 `Authorization: Bearer …` Header。

默认地址为 `http://127.0.0.1:18790/mcp`。远程访问使用 HTTPS 反向代理，并显式配置 `trusted_https_proxy`；这不会自动开启 TLS，Gateway 端口应处于受控网络内。

应用和接入策略可通过 SIGHUP 热加载，也可配置独立的 `admin_key_sha256` 后调用：

- `GET /admin/status`：版本、目录/指南/策略摘要、各接入的工具视图与开放范围、运行数、取票缓存统计。
- `POST /admin/reload`：重新读取启动配置文件；通过全部校验后替换策略。

热加载后让客户端重新发现工具，以取得新视图。管理凭据与 MCP 接入凭据分开。监听、存储、后端、全局容量等运行参数改变需要重启。已有应用连接 ID 不能改绑到其他 AppID、企业或区域；新绑定使用新的连接 ID。单个数据目录只允许一个服务进程。

## 权限与 Wake

`packs` 和 `operations` 的并集决定接入开放的操作；actor 的非空 `operations` 进一步收窄。高风险操作及需要显式确认的 shortcut，还必须出现在管理员配置的 `high_risk_operations` 中。工具参数不提供 `approved`、`as`、Token 或执行应用选择。

`resource_mode` 必须明确配置：

- `resource_scoped`：对目录已登记的资源绑定检查接入和 actor 的交集；CLI 的实际子请求仍核对资源路径。当前包含部分 Wiki、聊天、Drive folder、文档读取、Base 记录及 Sheets 操作，具体以 `resource_bindings` 为准。
- `application_scope`：明确委托应用可访问的资源范围。无法证明资源范围的 shortcut 使用此模式；actor 仍受操作限制。

资源 ID 列表不自动包含子资源，也不推断提问者的个人飞书 ACL。`application_scope` 下仍配置 actor resources 时，只能执行支持这些资源检查的操作。

`identity_mode=client` 以接入身份运行；`wake_actor` 在实际操作时要求有效的提问者 Header。发现可以没有 actor，实际调用、文件、Resources 和 Job 访问不能缺失。请求带有提问者时，搜索、describe、指南总览和 `feishu_context` 只展示该提问者能执行的操作。

按照 [Wake 官方说明](https://docs.qoder.cn/qoderwake/skills-and-integrations)，使用 Streamable HTTP、开启 IM 提问者身份传递、Headers Helper 留空。服务读取：

```text
X-QoderWake-AtWaker-Sender-Id
X-QoderWake-AtWaker-Sender-Channel
```

飞书 actor 表示为 `feishu:user_id:<user_id>`。Name、Open-Id、Union-Id 不参与身份归并；模型参数和 `x-mcp-header` 不能替代可信来源身份。Header 本身没有签名，接入 key 用于认证受信任的 Wake 来源。

多个 Waker 需要不同工具权限时配置不同接入 key。Gateway 不依赖未被官方承诺的 Waker/chat/session/task 参数。已进入 Wake 共享对话、记忆或群消息的内容，由客户端管理其可见范围。

## 工具与结果

静态 API 使用 `params`、`data` 和可选 `files`；shortcut 使用逻辑参数，例如 `doc`、`base_token`、`spreadsheet_token`。支持的 JSON payload 参数可直接传对象/数组，包含已公开的细粒度 schema。完整参数以具名工具或 `feishu_tools_describe` 为准。复杂操作可给 describe 传 parameter，只展开需要的顶层参数 schema。

所有接入都提供 `feishu_guide`：无参数返回当前请求可见的领域、操作数量和入口操作，`domain` 返回领域指南，`topic` 返回指南引用的主题。MCP `instructions` 按接入列出开放领域和调用约定。紧凑视图中，`feishu_tools_search` 按任务说法查找操作（中英文均可，可按 `domain`、`risk` 过滤），`feishu_tools_describe` 同时返回该操作的指南；已知 operation_id 和参数时可直接执行。具名工具的描述附带适用与不适用场景。

写入必须提供 `idempotency_key`。同键同参返回原回执，同键异参返回冲突。幂等作用域包含应用连接、接入、actor 与 operation_id。

`async=true` 持久化登记后返回 job_id，通过 `feishu_job_get` 查询；同步调用的 invocation_id 也可用于回执查询。`feishu_job_cancel` 停止剩余工作。重启后 queued 继续领取，running 转为 unknown。外部写入已开始而响应丢失、或多步写入已有成功步骤又中断时，需要核对外部结果。部分成功保留逐项回执。

`status=succeeded` 表示本次操作调用完成。上游导出等 API 自身返回的 processing、ticket 或 task_id 保留在 data 中，需要按上游回执继续查询。MCP 调用结果与飞书异步操作状态分别表达。

应用权限错误保留数字错误码、log_id、缺失 scopes 和 Retry-After。分页句柄绑定主体、操作和查询；既支持静态 params.page_token，也支持 shortcuts 的 page_token/cursor。offset 分页使用上游 next_offset。

## 文件与过程产物

小文本通过 `feishu_file_create` 保存。文件输入使用 `{"file_id":"f_…"}`；通用 API 形如 `files.image`，shortcut 形如 `content`。`workspace_files` 是相对名称到 file_id 的映射，在本次调用的 `inputs/` 下提供，用于 Markdown/XML 中的相对资源引用。

每次调用独立工作区，输入只从当前主体的 File Store 挂载。输出使用相对名称；超限回执（包括 data、meta 和文件清单）保存为 result.json。产物归档后返回 files 与 `feishu-file://files/…` resource_link。未改变的输入不会重复归档，部分失败保留已经产生的文件。工作区退出后清理。

- `feishu_file_list/stat/read/delete` 管理当前主体的文件；read 每次最多 64 KiB，返回 next_offset 和 eof。
- `resources/read` 适合小文件；每次检查当前主体和产物来源操作的权限。
- `POST /files?name=example.bin` 上传，`GET /files/{file_id}` 下载；携带与 MCP 相同的接入认证和 Sender Header。
- `allow_transfer_tickets=true` 开启 `feishu_file_transfer`：签发 60 秒有效的单次上传/下载票据，传输时再次检查原主体的当前权限。

本地文件适配器从 stdin 读取工具返回的票据 JSON：

```sh
python3 scripts/transfer_file.py --endpoint https://gateway.example --file ./report.pdf < transfer.local.json
```

上传返回 file_id；下载使用新目标文件名，完成后原子落盘。票据只授权一次文件传输，但持有者可以使用它，因此只交给本次传输适配器。该方案可用于不能为文件传输保留 Sender Header 的宿主。票据不自动把产物发送到 Wake 聊天；目标客户端的本地路径和展示流程需要按其版本验收。

默认单文件 16 MiB、每次工作区 64 MiB、每接入 256 MiB，文件保留 24 小时。每次挂载和归档最多 256 个文件。Job 对输入文件持有引用；到期清理跳过排队和运行中的引用，主动删除会返回 file_in_use。文件过期不删除幂等回执。

## 限额和事件

`workers` 控制总执行并发；应用、接入和 actor 可分别配置 `max_concurrent`、`max_requests_per_minute`。后者为固定分钟窗口，0 表示不另设该层调用次数限制。应用的 `outbound_requests_per_second` 默认 5，控制实际出站节奏，并遵守上游 Retry-After。默认每接入最多 100 个排队/运行中的后台操作。

事件接入需在应用配置中设置 `event_verify_token_env`、`event_encrypt_key_env` 和明确的 `event_types`，然后由管理员在飞书配置加密 HTTP 回调到 `/events/<app_connection_id>`。该配置不由模型修改。

回调核对应用、加密帧、签名、验证 Token 与时间窗口，只保存配置的事件类型。读取还要求 `events.read` 操作权限、接入 `event_types` 和（启用时）actor `event_types`。不提供依赖互动应答的卡片处理。每个应用保留最多 24 小时、10000 条、64 MiB；读取发现已过期的分页范围会返回缺口错误。

`feishu_events_read` 是有界读取，最多 100 条、等待最多 30 秒，也可登记为 Job。它替代旧 `event +subscribe` 的 Agent 使用入口；无需为每次 MCP 调用启动长期 CLI daemon。

## 可选 Skill 与运行边界

[feishu-gateway Skill](skills/feishu-gateway/SKILL.md) 提供领域路由表、调用约定、写入回执、文件复用、分页和错误处理方法，可以按宿主的 Skill 安装方式使用。`python3 scripts/package_skill.py local/skill --zip` 生成带离线指南的 Skill 目录与 ZIP，可按 Wake 的 Skill 压缩包方式上传；发布包中的 Skill 已包含这些指南。工具自身包含 schema 和必要说明，权限全部由 Gateway 检查。

Gateway 与固定 wrapper 属于受信任服务。文件访问使用 rooted FileIO，外部 URL 请求经代理检查，真实应用凭据不进入 wrapper。此结构不构成对同一 OS 用户的进程隔离；有隔离要求时，使用独立服务身份和受控数据目录。

当前认证是预配 key 的 MCP 接入配置，没有实现 OAuth 授权发现；标准 Tasks 扩展与旧式 SSE endpoint 未启用。实际验证及外部验收边界集中记录在 [VALIDATION.md](docs/VALIDATION.md)。
