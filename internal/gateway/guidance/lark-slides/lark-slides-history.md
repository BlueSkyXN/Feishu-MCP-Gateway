> 由 lark-cli `7f70de0666a2` 的 `lark-slides` 参考文档转换；调用约定见 `slides` 领域指南（`feishu_guide` 的 domain 参数）。

# slides history（历史版本与回滚）

用于查看 Slides XML presentation 历史版本、按 `history_version_id` 回滚，以及查询回滚任务状态。

`entries[].edit_time` 是 UTC RFC3339 时间字符串（例如 `2026-06-22T12:24:45Z`）。按时间匹配时先将其解析为时间值，再比较先后关系或时间差。

## 安全流程

1. 先用分页接口 `slides.+history-list` 找到目标版本的 `history_version_id`。
2. 如果用户指定的是 `revision_id`，不要假设它唯一，也不要把 `revision_id` 直接传给 `slides.+history-revert`。先拉一页并在 `entries[]` 中筛选 `revision_id` 相同的候选；如果未匹配到且 `has_more=true`，继续用 `page_token` 翻页；如果已匹配到候选，最多额外再拉一页补齐可能跨页的相邻候选。最终优先根据用户目标时间与 `edit_time` 的接近程度选择最合适的一条，取同一条的 `history_version_id`；如果没有目标时间，或多个候选无法可靠区分，再向用户展示候选版本（`history_version_id`、`revision_id`、`edit_time`、`name/description`）并确认后回滚。
3. 如果用户指定的是某一时刻但没有指定 `revision_id`，按 `entries[].edit_time` 匹配；优先选择不晚于目标时刻的最近一条历史记录，无法明确匹配时先向用户确认候选版本。
4. 使用 `slides.+history-revert` 发起回滚。接口会立即返回 `task_id`，回滚任务在服务端异步执行。
5. 如果返回 `status: running`，保存 `task_id`，按照返回的 `poll_after_ms` 等待后调用 `slides.+history-revert-status`。任务创建成功后，不得因为状态查询失败而重新发起回滚。
6. 状态变为 `done`、`partial_failed` 或 `failed` 后停止轮询；达到整体轮询上限时也停止轮询，并向用户返回 `task_id` 和当前状态。
7. 回滚完成后，用 `slides.+xml-get` 读取演示文稿确认内容。

## 按 revision_id 或时间点回滚

当用户说“回滚到 revision_id=42”“恢复到昨天下午 3 点的版本”这类需求时，流程是：

1. 执行 `slides.+history-list`（presentation="<presentation>"） 获取第一页历史记录；`slides.+history-list` 是分页接口，只有 `has_more=true` 且还需要更多候选时才继续传 `page_token` 翻页。
2. 如果用户给出 `revision_id`：先筛选当前页中 `entries[].revision_id == 用户给出的 revision_id`。如果未命中且 `has_more=true`，继续拉下一页；如果已经命中候选，最多额外再拉一页，补齐同一个 `revision_id` 可能跨页出现的相邻 `history_version_id`。若用户同时给出目标时间，在候选里选择 `edit_time` 与目标时间最接近的一条；若未给目标时间但候选只有一条，可直接使用；若多个候选无法可靠区分，不要自行取第一条，向用户展示候选并确认。
3. 如果用户只给出时间：用 `entries[].edit_time` 匹配，选择目标时刻之前最近的一条；如果用户表达的是“最接近某时刻”，则选择绝对时间差最小的一条。
4. 从最终匹配条目读取 `history_version_id`。`history_version_id` 对应服务端 `minor_history.version`，这是回滚接口需要的 ID。
5. 执行 `slides.+history-revert`（presentation="<presentation>"，history_version_id="<history_version_id>"）。

候选确认时使用类似格式：

```text
同一个 revision_id 命中多个历史版本，请确认要回滚哪一条：
- history_version_id=11 revision_id=42 edit_time=2026-06-22T12:24:45Z name=...
- history_version_id=12 revision_id=42 edit_time=2026-06-22T12:25:14Z name=...
```

## 命令

```text
# 列出历史版本
slides.+history-list {"presentation": "<slides_url_or_token>", "page_size": 20}

# 翻页
slides.+history-list {"presentation": "<slides_url_or_token>", "page_size": 20, "page_token": "<page_token>"}

# 发起回滚任务，立即返回 task_id
slides.+history-revert {"presentation": "<slides_url_or_token>", "history_version_id": "42"}

# 查询回滚任务状态
slides.+history-revert-status {"presentation": "<slides_url_or_token>", "task_id": "<task_id>"}
```

## 参数

| 命令 | 参数 | 必填 | 说明 |
|-|-|-|-|
| `slides.+history-list` | `presentation` | 是 | `xml_presentation_id`、Slides URL，或可解析为 Slides 的 wiki URL |
| `slides.+history-list` | `page_size` | 否 | 返回条数，范围 `1-20`，默认 `20` |
| `slides.+history-list` | `page_token` | 否 | 上一页返回的 `page_token` |
| `slides.+history-revert` | `presentation` | 是 | 同一个演示文稿 |
| `slides.+history-revert` | `history_version_id` | 是 | `slides.+history-list` 返回的 `history_version_id`，必须大于 0 |
| `slides.+history-revert-status` | `presentation` | 是 | 同一个演示文稿 |
| `slides.+history-revert-status` | `task_id` | 是 | `slides.+history-revert` 返回的 `task_id` |

## 异步轮询策略

1. `slides.+history-revert` 返回 `task_id` 后，认为回滚任务已经成功创建。
2. 如果 `status` 不是 `running`，不再调用状态接口。
3. 如果 `status` 是 `running`，等待响应中的 `poll_after_ms` 后调用 `slides.+history-revert-status`；`poll_after_ms` 缺失、为 `0` 或非法时，默认等待 10 秒。
4. 状态查询返回 `running` 时继续轮询；返回 `done`、`partial_failed` 或 `failed` 时停止。
5. 除非用户另有要求，默认最多轮询 5 分钟。达到上限后停止轮询，向用户说明任务仍在运行并返回 `task_id`，不得将其描述为回滚失败。
6. 状态查询出现临时错误时，按相同间隔最多连续重试 3 次；只重试 `slides.+history-revert-status`，不得重新调用 `slides.+history-revert`。
7. `done` 后读取当前演示文稿内容进行验证。
8. `partial_failed` 或 `failed` 时展示 `failed_block_tokens`；除非用户明确确认，不得自动再次发起回滚。

## 返回值要点

`slides.+history-list` 返回：

```json
{
  "entries": [
    {
      "revision_id": 42,
      "history_version_id": "11",
      "edit_time": "2026-06-22T12:24:45Z",
      "type": 1,
      "name": "版本名",
      "description": "版本说明",
      "editor_ids": ["ou_xxx"]
    }
  ],
  "has_more": true,
  "page_token": "page_token"
}
```

`slides.+history-revert` 返回：

```json
{
  "task_id": "task_xxx",
  "status": "running",
  "history_version_id": "11",
  "poll_after_ms": 10000
}
```

`slides.+history-revert-status` 返回：

```json
{
  "status": "partial_failed",
  "history_version_id": "11",
  "failed_block_tokens": ["blk_xxx"]
}
```

`status` 可能是 `running`、`done`、`partial_failed`、`failed`。当状态是 `partial_failed` 或 `failed` 时，优先检查 `failed_block_tokens`。

## 回滚后验证

回滚成功后必须读取一次当前内容确认：

```text
slides.+xml-get {"presentation": "<slides_url_or_token>", "output": "./presentation.xml"}
```
