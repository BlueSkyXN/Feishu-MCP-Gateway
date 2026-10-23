# 版权与第三方许可

Feishu-MCP-Gateway 的原创代码 Copyright (c) 2026 BlueSkyXN，采用 GPL-3.0-only。完整许可见 LICENSE。组合分发遵循 GPLv3；下面的第三方材料继续保留原有许可与署名。

`internal/gateway/catalog.json` 与 `docs/coverage.json` 根据 `larksuite/cli` 的已提交 Catalog Snapshot 和公开 Shortcut 声明转换生成。来源版本记录在文件中；转换脚本为 `scripts/import_catalog.py`。

`internal/gateway/dpop_url.go` 的 HTU 规范化实现来自同一固定版本，保留原始版权声明。`backend/` 通过其公开 extension 与 command 包构建受控 wrapper。事件加密协议核对于 MIT 许可的 `github.com/larksuite/oapi-sdk-go/v3@v3.7.2/event/event.go`（Copyright 2022 Lark Technologies Pte. Ltd.）。

来源：https://github.com/larksuite/cli

MIT License

Copyright (c) 2026 Lark Technologies Pte. Ltd.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

Go 依赖的版本锁定在 go.mod / go.sum 和对应源码包的 backend/go.mod / go.sum。Release 中的 THIRD_PARTY_LICENSES 包含实际链接依赖的许可文本与清单；对应源码包包含 Gateway、固定版本 lark-cli、vendor 依赖与离线构建脚本。Go 工具链与操作系统组件由构建环境提供。
