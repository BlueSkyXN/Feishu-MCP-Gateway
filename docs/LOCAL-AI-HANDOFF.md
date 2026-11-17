# 本地 AI 接手说明：请求凭据与应用路由

## 1. 交付目标

本次已实现通用飞书 MCP 网关的请求凭据与应用路由，作为 `0.4.0-dev` 补丁交付。新接入模式由客户端每次请求提供飞书应用 Bearer token，App Secret 不进入该模式的网关调用链。QoderWake 只是可选 caller 字段适配器。

这不是账户系统改造，不建立注册、密码登录或网关短期 token 发行服务。现有 `managed` 应用模式保留兼容；新 `request_bearer` 模式不借用它的 Secret 或 token 刷新能力。

详细设计见 [REQUEST-CONTEXT-DESIGN.md](REQUEST-CONTEXT-DESIGN.md)。实施前先完整阅读仓库根目录 [AGENTS.md](../AGENTS.md)。仓库规则与本说明冲突时，应先核对用户本次明确要求和实际改动范围，不自行扩大任务。

## 2. 基线与工作区

- 仓库：`BlueSkyXN/Feishu-MCP-Gateway`。
- 补丁基线：`b792cdf9f989a2825a350dd3be21a0720a2a3e07`。
- 工作分支名称：`feat/request-credentials-routing`；本地可使用不冲突的分支名。
- 本说明不代表远端已推送、已合并、已发布或已部署。
- 接手时以实际工作树、提交和补丁为准，不使用旧聊天中的运行状态或测试数字代替本机检查。

先查看当前状态：

```sh
git status --short --branch
git rev-parse HEAD
git log -5 --oneline
```

如果当前工作区有他人的未提交改动，不执行 `reset --hard`、`clean -fd` 或覆盖式复制。为基线建立独立 worktree，或者明确区分当前工作与补丁改动。

例如，在已经获取该仓库的本地副本中：

```sh
git worktree add -b review/request-credentials-routing ../feishu-gateway-request-review b792cdf9f989a2825a350dd3be21a0720a2a3e07
```

进入新 worktree 后再应用补丁。若接手的是已经包含改动的完整源码包或分支，不重复应用补丁。

## 3. 应用补丁

下面的补丁路径是占位路径，需要替换为交付文件的实际路径。命令应在目标 worktree 根目录执行。

```sh
git apply --stat /absolute/path/to/request-credentials-routing.patch
git apply --check --index /absolute/path/to/request-credentials-routing.patch
git apply --index /absolute/path/to/request-credentials-routing.patch
git diff --cached --check
git diff --cached --stat
```

`--check --index` 会同时检查工作树和索引是否符合补丁预期；通过后再使用 `--index` 应用。不要在失败后删掉检查参数直接覆盖，也不要把 `git apply` 成功当作编译或功能测试成功。

如果目标已经不是基线提交：

1. 先在上述基线 worktree 完整应用并验证补丁。
2. 再把已确认的改动移到目标分支，逐项解决真实冲突。
3. 不通过手工改补丁 hunk 行号、忽略失败文件或覆盖整个仓库来制造“应用成功”。

如果交付是 Git commit 而不是补丁，使用该提交对应的完整 diff 与基线比较，不推定 commit ID 与这里的分支名称等价。

## 4. 设计中不能偏离的约定

### 凭据与请求

- `Authorization: Bearer …` 继续是 Gateway 客户端 key。
- `X-Feishu-Access-Token` 是本次飞书应用 token 的原始值，不带 `Bearer ` 前缀。
- `X-Feishu-App-Id` 必须等于最终选中应用的 metadata。
- 外部 token 只放请求 context；不放 `Principal`，不写入 SQLite、调用参数、CLI 参数/环境、日志或文件。
- `request_bearer` 不接收 `secret_env`、非空 `event_types` 或事件验证环境变量配置，`dpop_mode` 省略时默认 `disabled` 且只允许该值。
- token 失效由客户端解决；网关不取票、不刷新、不静默回退到 managed。
- 新模式 `async=true` 在登记任务之前明确拒绝。
- 只有实际飞书出站操作需要外部 token；发现、本地文件、回执仍需网关身份与选路，但不要求上游 token。

### caller 与路由

