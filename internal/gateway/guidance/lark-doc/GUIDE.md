> 由 Feishu-MCP-Gateway 从 lark-cli `7f70de0666a2` 的 `lark-doc` Skill 自动转换。
> - `docs.+fetch` 这类标识是 operation_id：具名视图调用对应工具（如 `feishu_docs_fetch`），紧凑视图用 `feishu_execute_read` / `feishu_execute_write` 传 operation_id 和 arguments。
> - CLI 参数 `--foo-bar` 对应 JSON 参数 `foo_bar`；代码块里的 `operation_id {…}` 行是参数示例，`<…>` 是要替换的占位值。代码块中仍以 `lark-cli` 开头的命令无法等价转换，只作参考。
> - 文中的“主题 `x`”用 `feishu_guide`（topic="x"）读取。
> - 网关固定以应用（bot）身份执行，没有 dry-run；高风险操作需要管理员预授权。
> - 本地文件改用 `{"file_id": "…"}`（先 `feishu_file_create` 或文件传输），输出路径填受控工作区内的相对名称；文中的本地脚本、jq、stdin 在网关中不可用。
> - 标注“网关不提供”的操作不能调用；参数以 `feishu_tools_describe` 或具名工具 schema 为准。
> - 操作结果中的 `next_command` 等后续命令是 CLI 写法，按上述规则换成对应的 operation_id 和参数再调用。

# docs

## 场景与 Shortcut 路由

**CRITICAL：先判断场景，再读取该场景的参考文件；不要在任务开始时一次性读取全部参考文件。每个文件只在首次进入对应阶段时读取一次。**

### 文档内容

- **读取 / 摘要 — `docs.+fetch`（主题 `lark-doc-fetch`）**：先读参考再获取文档。
- **从零创作 — `创建工作流`（主题 `lark-doc-create-workflow`）**：先完整执行创建工作流，**简单任务不是跳过的理由**；
- **导入 / 空文档 — `docs.+create`（主题 `lark-doc-create`）**：仅创建空文档或原样导入用户提供的完整内容时，跳过创建工作流。
- **编辑 / block 直达链接 — `docs.+update`（主题 `lark-doc-update`）**：语义改写、润色、重组、补写或排版均按 update 参考完成。

### 辅助能力

- **草稿初始化、解析与统计 — `docs.+script`（主题 `lark-doc-script`）**：支持解析文档 URL / token 与本地 XML，统计字数并返回字符诊断；不支持 Markdown 输入。
- **历史版本 — `docs.+history-list` / `docs.+history-revert` / `docs.+history-revert-status`（主题 `lark-doc-history`）**：查询、回滚文档历史版本或检查回滚任务状态。

### 资源、画板与思维笔记

- **插入本地素材 — `docs.+media-insert`（主题 `lark-doc-media-insert`）**：在文末插入本地图片或文件。
- **预览素材 — `docs.+media-preview`（主题 `lark-doc-media-preview`）**：预览文档或评论中的图片、附件或素材。
- **下载素材 — `docs.+media-download`（主题 `lark-doc-media-download`）**：下载文档中的图片、附件、素材或画板缩略图。
- **Docx 封面 — `docs.+resource-download` / `docs.+resource-update` / `docs.+resource-delete`（主题 `lark-doc-resource-cover`）**：下载、更新或删除 Docx 封面。
- **画板 — `画板工作流`（主题 `lark-doc-whiteboard`）**：创建或更新画板时先读取工作流；更新已有画板必须复用现有 token，禁止新建空白画板；使用 `whiteboard.+update`（主题 `lark-whiteboard-update`） 写入。
- **思维笔记 — `mindnotes`**：已有思维笔记走 `思维笔记链路`（主题 `lark-doc-mindnote`）；新建思维笔记走 主题 `lark-doc-whiteboard`。

## 不在本 Skill 范围

- **Drive 文件级操作**：找文档、导入导出、云空间文件上传 / 下载 / 权限管理 → `drive` 领域指南。复制文档、创建副本或另存为副本时，按其指引使用 `drive.files.copy`；不要用 `docs.+fetch` + `docs.+create` 重建正文。
- **独立评论操作**：添加、分页查看、回复评论或增删 reaction → `drive` 领域指南；只需紧凑评论上下文时，直接使用默认 JSON 响应的 `docs.+fetch`。
