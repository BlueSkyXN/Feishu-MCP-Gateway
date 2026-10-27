> 由 lark-cli `7f70de0666a2` 的 `lark-doc` 参考文档转换；调用约定见 `docs` 领域指南（`feishu_guide` 的 domain 参数）。

# `docs.+create`（创建飞书云文档）

从 XML（默认）或 Markdown 内容创建一个新的飞书云文档；语义创作默认使用 XML，只有 Authoring 明确判定为 Markdown 例外时才使用 Markdown。

写入前必须按 `doc_format` 读取对应格式参考：`xml` 读取 主题 `lark-doc-xml`，`markdown` 读取 主题 `lark-doc-md`；Markdown 中使用 XML 扩展标签时还须读取 主题 `lark-doc-xml`。

## 命令

```text
# 简单内容优先使用 `--content -`，文件导入如下：
docs.+create {"doc_format": "xml", "content": {"file_id": "<file_id>"}}
docs.+create {"doc_format": "markdown", "content": {"file_id": "<file_id>"}}
```

## 返回值

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "document": {
      "document_id": "docx_token",
      "revision_id": 1,
      "url": "https://xxx.feishu.cn/docx/docx_token",
      "new_blocks": [
        { "block_id": "blkcnXXXX", "block_type": "whiteboard", "block_token": "boardXXXX" }
      ]
    },
    "warnings": [],
    "tips": ""
  }
}
```

- **`document.new_blocks`**：本次操作新增的 block 列表（如画板）。`block_id` 可用于 `docs.+update` 的 `block_id` 做精确编辑；`block_token` 是资源块（如画板）的 token，可交给 `whiteboard` 领域 等 skill 继续操作。
- **`warnings`**：服务端返回的警告列表；`ok=true` 时也要检查，按提示确认是否存在降级或未完全处理的内容。
- **`tips`**：服务端返回的后续处理建议；为空表示没有额外建议，非空本身不表示创建失败。
- **`permission_grant`**：仅以 bot 身份创建时返回。CLI 会尝试为当前 CLI 用户授予新文档的 `full_access`；`status` 为 `granted` 表示授权成功，`skipped` 表示没有可用的当前用户 `open_id`，`failed` 表示文档已创建但授权失败。`perm` 固定为 `full_access`，失败或跳过时按 `message` / `hint` 处理。**自动授权不等于 owner 转移；用户要求转移 owner 时必须单独确认。**

## 参数

|参数|必填|说明|
|-|-|-|
|`title`|否|文档标题，Markdown 导入时使用；XML 创建推荐在 `content` 开头写 `<title>...</title>`；多个标题仅保留第一个|
|`content`|视情况|文档内容（XML 或 Markdown 格式）；不传 `content` 时必须传 `title`|
|`reference_map`|否|结构化 `reference_map` JSON object；必须与 `content` 一起使用。普通写入优先把结构写在正文里；该参数主要用于保留或回放已有 `document.reference_map`。支持直接 JSON、任务独占目录内的相对 `{"file_id": …}`，或 `-` 从 stdin 读取。|
|`doc_format`|否|CLI 与语义创作均默认 `xml`，并建议显式传入；仅用户明确要求 Markdown 或保真导入 Markdown 时使用 `markdown`。不要混用完整的 XML 与 Markdown 文档格式；Markdown 中允许使用文档已定义的 XML 扩展标签。|
|`parent_token`|否|父文件夹或知识库节点 token（与 `parent_position` 互斥）|
|`parent_position`|否|父节点位置，如 `my_library`（与 `parent_token` 互斥）|

## 需要回查文档

用 `docs.+fetch`（doc="<document_id 或文档 URL>"，detail="with-ids"） 回查，若需要更多信息可查看 `docs.+fetch`（主题 `lark-doc-fetch`）。
