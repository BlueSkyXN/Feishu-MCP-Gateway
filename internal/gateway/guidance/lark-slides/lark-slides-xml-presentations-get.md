> 由 lark-cli `7f70de0666a2` 的 `lark-slides` 参考文档转换；调用约定见 `slides` 领域指南（`feishu_guide` 的 domain 参数）。

# `slides.+xml-get`（读取演示文稿 XML）

读取全文或单页 XML。全文验证优先将结果保存到本地文件；局部编辑可读取单页 XML，从顶层块的 `id` 属性取得 `slides.+replace-slide` 所需的 `block_id`。

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `presentation` | 是 | `xml_presentation_id`、Slides URL，或可解析为 Slides 的 wiki URL |
| `revision_id` | 否 | 版本号；`-1` 表示最新版本 |
| `raw` | 否 | 将 XML 直接输出到 stdout；不能与 `output`、jq（网关不提供） 或非 JSON  一起使用 |
| `slide_id` | 否 | 只读取指定页面；不能与 `slide_number` 或 `remove_attr_id` 一起使用 |
| `slide_number` | 否 | 只读取指定的 1-based 页码；不能与 `slide_id` 或 `remove_attr_id` 一起使用 |
| `remove_attr_id` | 否 | 仅全文读取可用；移除 XML `id` 属性，不适合后续精确块编辑 |

## 示例

```bash

# 读取单页，同时记录 revision_id 用于后续乐观锁
```

JSON 输出中，全文 XML 位于 `.data.xml_presentation.content`，单页 XML 位于 `.data.slide.content`；二者的 `.data.revision_id` 都可用于后续写操作。

相关命令：

- `slides.+replace-slide`（主题 `lark-slides-replace-slide`） — 块级替换 / 插入
- `slides.+update-slide`（主题 `lark-slides-update-slide`） — 整页覆盖
