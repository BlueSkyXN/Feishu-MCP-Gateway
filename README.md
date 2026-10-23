# feishu-mcp-gateway

通用飞书 MCP Server，提供应用身份操作、受控文件和后台执行。飞书执行身份固定为 bot / TAT，支持多个应用连接与客户端；Wake 可附带可信提问者身份进一步限制权限。

当前为 `0.1.0-dev`。使用官方 MCP Go SDK，支持已测试的 `2026-07-28` 与 `2025-11-25` Streamable HTTP。认证使用管理员预配 Bearer key；未实现交互式 OAuth 授权发现。

## 当前能力

- 按能力包和明确操作清单生成具名 MCP tools，输入参数使用 JSON Schema 校验。
- 逐请求建立客户端和可选 actor 身份；每个接入绑定一个飞书应用。
- 指定资源模式与应用资源范围模式；指定资源模式只开放能够执行相应检查的操作。
- CLI 作为受信任服务端子进程，固定二进制 SHA-256，独立环境和临时目录，始终 `--as bot` + bot strict mode。
- 写入幂等、SQLite 回执、后台 Job、取消和重启恢复；不确定写入保留 `unknown`。
- 文件 URI、MCP Resources、有界读取、认证上传下载、容量和到期清理。
- 分页句柄绑定当前主体、操作和查询条件。

静态目录来源为 lark-cli `7f70de0666a21ff77ab882c62b61aff44ae3734d`。251 个方法中，177 个具有当前 JSON 执行适配；45 个 user-only 方法排除，25 个应用身份高风险写操作和 4 个未适配输入类型暂未开放。逐项见 [覆盖台账](docs/coverage.json)。

目录中的 `adapter_ready` 表示已有参数和执行映射。真实应用权限与飞书返回仍须在目标环境验证。文档、Base 等部分能力位于 lark-cli shortcuts，尚未纳入这个静态目录，当前版本没有完整替代全部应用身份 shortcuts。

## 构建和检查

需要 Go 1.26 或更新版本。MCP SDK 要求 Go 1.25；当前 SQLite 驱动要求 Go 1.26。

```sh
go test ./...
go test -race ./...
go vet ./...
go build -o bin/feishu-mcp-gateway ./cmd/gateway
```

CLI 使用原生二进制路径，不使用会自动下载或更新的 npm 启动包装器。先核对实际版本和 SHA-256，再验证目录映射：

```sh
python3 scripts/verify_backend.py /absolute/path/to/native/lark-cli
```

该脚本使用临时 HOME、占位应用凭据，仅调用 `schema` 和 `--dry-run`，不会执行真实飞书业务写入。验证结果默认保存在忽略的 `local/evidence/`。

## 配置

将 [示例配置](examples/config.json) 复制到 `local/config.json`，替换占位值。相对 `data_dir` 相对于配置文件目录解析。

接入 key 可以使用以下命令生成并保存到本地受控文件；配置中只填写 `key_sha256`，原始 key 配置到 MCP 客户端。

```sh
mkdir -p local
umask 077
./bin/feishu-mcp-gateway --generate-key > local/access-key.txt
```

`secret_env` 指向启动环境中的 App Secret。实际 App Secret 不写入示例配置，不复制个人 CLI profile 或 UAT。客户端工具参数不能改变应用连接或执行身份。

```sh
./bin/feishu-mcp-gateway --config local/config.json --check
./bin/feishu-mcp-gateway --config local/config.json
```

MCP 地址为 `http://127.0.0.1:18790/mcp`，客户端设置 `Authorization: Bearer <接入 key>`。需要远程访问时，由受控 HTTPS 反向代理提供入口，并显式配置 `trusted_https_proxy`；该设置要求后端端口处于受控网络内，不会自动启用 TLS。

配置修改通过受控重启生效。停用接入或应用后，新请求及恢复执行的 Job 会按新配置检查权限。单个数据目录只允许一个 Gateway 进程运行。

