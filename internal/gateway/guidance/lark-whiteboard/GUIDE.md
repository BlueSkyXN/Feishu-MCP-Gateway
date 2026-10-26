> 由 Feishu-MCP-Gateway 从 lark-cli `7f70de0666a2` 的 `lark-whiteboard` Skill 自动转换。
> - `docs.+fetch` 这类标识是 operation_id：具名视图调用对应工具（如 `feishu_docs_fetch`），紧凑视图用 `feishu_execute_read` / `feishu_execute_write` 传 operation_id 和 arguments。
> - CLI 参数 `--foo-bar` 对应 JSON 参数 `foo_bar`；代码块里的 `operation_id {…}` 行是参数示例。
> - 文中的“主题 `x`”用 `feishu_guide`（topic="x"）读取。
> - 网关固定以应用（bot）身份执行，没有 dry-run；高风险操作需要管理员预授权。
> - 本地文件改用 `{"file_id": "…"}`（先 `feishu_file_create` 或文件传输），输出路径填受控工作区内的相对名称；文中的本地脚本、jq、stdin 在网关中不可用。
> - 标注“网关不提供”的操作不能调用；参数以 `feishu_tools_describe` 或具名工具 schema 为准。

> [!IMPORTANT]
> - 运行 `lark-cli --version`，确认可用，无需询问用户。
> - 运行 `npx -y @larksuite/whiteboard-cli@^0.2.13 -v`，确认可用，无需询问用户。

---

## 快速决策

> 先判断「只读还是写入」，再在对应表内按上到下匹配，**命中即停**。

### A. 只读 · 查看 / 导出（不改画板）

| 用户需求 | 行动 |
|---|---|
| 查看画板内容 / 导出图片 | `whiteboard.+export`（output_type="preview"）（主题 `lark-whiteboard-export`）                       |
| 导出 SVG 矢量图 | `whiteboard.+export`（output_type="svg"）（主题 `lark-whiteboard-export`）                       |
| 提取画板的 Mermaid/PlantUML 源码 | `whiteboard.+export`（output_type="source"）（主题 `lark-whiteboard-export`） |

### B. 写入 · 创作 / 编辑（会改画板，命中即停）

| 场景 | 行动 | 写入方式 | 对原内容 |
|---|---|---|---|
| 用户**已提供** Mermaid/PlantUML/SVG 代码，或明确指定用该格式 | 使用该代码 → `whiteboard.+update`（主题 `lark-whiteboard-update`），`input_format` 取单值 `mermaid` / `plantuml` / `svg`；写入非空已有画板并需要 overwrite 时，先确认会整板重建；若 SVG 用于修改已有画板，先走 主题 `svg-edit`（主题 `svg-edit`） 有损确认 | overwrite / append | 按用户要求 |
| 从零新建复杂图表（架构/流程/组织等） | → **§ 创作 Workflow（主题 `lark-whiteboard-workflow`）** | 首次写入 | — |
| 修改 / 增补已有画板 | → **§ 编辑 Workflow（主题 `lark-whiteboard-workflow`）** | 见该表 | 见该表 |

## Shortcuts

| Shortcut                                          | 说明 |
|---------------------------------------------------|---|
| `whiteboard.+export`（主题 `lark-whiteboard-export`） | 导出画板为预览图片、SVG 矢量图、代码或原始节点结构。 |
| `whiteboard.+update`（主题 `lark-whiteboard-update`） | 更新画板，支持 PlantUML、Mermaid、SVG 或 OpenAPI 原生格式 |

---

## 不在本 skill 范围
- 文档内容编辑 → `docs` 领域 `docs` 领域指南
- 在文档中创建画板 → lark-doc-whiteboard.md（主题 `lark-doc-whiteboard`）
- 表格 / Base 操作 → `sheets` 领域指南 / `base` 领域指南
