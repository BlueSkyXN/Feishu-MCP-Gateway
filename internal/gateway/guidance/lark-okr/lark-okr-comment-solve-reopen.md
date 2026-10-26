> 由 lark-cli `7f70de0666a2` 的 `lark-okr` 参考文档转换；调用约定见 `okr` 领域指南（`feishu_guide` 的 domain 参数）。

# `okr.+comment-solve`（网关不提供：仅支持用户身份） / `okr.+comment-reopen`（网关不提供：仅支持用户身份）

解决/重新打开一条评论。实体级评论按单条评论处理；划词评论则是操作整个评论串。只支持 user 身份。

## 推荐命令

```text
# 解决实体级评论或整个划词评论串。
okr.+comment-solve  # 网关不提供：仅支持用户身份

# 重新打开已解决的实体级评论或划词评论串。
okr.+comment-reopen  # 网关不提供：仅支持用户身份

# 预览解决评论的状态变更请求，不实际执行。
okr.+comment-solve  # 网关不提供：仅支持用户身份
```

## 参数

| 参数           | 必填 | 默认值  | 说明                                                                                  |
|----------------|------|---------|---------------------------------------------------------------------------------------|
| `comment_id`   | 是   | —       | 评论 ID，int64 正整数。可从 `okr.+comment-list`、`okr.+comment-detail` 或 `okr.+comment-get` 获取。     |
| `user_id_type` | 否   | open_id | open_id、union_id、user_id 或 user_key。                                              |
| `style`        | 否   | simple  | affected_comments 的正文风格：simple（SemiPlainContent）或 richtext（ContentBlock）。 |
| dry-run（网关不提供，改为先读取核对）      | 否   | —       | 预览 API 调用而不实际执行。                                                           |
|        | 否   | json    | 输出格式。                                                                            |

## 工作流程

1. 使用 `okr.+comment-list`（主题 `lark-okr-comment-list`）、`okr.+comment-detail`（主题 `lark-okr-comment-detail`） 或 `okr.+comment-get`（主题 `lark-okr-comment-get`） 获取并确认 comment-id。
2. 检查评论是否属于划词串：如果返回有 selection.id，solve/reopen 会影响同一 selection.id 下的全部评论。
3. 根据用户动作选择 `okr.+comment-solve`（网关不提供：仅支持用户身份） 或 `okr.+comment-reopen`（网关不提供：仅支持用户身份）；先用 dry-run（网关不提供，改为先读取核对） 检查目标接口。
4. 执行后检查 affected_comments，确认实体级评论或整条评论串的状态变化范围。

## 输出

返回 JSON：

```json
{
  "affected_comments": [
    {
      "id": "7000000000000000004",
      "target": {
        "target_type": "objective",
        "target_id": "2345678901234567890"
      },
      "commentator_id": "ou_xxx",
      "status": "solved",
      "create_time": "2025-01-15 10:30:00",
      "update_time": "2025-01-15 11:30:00",
      "selection": {
        "id": "8000000000000000001",
        "selected_text": "提升核心接口稳定性"
      },
      "content": {
        "text": "请补充指标", "mention": [], "docs": [], "images": []
      }
    }
  ],
  "style": "simple"
}
```

- `okr.+comment-solve`（网关不提供：仅支持用户身份） 成功后 affected_comments 的 status 通常为 solved；`okr.+comment-reopen`（网关不提供：仅支持用户身份） 成功后通常为 open。
- simple 风格返回 SemiPlainContent；richtext 风格返回 ContentBlock。

## 注意事项

- 划词评论按评论串解决/重开，但 `okr.+comment-delete`（网关不提供：仅支持用户身份）（主题 `lark-okr-comment-delete`） 仍然只删除单条评论。
- 解决不是删除，之后可以用 `okr.+comment-reopen`（网关不提供：仅支持用户身份） 恢复；删除后不可恢复。
- 该操作是写操作，执行前应确认 comment-id 和目标动作。

## 参考

- `okr` 领域指南 — OKR 命令、路由和通用约定
- OKR 实体定义（主题 `lark-okr-entities`） — Comment、评论串和状态规则
- ContentBlock 格式（主题 `lark-okr-contentblock`） — affected_comments 正文格式
- `okr.+comment-get`（主题 `lark-okr-comment-get`） — 获取状态和 selection.id
- `okr.+comment-delete`（网关不提供：仅支持用户身份）（主题 `lark-okr-comment-delete`） — 永久删除单条评论
