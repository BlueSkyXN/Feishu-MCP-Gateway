# 检查与发布

## PR 检查

`.github/workflows/ci.yml` 在每个 PR 和 main 的推送上运行，环境为 ubuntu（linux/amd64）：

1. 从 larksuite/cli 取目录记录的固定提交，构建受控后端，运行其 race 测试与 vet。
2. 使用最新稳定版 Go 运行 `go vet`、`go test`、`go test -race`，并构建 Gateway；实际 wrapper 的集成测试一起运行。
3. `verify_backend.py` 核对后端声明，`smoke_gateway.py` 启动独立进程检查。
4. 交叉构建 darwin/arm64。
5. 仓库提供 `scripts/refresh_catalog.py` 时，重新生成目录与指南，并要求结果与提交内容一致。

另一个任务用 Go 1.26 运行 `go vet` 与 `go test`，核对最低版本要求。检查使用占位凭据，不访问飞书。

## 发布

在 Actions 中手动运行 Release 工作流，填写版本号（如 `v0.3.0-dev`）：

1. 运行与 PR 检查相同的验证。
2. `build_release.py` 生成 macOS arm64 与 Linux amd64 发布包、对应源码包、`PROVENANCE.json` 和 `SHA256SUMS`。
3. `verify_release.py` 核对校验和，并在关闭网络、使用空模块缓存与构建缓存的条件下用源码包重建，确认二进制与发布包一致；结果写入 `RELEASE-VERIFY.json`。
4. 为发布包和 `PROVENANCE.json` 生成 GitHub 构建证明，创建草稿版本。版本号含 `-` 时标记为预发布。

维护者核对草稿的说明与附件后手动发布。修改发布脚本或该工作流的 PR 会运行第 2、3 步，不创建版本。

发布前仍需人工完成密钥与隐私扫描，以及真实飞书和 Wake 的联调。
