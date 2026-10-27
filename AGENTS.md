# Feishu-MCP-Gateway

通用飞书 MCP Server。对外能力由 Operation 目录定义；始终使用应用身份，禁止用户 OAuth、UAT 和个人 profile 回退。Wake 身份只是可选可信来源适配。

- 对每次调用、文件、Job 和游标访问检查当前客户端及 actor；MCP session、模型参数和资源 ID 不构成授权。
- 工具集合按接入配置固定，不能在请求中修改共享 Server 的工具注册。
- schema、风险、执行映射和权限检查使用同一操作定义。未支持的能力明确关闭。
- 目录、覆盖台账与 internal/gateway/guidance/ 由 scripts/refresh_catalog.py 从固定 lark-cli 生成：改转换脚本或领域表后重新生成，不手改生成文件。skills/feishu-gateway/SKILL.md 用 --render-skill 生成。
- 发现入口（搜索、describe、指南、context）与执行授权使用同一可见性判断。
- 使用 Go 标准库及已锁定依赖；配置示例只包含占位值。秘密和开发材料放入忽略的 local/。
- 默认测试使用本地模拟执行器，不调用真实飞书，不读取开发者个人配置。
- 验证：go test ./...、go test -race ./...、go vet ./...、go build ./cmd/gateway。
- 推送前运行 gitleaks git --config .gitleaks.toml --redact；提交、PR 说明和文档中不写本机路径与个人信息。
- 发布、部署、真实飞书写入另需明确授权。
