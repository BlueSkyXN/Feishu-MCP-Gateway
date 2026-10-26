> 由 Feishu-MCP-Gateway 从 lark-cli `7f70de0666a2` 的 `lark-markdown` Skill 自动转换。
> - `docs.+fetch` 这类标识是 operation_id：具名视图调用对应工具（如 `feishu_docs_fetch`），紧凑视图用 `feishu_execute_read` / `feishu_execute_write` 传 operation_id 和 arguments。
> - CLI 参数 `--foo-bar` 对应 JSON 参数 `foo_bar`；代码块里的 `operation_id {…}` 行是参数示例。
> - 文中的“主题 `x`”用 `feishu_guide`（topic="x"）读取。
> - 网关固定以应用（bot）身份执行，没有 dry-run；高风险操作需要管理员预授权。
> - 本地文件改用 `{"file_id": "…"}`（先 `feishu_file_create` 或文件传输），输出路径填受控工作区内的相对名称；文中的本地脚本、jq、stdin 在网关中不可用。
> - 标注“网关不提供”的操作不能调用；参数以 `feishu_tools_describe` 或具名工具 schema 为准。

# markdown (v1)

## 快速决策

- `markdown.+create` / `markdown.+overwrite` 失败时，先判断是不是身份和权限问题：`bot` 更常见的是 app scope 或目标目录 ACL，`user` 更常见的是用户授权或用户 ACL；不要不加判断地来回切身份重试。

- 用户要**上传、创建一个原生 `.md` 文件**，使用 `markdown.+create`
- 用户要**比较原生 `.md` 文件的历史版本差异**，或比较远端 Markdown 与本地草稿，使用 `markdown.+diff`
- 用户要**读取 Drive 里某个 `.md` 文件内容**，使用 `markdown.+fetch`
- 用户要对 Markdown 文件做**局部文本替换 / 正则替换**，优先使用 `markdown.+patch`
- 用户要**覆盖更新 Drive 里某个 `.md` 文件内容**，使用 `markdown.+overwrite`
- 用户要先拿 Markdown 文件的历史版本号，再做比较/下载/回滚，先用 `drive` 领域指南 的 `drive.+version-history`
- 用户要把本地 Markdown **导入成在线新版文档（docx）**，不要用本 skill，改用 `drive` 领域指南 的 `drive.+import`（type="docx"）
- 用户要对 Markdown 文件做**rename / move / delete / 搜索 / 权限 / 评论**等云空间（云盘/云存储）操作，不要留在本 skill，切到 `drive` 领域指南
- `markdown.+create` / `markdown.+overwrite` 命中 `missing scope`、`permission denied`、`not found`、`quota_exceeded`、`version limit` 时，默认停止重试并按报错 hint 处理；只有 `rate_limit`、`server_error` 或临时网络错误才做有限退避重试。
- `markdown.+create` 的目标参数不要猜：Drive 文件夹用 `folder_token`，Wiki 节点用 `wiki_token`。如果用户给的是 URL，可以直接传完整 URL；CLI 会归一成 token。不要把 doc/sheet/wiki URL 放进 `folder_token` 试错。

## 核心边界

- 本 skill 处理的是 **Drive 中作为普通文件存储的 Markdown**，不是 docx 文档
- `name` 和本地 `file` 文件名都必须显式带 `.md` 后缀；不满足时 shortcut 会直接报错
- `content` 支持：
  - 直接传字符串
  - `{"file_id": …}` 从本地文件读取内容
  - `-` 从 stdin 读取内容
- `markdown.+patch` 的内部语义是：**先完整下载 Markdown，再本地替换，再整文件覆盖上传**
- `markdown.+patch` 不是服务端原子 patch；它是 CLI 侧编排出来的局部更新能力
- `markdown.+patch` 当前只支持**单组** `pattern` / `content`
- `markdown.+patch` 替换后的最终内容**不能为空**；CLI 会拒绝上传空文件，因为 Drive 不支持零字节 Markdown，且空文件通常是误操作
- `file` 只接受本地 `.md` 文件路径

正则替换时要特别注意 `pattern` 的转义：

```text
# BAD: 未转义正则特殊字符，可能匹配到错误位置
markdown.+patch {"file_token": "boxcnxxxx", "regex": true, "pattern": "version (1.0)", "content": "version (2.0)"}

# GOOD: 显式转义括号和点号
markdown.+patch {"file_token": "boxcnxxxx", "regex": true, "pattern": "version \\(1\\.0\\)", "content": "version (2.0)"}
```

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli markdown +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| `markdown.+create`（主题 `lark-markdown-create`） | Create a Markdown file in Drive |
| `markdown.+diff`（主题 `lark-markdown-diff`） | Compare two remote Markdown versions, or compare remote Markdown against a local file |
| `markdown.+fetch`（主题 `lark-markdown-fetch`） | Fetch a Markdown file from Drive |
| `markdown.+patch`（主题 `lark-markdown-patch`） | Patch a Markdown file in Drive via fetch-local-replace-overwrite |
| `markdown.+overwrite`（主题 `lark-markdown-overwrite`） | Overwrite an existing Markdown file in Drive |

## 参考

- `drive` 领域指南 — Drive 文件管理、导入 docx、move/delete/search 等