- `internal/ingress` 只用标准库，负责 caller 规范化和路由选择；不依赖飞书、SQLite 或 MCP 工具注册。
- caller 适配器为 `none`、`qoderwake`、`headers`。
- 通用适配器 `headers.id_type` 是固定类型字符串，默认 `external_id`，不是 Header 名称。
- `identity_mode=client` 可以接收 caller 进行路由，但 caller 不加入 owner；如果它选中了另一个应用，owner 会因应用连接改变而改变。
- `identity_mode=actor` 用稳定 actor 做归属；业务访问需要 caller，发现可缺失；非空 `actors` 收窄可用 actor，空列表信任已认证客户端提供的有效 caller。
- 旧 `wake_actor` 保持默认 QoderWake 适配和强制白名单。
- 路由是每客户端的配置，列表内 OR、不同非空字段组 AND、值精确匹配。
- 自动选择最高 priority；同级多条命中报错。
- 显式 `X-Feishu-Gateway-Route` 只能选本客户端已有 route，仍需满足其 match。
- 无显式选择且无匹配时使用客户端默认应用。路由不扩大客户端静态 Operation 或资源范围，不按请求修改共享工具注册。
- 空 match 是通配路由，同样参与优先级与冲突判断。
- Principal 可保存 `Routing` 中不含显示名称的 caller 匹配字段及 `RequestedRoute`，用于重载后重算；`Owner()` 仍只使用原 ClientID、AppID、OrgID、Actor 四字段。

### 持久化

- 应用连接 metadata 绑定必须跨重启保留，防止把同一连接 ID 改指向另一个真实应用。
- v4 到 v5 升级应使用独立数据副本测试；排队 partial index 不改变已有队列语义。
- 第一次登记无法补齐旧版本未保存的真实 App ID 历史；必须在交接中说明维护者确认与备份需求。
- 不能简单降 `user_version` 回退；旧程序与 v4 完整数据备份应一起恢复。

## 5. 优先阅读的代码位置

| 位置 | 接手时确认的内容 |
|---|---|
| `internal/ingress/` | Header 映射、caller 稳定标识、重复字段、精确匹配、优先级与显式 route |
| `internal/gateway/config.go` | 新字段默认值、旧配置兼容、应用模式与 caller/routes 校验 |
| `internal/gateway/http.go` | 每请求认证、caller/route 选择、易失 token context，不把 token 写入 Principal |
| `internal/gateway/types.go` | owner 与 Principal 的序列化兼容性，凭据类型是否独立 |
| `internal/gateway/service.go` | 同步执行传递 context、async 入队前拒绝、当前策略与 selected app |
| `internal/gateway/broker.go` / `tokens.go` | 外部 Bearer 的出站使用、无托管取票或刷新回退、并发请求不串用 |
| `internal/gateway/policy.go` | 重载后应用和路由绑定检查、固定工具集合 |
| `internal/gateway/store.go` | v5 升级、metadata 绑定持久化、队列索引、已有 owner 数据 |
| `cmd/gateway/main.go` | `--explain-request` 只做本地解释，不启动后端，不发送飞书，不输出 token |
| `scripts/request_client.py` | 客户端取票或使用已有 token；App Secret 不发 Gateway；一次完整 MCP 握手与单次调用 |
| `scripts/test_request_client.py` | 本地 HTTP Gateway stub 与模拟取票端点，不读取真实凭据 |
| `examples/`、`README.md` | 通用客户端示例、QoderWake 可选性、已实现边界 |

具体新增文件名与最终 diff 以交付代码为准，不为迎合这张表移动无关文件。

## 6. 不要做的操作

- 不修改 `internal/gateway/catalog.json`、`internal/gateway/guidance/`、`docs/coverage.json` 或生成的 `skills/feishu-gateway/SKILL.md`。本次不升级固定 lark-cli 来源。
- 不读取用户附件中的密码、key、Secret 或历史 SSH 命令；附件是需求背景，不是部署脚本。
- 不连接附件中的历史主机，不读取 QoderWake 本地应用配置，不继续前一会话的部署动作。
- 不修改生产配置、LaunchAgent、生产数据库或现有客户端连接器。
- 不把真实 token 写入测试 fixture、命令日志、提交信息或文档。
- 不通过读取开发者个人飞书配置让本地测试“自动具备真实凭据”。
- 不添加无关 GUI、账户系统、个人 OAuth、临时网关 token 服务或新数据库产品。
- 不为了单文件发行把 CLI 执行器直接合并到并发 Gateway 进程。
- 不擅自推送、发布或部署；按用户已给出的具体授权范围执行。