## 权限和 Wake

`identity_mode=client` 以接入身份执行；`wake_actor` 在实际操作时要求有效的 Wake Sender Header。工具发现可以不含 Sender。两种模式不会因某次请求缺失 Header 自动切换。

Wake 使用 Streamable HTTP，开启 IM 提问者身份传递，Headers Helper 留空。服务读取 Sender-Id 和 Sender-Channel，将飞书用户表示为 `feishu:user_id:<id>`。接入 key 认证 Wake 来源；Header 本身不是签名凭据。

`packs` 如 `wiki.read` 选择工具集合，`operations` 可以额外列出具体 operation ID。两者是并集。`resource_mode` 必须明确选择：

- `resource_scoped`：检查接入与 actor 的资源集合交集；当前支持 Wiki space、指定聊天、指定 Drive folder 的已登记操作。
- `application_scope`：管理员明确委托应用可访问的资源范围；仍受操作集合和可选 actor 限制。

actor 的 `operations` 非空时进一步收窄操作；其 `resources` 非空时进一步收窄资源。资源清单只做明确 ID 检查，不推断个人飞书 ACL，也不自动递归授予目录子对象权限。

多个 Waker 共用接入 key 时共用接入级权限。需要不同权限时配置不同接入。当前版本没有 Waker、Session、群上下文或业务任务实体。

## 工具、文件和 Job

具名操作使用如下参数结构；实际必填字段以客户端获取的 schema 为准：

```json
{
  "params": {"space_id": "已授权的空间 ID"},
  "async": false
}
```

写操作要求 `idempotency_key`。相同键和参数返回原回执，异参复用返回冲突。`async=true` 在持久化后返回 `job_id`，通过 `feishu_job_get` 查询。`feishu_job_cancel` 只请求停止剩余工作。CLI 调用超时或响应丢失时，写操作可能进入 `unknown`，必须核对后再决定如何处理。

进程重启后，尚未执行的队列继续处理；原 `running` 状态转为 `unknown`，不自动重放。幂等记录不随临时文件到期删除。

`feishu_file_create` 保存小文本；`feishu_file_stat/read/delete` 管理当前主体的文件。操作结果超出内联上限时返回 file_id 和 `feishu-file://files/...` resource_link。产物仍受来源操作的当前权限约束。

文件数据接口：

- `POST /files?name=example.bin`：请求体为文件字节。
- `GET /files/{file_id}`：下载已授权文件，返回 attachment。

数据接口需要与 MCP 请求相同的认证和 Sender Header，不接受 query-string key。客户端本地文件需由宿主上传；远程 MCP Server 不解析客户端本地路径。Wake 的自动上传、下载落地及各类请求身份透传仍须在实际客户端验收。

默认每文件 16 MiB、每接入 256 MiB、保留 24 小时；可通过配置调整。MCP 单次文件读取最多 64 KiB，Resources 适用于小文件，较大文件使用分段读取或数据接口。

默认每个接入最多保留 100 个排队或运行中的后台操作，可配置 `max_pending_jobs_per_client`。同一接入下的不同 actor 共用容量和队列限额。CLI 部分成功回执返回 `partial` 并保留逐项结果，不能按整个批次重新发送。

## 实现边界

首个执行后端把 CLI 子进程纳入受信任服务边界，App Secret 只存在于服务启动环境和该次子进程环境，TAT/DPoP 取票由 CLI 自身处理。尚未实现设计中的常驻取票代理，因此并发执行会有重复取票及子进程开销。需要防止同机 Agent 读取服务凭据时，使用独立 OS 服务身份。

目前不提供高风险写入、通用任意 HTTP/Shell、自动用户授权、CLI 文件参数适配、标准 Tasks 扩展、紧凑工具视图或业务流程编排。工具 annotations 对可能创建 Job/文件的操作保守标注，授权始终在服务端执行。

已验证范围与剩余验收见 [开发验收记录](docs/VALIDATION.md)。
