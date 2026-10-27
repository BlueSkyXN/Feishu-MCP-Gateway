# 检查与发布

## PR 检查

`.github/workflows/ci.yml` 在每个 PR 和 main 的推送上运行，环境为 ubuntu（linux/amd64）：

1. 从 larksuite/cli 取目录记录的固定提交，构建受控后端，运行其 race 测试与 vet。
2. 使用最新稳定版 Go 运行 `go vet`、`go test`、`go test -race`，并构建 Gateway；实际 wrapper 的集成测试一起运行。
3. `verify_backend.py` 核对后端声明，`smoke_gateway.py` 启动独立进程检查，`test_verify_release.py` 用构造的发布目录检查发布核对脚本。
4. 交叉构建 darwin/arm64。
5. `scripts/refresh_catalog.py` 按目录记录的 lark-cli 提交重新生成目录、覆盖台账与指南，结果须与提交内容一致；不一致时在本地运行该脚本并提交结果。

另一个任务用 Go 1.26 运行 `go vet` 与 `go test`，核对最低版本要求。检查使用占位凭据，不访问飞书。

第三个任务用 gitleaks 扫描所有分支的完整提交历史，规则见 `.gitleaks.toml`：在默认规则上增加 macOS 用户目录与外接卷路径；`internal/gateway/guidance/` 中来自上游文档的示例值只放行 `generic-api-key`。推送前可在本地运行 `gitleaks git --config .gitleaks.toml --redact`。命中真实凭据时，先到对应平台吊销或轮换，再处理提交历史；确认是示例值时，在 `.gitleaks.toml` 中按路径和规则放行。

## 发布

在 Actions 中手动运行 Release 工作流，填写版本号（如 `v0.3.0-dev`）：

1. 运行与 PR 检查相同的验证。
2. `build_release.py` 生成 macOS arm64 与 Linux amd64 发布包（其中的 Skill 带离线指南）、对应源码包、`PROVENANCE.json` 和 `SHA256SUMS`。
3. `verify_release.py` 核对 `SHA256SUMS` 覆盖全部发布文件且校验和一致，再解开每个目标包，在关闭网络、使用空模块缓存与构建缓存的条件下用源码包重建，逐个比较包内二进制、`PROVENANCE.json` 记录与重建结果；结果写入 `RELEASE-VERIFY.json`。
4. 为发布包和 `PROVENANCE.json` 生成 GitHub 构建证明，创建草稿版本。版本号含 `-` 时标记为预发布。

维护者核对草稿的说明与附件后手动发布。修改发布脚本或该工作流的 PR 会运行密钥扫描与第 2、3 步，不创建版本。

发布前仍需完成真实飞书和 Wake 的联调。
