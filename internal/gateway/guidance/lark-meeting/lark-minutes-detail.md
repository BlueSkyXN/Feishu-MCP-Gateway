> 由 lark-cli `7f70de0666a2` 的 `lark-meeting` 参考文档转换；调用约定见 `vc` 领域指南（`feishu_guide` 的 domain 参数）。

# `minutes.+detail`

> `--summary` / `--todo` / `--chapter` / `--keyword` / `--transcript` 都是可选项；请求相应产物时必须显式传入，不传任何产物 flag 时只返回基础信息（如 `title`），AI 产物字段都不会出现。一次性获取所有产物：`--summary --todo --chapter --keyword --transcript`。

## 命令

```bash

# 仅逐字稿，覆盖已有文件，指定输出目录
```

## 输出

`minutes` 数组每条含 `minute_token`、`title`、`note_id`、`artifacts`。`note_id` 仅在该妙记关联了会议纪要时返回，可直接传给 `note.+detail`（主题 `lark-note-detail`） 拿纪要文档 token，无需再绕回 `vc.+detail`。`artifacts` 中**只包含本次请求的产物**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `artifacts.summary` | string | AI 总结。 |
| `artifacts.todos` | array | 待办事项列表。 |
| `artifacts.chapters` | array | 章节列表。 |
| `artifacts.keywords` | array | 关键词列表。 |
| `artifacts.transcript_file` | string | 逐字稿本地文件路径。 |

逐字稿默认落地 `./minutes/{minute_token}/transcript.txt`，与 `minutes.+download` 同目录便于聚合。指定 `output_dir=<dir>` 时改写到 `<dir>/artifact-{title}-{minute_token}/transcript.txt`。

## minute_token 来源

| 来源 | 取值字段 |
|------|---------|
| 妙记 URL `https://*.feishu.cn/minutes/obcxxx` | 截路径最后一段 `obcxxx` |
| `vc.+detail` | `minute_token` |
| `vc.+recording` | `minute_token` |
| `minutes.+search` | `minute_token` |

`minute_token` 不要直接传给 `note.+detail`：需要关联 Note 时，先从本命令结果读取 `note_id`。跨产物流程由 主题 `query-minutes-and-artifacts` 编排。

## 相关场景
- 查询妙记及其产物（主题 `query-minutes-and-artifacts`）
