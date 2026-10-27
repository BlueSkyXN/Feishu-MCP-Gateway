> 由 lark-cli `7f70de0666a2` 的 `lark-okr` 参考文档转换；调用约定见 `okr` 领域指南（`feishu_guide` 的 domain 参数）。

# `okr.+comment-get`

根据评论 ID 获取单条 OKR 评论，查看评论正文、状态、评论对象、引用关系和划词信息。本 shortcut 适合用于在编辑评论后确认其最终状态。

## 推荐命令

```text
# 获取一条评论的简化正文和元数据。
okr.+comment-get {"comment_id": "7000000000000000001"}

# 获取原始 ContentBlock 格式的评论正文。
okr.+comment-get {"comment_id": "7000000000000000001", "style": "richtext"}

# 预览获取评论的 API 调用，不实际执行。
okr.+comment-get {"comment_id": "7000000000000000001"}
```

## 参数

| 参数           | 必填 | 默认值  | 说明                                                                                   |
|----------------|------|---------|----------------------------------------------------------------------------------------|
| `comment_id`   | 是   | —       | 评论 ID，int64 正整数。                                                                |
| `user_id_type` | 否   | open_id | open_id、union_id、user_id 或 user_key。                                               |
| `style`        | 否   | simple  | simple 返回半纯文本格式，不涉及字体/颜色等信息时推荐使用；richtext 返回 ContentBlock。 |

## 工作流程

1. 如果只有目标 ID，先用 `okr.+comment-list`（主题 `lark-okr-comment-list`） 或 `okr.+comment-detail`（主题 `lark-okr-comment-detail`） 定位 comment-id。
2. 执行 `okr.+comment-get` `comment_id` "..."。
3. 根据后续操作检查 selection、status 和 ref_comment_id：selection.id 表示划词评论，status 为 solved 表示已解决，ref_comment_id 表示引用关系。

## 输出

```json
{
  "comment": {
    "id": "7000000000000000001",
    "target": {"target_type": "progress", "target_id": "3456789012345678901"},
    "commentator_id": "ou_xxx",
    "status": "open",
    "create_time": "2025-01-15 10:30:00",
    "update_time": "2025-01-15 10:30:00",
    "content": {"text": "进展不错", "mention": [], "docs": [], "images": []},
    "ref_comment_id": "7000000000000000000"
  },
  "style": "simple"
}
```

selection、solver_id、solved_time 和 ref_comment_id 按接口是否返回保留。

## 注意事项

- Objective/KeyResult 的划词评论通过 selection.id 归属于评论串；实体级评论没有 selection。
- 解决或重新打开请使用 `okr.+comment-solve`（网关不提供：仅支持用户身份） / `okr.+comment-reopen`（网关不提供：仅支持用户身份）（主题 `lark-okr-comment-solve-reopen`）。

## 参考

- `okr` 领域指南 — OKR 命令、路由和通用约定
- OKR 实体定义（主题 `lark-okr-entities`） — Comment 字段与评论串规则
- ContentBlock 格式（主题 `lark-okr-contentblock`） — 评论正文格式
- `okr.+comment-list`（主题 `lark-okr-comment-list`） — 查询目标下的评论