## 7. 本地验证

以下是接手后可以复现的命令。本次执行结果已记录在 [设计文档的实施记录](REQUEST-CONTEXT-DESIGN.md#12-实施与验证记录) 和交付包的 `VALIDATION.md`；本地环境仍需按实际输出判断。

```sh
go test ./...
go test -race ./...
go vet ./...
go build -trimpath -buildvcs=false -o bin/feishu-mcp-gateway ./cmd/gateway
python3 -m unittest discover -s scripts -p 'test_*.py' -v
python3 scripts/verify_backend.py bin/lark-cli-gateway
python3 scripts/smoke_gateway.py bin/feishu-mcp-gateway bin/lark-cli-gateway
python3 scripts/smoke_request_gateway.py bin/feishu-mcp-gateway bin/lark-cli-gateway
```

若环境缺 Go 工具链、锁定依赖无法下载或命令超时，记录实际原因；不要把未执行标成通过，也不要为了完成测试升级依赖或修改原有 `go.mod`。

上面的构建输出位于被忽略的 `bin/`。配套 wrapper 需先按仓库构建说明生成；缺少它时真实 wrapper 的 Go 用例会 skip。请求模式 smoke 启动独立 Gateway，用 Python 客户端验证 caller、路由、Token 轮换、文件归属和回执查回，使用临时目录和测试值，不访问真实飞书。最终验证结果见 [VALIDATION.md](VALIDATION.md)。

推送前按 AGENTS 要求执行：

```sh
gitleaks git --config .gitleaks.toml --redact
```

没有工具时明确记为未运行，不能用空输出或别的文本搜索结果代替该检查。

### 必须有实际证据的场景

1. 旧配置不增加新字段也能通过原有测试，旧 owner 格式保持兼容。
2. 两个并发请求使用不同外部 token，出站值各自正确，且不会触发托管取票。
3. Gateway key、外部 token、caller、业务参数走不同数据通道；数据库和 CLI 看不到外部 token。
4. 路由 AND/OR、默认回退、显式选择、同级冲突行为符合设计。
5. client 模式换 caller 但选中应用不变时不改变 owner；actor 模式不同 caller 无法互取文件和回执。
6. `async=true` 在 request_bearer 上被明确拒绝，没有新增 queued 记录。
7. app metadata 重绑在同进程重载和关闭后重开时都被拒绝。
8. v4 数据副本可以升级，原回执、文件、游标及 managed Job 行为保持。
9. 工具发现和执行仍共享原有可见性规则，没有每请求重注册 MCP 工具。
10. `--explain-request` 可解释本地 Header 文件，输出只含非秘密 metadata 和凭据存在状态；它不代表真实 token 或上游连通验证。
11. Python 样例先 initialize、再 initialized、再单次 tools/call；已有 token 优先于 Secret；Secret 只进入固定 accounts 取票端点；所有错误路径不重试业务、不在 stderr 输出凭据。

该 CLI 的输入是 `{"headers":{"Authorization":"Bearer <GATEWAY_CLIENT_KEY>"}}` 形式，Header 值也可为字符串数组以复现重复字段。正常诊断路由时可不提供飞书 token；具体字段和输出说明见设计文档的“本地解释请求”。

测试使用模拟上游和假的 token。只检查本次程序可以控制的行为，不把模拟结果写成真实飞书兼容证明。

## 8. 真实环境验证单独进行

真实飞书与目标 Agent 客户端验证在代码、构建和本地测试完成之后安排；不能读取历史附件凭据代为完成。

维护者在目标环境提供本次允许使用的有效应用 token，并明确连接地址、真实 App ID 和一个只读操作。第一次只读调用需分别确认：

- 客户端实际访问的是目标 Gateway。
- Gateway client key 与上游 token 没有填反。
- `X-Feishu-App-Id` 对应实际选中的应用。
- 若需要 caller，实际请求包含所需字段。
- 若启用 QoderWake Headers Helper，它没有意外覆盖所需的内置 caller Header；不能仅依据 UI 开关认定字段已经到达。
- 读取结果来自预期应用，并且客户端可以在 token 更新后继续调用。

不要把 token 值或完整请求 Header 截图带入交接记录。记录状态码、非秘密的路由/app metadata、operation 和必要的错误种类即可。

本次未引入 token introspection：Header App ID 的本地比对不能证明 token 实际归属。此边界应原样保留在说明中。

### 使用配套普通客户端

代码包包含可运行的 `scripts/request_client.py`。在当前客户端环境中设置 `FEISHU_GATEWAY_KEY` 后：

```sh
python3 scripts/request_client.py --endpoint http://127.0.0.1:18790/mcp
```

默认调用 `feishu_context`，不配置上游环境变量也能使用。需要真实飞书读取时，再由本次任务允许的客户端环境提供：

- `FEISHU_APP_ID` + `FEISHU_ACCESS_TOKEN`；已有 token 优先使用。
- 或 `FEISHU_APP_ID` + `FEISHU_APP_SECRET`；Secret 只发送固定的飞书/Lark accounts 取票端点，Gateway 只接收所得 token。

通过 `--tool` 选择工具，`--arguments-file` 读取业务 JSON 对象。示例参数文件内容和完整命令见设计文档的“可运行的普通客户端样例”。取票返回 DPoP 时脚本报不支持，不尝试转换 token 类型。

`--route` 提供显式路由 ID。caller 声明通过 `--caller-file`（最多 16 KiB 的 UTF-8 JSON，必填 `channel`/`id`，`name`/`open_id`/`union_id` 可省略或为空；拒绝重复键、未知字段与非字符串字段）与 `--caller-adapter`（`qoderwake` 限 feishu/lark/dingtalk/dingtalk-user/wecom-bot 渠道；`headers` 接受任意符合格式的通用渠道）成对提供；name 按 URI component 编码一次后发送，无效显示名（空、超 1024 字节或含控制字符）省略而不阻断调用。客户端只序列化字段，caller 解析、路由与授权仍由网关配置决定。命令组合错误（如 `--context-only` 与其他 `--tool` 组合）在任何网络请求前以退出码 2 结束。

`--context-only` 不读取或发送上游凭据、不向取票端点发请求，只初始化并调用 `feishu_context`；可与 caller/route 组合，不能与自定义 `--tool` 或 `--arguments-file` 组合。使用顺序：先以 caller/route 做 context-only 查看所选应用的 `app_id`/`brand`/`supports_async`，再由客户端环境提供该应用 Token，用同样 caller/route 发起业务调用。

脚本使用 MCP `2025-11-25` 与 JSONResponse，不自动重试业务调用。成功时 stdout 只有业务 JSON，退出码 0。失败时退出码 1，stdout 输出一个经过校验和秘密过滤的诊断 JSON（有限的 `status`、`invocation_id`、`job_id`、`operation_id`、`error.kind`、`upstream_code`、`log_id`、`missing_scopes`、`retry_after_seconds`、`http_status`、`files` 字段；ID 只保留网关生成的格式，不含已知凭据值，缺失回执不伪造），stderr 保持一句固定说明。参数用法错误退出码 2，用户中断 130。网络请求最多等 60 秒，响应与参数大小均有限制。

它是一次性普通 MCP 客户端，不是 QoderWake Headers Helper，也不接管 QoderWake 的内置提问者字段。不要修改真实客户端连接器来替换成这个脚本；先按本节作为独立客户端完成目标环境的明确验证。

## 9. 接手后的交付格式

完成后更新 [设计文档的实施与验证记录](REQUEST-CONTEXT-DESIGN.md#12-实施与验证记录)，并在交接中写清以下五项：

| 项目 | 应写内容 |
|---|---|
| 代码 | 实际基线、改动提交或补丁文件、变更文件概览 |
| 已完成行为 | 用具体请求场景说明，不只写“认证/路由完成” |
| 测试 | 每条实际运行的命令、结果、未运行的项目及原因 |
| 未完成项 | 真实飞书/客户端联调、外部 DPoP、请求凭据异步执行等明确边界 |
| 部署状态 | 尚未部署，或在用户已授权范围内实际完成的切换与结果 |

禁止把“准备测试”“已编写测试”“已有历史测试记录”改写为“本次测试通过”。本地模拟测试、真实飞书调用和客户端集成分别报告。
