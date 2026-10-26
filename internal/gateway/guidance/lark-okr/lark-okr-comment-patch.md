> 由 lark-cli `7f70de0666a2` 的 `lark-okr` 参考文档转换；调用约定见 `okr` 领域指南（`feishu_guide` 的 domain 参数）。

# `okr.+comment-patch`（网关不提供：仅支持用户身份）

修改指定评论的正文。评论目标、划词定位、引用关系则一经创建不可修改。只支持 user 身份。

`content` 是业务必填项：OpenAPI schema 中该字段可能表现为可选，但实际修改评论必须提供非空正文。

## 推荐命令

```text
# 使用 simple 风格修改评论正文。
okr.+comment-patch  # 网关不提供：仅支持用户身份

# 使用 richtext 文件修改评论正文。
okr.+comment-patch  # 网关不提供：仅支持用户身份

# 写入前预览修改评论的 API 调用，不实际执行。
okr.+comment-patch  # 网关不提供：仅支持用户身份
```

## 参数

| 参数           | 必填 | 默认值  | 说明                                                                                                                          |
|----------------|------|---------|-------------------------------------------------------------------------------------------------------------------------------|
| `comment_id`   | 是   | —       | 评论 ID，int64 正整数；可从 `okr.+comment-list`（主题 `lark-okr-comment-list`） 或 `okr.+comment-detail`（主题 `lark-okr-comment-detail`） 获取。 |
| `content`      | 是   | —       | 新正文；输入风格：`simple`（半纯文本 JSON，推荐） \| `richtext`（完整 ContentBlock JSON），支持 @文件路径。                   |
| `style`        | 否   | simple  | 输入/输出风格：simple 或 richtext。                                                                                           |
| `user_id_type` | 否   | open_id | open_id、union_id、user_id 或 user_key。                                                                                      |
| dry-run（网关不提供，改为先读取核对）      | 否   | —       | 预览 API 调用而不实际执行。                                                                                                   |
|        | 否   | json    | 输出格式。                                                                                                                    |

## 工作流程

1. 使用 `okr.+comment-list`（主题 `lark-okr-comment-list`）、`okr.+comment-detail`（主题 `lark-okr-comment-detail`） 或 `okr.+comment-get`（主题 `lark-okr-comment-get`） 确认 comment-id 和目标评论。
2. 准备 content：通常建议使用 simple 格式，需要精确控制 @用户的位置时，可以使用 richtext 格式，参考 ContentBlock 格式（主题 `lark-okr-contentblock`）
3. 执行 `okr.+comment-patch`（网关不提供：仅支持用户身份）；真实写入前用 dry-run（网关不提供，改为先读取核对） 检查请求。
4. 如果要解决或重新打开评论，不要使用 patch，改用 `okr.+comment-solve`（网关不提供：仅支持用户身份）（主题 `lark-okr-comment-solve-reopen`） 或 `okr.+comment-reopen`（网关不提供：仅支持用户身份）（主题 `lark-okr-comment-solve-reopen`）。

## 输出

返回 JSON：

```json
{
  "comment": {
    "id": "7000000000000000004",
    "target": {"target_type": "progress", "target_id": "3456789012345678901"},
    "commentator_id": "ou_xxx",
    "status": "open",
    "create_time": "2025-01-15 10:30:00",
    "update_time": "2025-01-15 11:00:00",
    "content": {"text": "更新后的评论", "mention": [], "docs": [], "images": []}
  },
  "style": "simple"
}
```

simple 风格的 content 为 SemiPlainContent；richtext 风格的 content 为 ContentBlock。

## 注意事项

- patch 不会改变评论的 target、selection、ref_comment_id 或 status。
- simple 输入不支持 docs/images；需要富文本元素时使用 richtext。
- 空正文不允许提交；如需删除评论，请使用 `okr.+comment-delete`（网关不提供：仅支持用户身份）（主题 `lark-okr-comment-delete`），删除不可恢复。

## 参考

- `okr` 领域指南 — OKR 命令、路由和通用约定
- OKR 实体定义（主题 `lark-okr-entities`） — Comment 字段与评论串规则
- ContentBlock 格式（主题 `lark-okr-contentblock`） — 评论正文格式
- `okr.+comment-get`（主题 `lark-okr-comment-get`） — 获取更新前后的评论
- `okr.+comment-delete`（网关不提供：仅支持用户身份）（主题 `lark-okr-comment-delete`） — 永久删除评论
