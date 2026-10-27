> 由 lark-cli `7f70de0666a2` 的 `lark-meeting` 参考文档转换；调用约定见 `vc` 领域指南（`feishu_guide` 的 domain 参数）。

# `note.+transcript` (not available in the gateway: user identity only)

只在 `note.+detail` 已确认 `note_display_type=unified` 时使用。普通纪要逐字稿是独立 Docx 文档，应回到 `docs` 领域指南 读取 `verbatim_doc_token`。

```text
note.+transcript  # 网关不提供：仅支持用户身份
```

## 行为契约

- CLI 会先校验该 Note 是否为 `unified`；不是 unified 时不拉取 transcript。
- CLI 内部自动翻页并拼接完整内容；任一页失败时整体报错，不保存半截 transcript。
- 默认保存到 `./notes/{note_id}/unified_transcript.md`；`--transcript-format plain_text` 时保存为 `.txt`。
- 目标文件已存在时会失败；用户明确要覆盖时才加 `overwrite`。

## 相关场景
- 基于 note_id 查询纪要、逐字稿、共享文档等（主题 `query-note-and-artifacts`）
