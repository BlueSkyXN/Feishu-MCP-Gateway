# 0.1.0-dev 验收记录

开发环境：macOS arm64，Go 1.27.2。模块最低版本 Go 1.26.0。MCP SDK v1.8.0，SQLite 驱动 v1.61.0。

当前检查包括：

- 使用官方 SDK 客户端分别以 2026-07-28、2025-11-25 连接本地 HTTP 服务，发现并调用工具。
- 同一连接内 A→B→A 切换，缺失 actor、越权资源、工具参数注入和未知身份配置拒绝。
- SQLite 写入幂等、并发同键、异参冲突、重启去重、不确定写入不重放。
- 文件的 Tools、Resources、HTTP 访问隔离，文件到期、路径名称拒绝及来源权限撤销。
- 后台 Job 查询、跨主体拒绝、取消、排队恢复及运行中任务转 unknown。
- CLI 子进程参数映射、宿主用户 Token/代理/其他秘密环境变量不继承，二进制变化拒绝。

实际 CLI 核对使用本机已安装的原生 lark-cli 1.0.98：177 个目录操作的 schema 字段和 bot 身份元数据均匹配；Wiki space get、Drive files list、Drive create_folder 的 dry-run 均成功。全部使用占位应用凭据及临时 HOME，没有真实飞书业务请求。

运行验收结果保存在忽略的 local/evidence/。开发测试通过不能推定目标应用权限已开通或 Wake 文件传输已验收。

已完成 `go test -race ./...` 与 `go vet ./...`。另以独立 Gateway 进程验证配置检查、健康检查、MCP 初始化和工具发现、应用身份上下文、文件创建、Resources 读取、认证下载、未认证拒绝和正常退出；此过程没有调用飞书业务工具。

仍需在后续目标环境验证：

- 真实应用 TAT/DPoP、应用 scope、资源 ACL、限流及实际返回数据。
- Wake 版本、连接器配置、Sender 在 Resources/Job/文件接口上的传递和本地文件落地。
- 官方 conformance 完整测试集、Linux 原生运行及运维部署。
- 全部 shortcuts、CLI 文件传输操作、高风险写入和常驻取票代理。

当前没有部署、发布或真实飞书写入。
