> 由 lark-cli `7f70de0666a2` 的 `lark-okr` 参考文档转换；调用约定见 `okr` 领域指南（`feishu_guide` 的 domain 参数）。

# `okr.+comment-delete` (not available in the gateway: user identity only)

永久删除一条评论。删除划词评论时只删除指定评论，不会删除同一 selection.id 下的其他评论。

## 功能简介

删除一条特定评论。本 shortcut 为高风险接口，删除的评论不可找回，如果只是暂时结束讨论，可使用 `okr.+comment-solve`（网关不提供：仅支持用户身份）。只支持 user 身份。

## 推荐命令
```text
# 预览删除请求，不实际执行永久删除。
okr.+comment-delete  # 网关不提供：仅支持用户身份
# 确认删除目标后，执行不可恢复的删除操作。
okr.+comment-delete  # 网关不提供：仅支持用户身份
```

## 参数

| 参数         | 必填         | 默认值 | 说明                                                        |
|--------------|--------------|--------|-------------------------------------------------------------|
| `comment_id` | 是           | —      | 要删除的评论 ID，int64 正整数。建议先由 `okr.+comment-get` 核对。 |

## 工作流程

1. 使用 `okr.+comment-list`（主题 `lark-okr-comment-list`）、`okr.+comment-detail`（主题 `lark-okr-comment-detail`） 或 `okr.+comment-get`（主题 `lark-okr-comment-get`） 定位并确认 comment-id。
2. 判断是否真的需要删除：解决评论使用 `okr.+comment-solve`（网关不提供：仅支持用户身份）（主题 `lark-okr-comment-solve-reopen`），删除只用于永久移除内容。
3. 先执行带 dry-run（网关不提供，改为先读取核对） 的命令检查 URL 和 comment-id。
4. 向用户明确说明删除不可恢复；得到确认后，在原始命令末尾追加 管理员预授权（high_risk_operations） 执行。
5. 根据 deleted=true 和返回的 comment_id 确认结果。

## 输出

删除成功返回 JSON：
```json
{
  "deleted": true,
  "comment_id": "7000000000000000004"
}
```

## 注意事项

- 删除是单条评论级操作，即使评论属于划词评论串，也不会连带删除其他评论。
- 删除后不能使用 `okr.+comment-reopen`（网关不提供：仅支持用户身份） 恢复；暂时关闭讨论应使用 `okr.+comment-solve`（网关不提供：仅支持用户身份）。
- 该命令不需要 style，因为接口没有返回 Comment 正文。

## 参考

- `okr` 领域指南 — OKR 命令、路由和通用约定
- OKR 实体定义（主题 `lark-okr-entities`） — Comment、评论串和状态规则
- `okr.+comment-get`（主题 `lark-okr-comment-get`） — 删除前核对评论
- `okr.+comment-solve`（网关不提供：仅支持用户身份） / `okr.+comment-reopen`（网关不提供：仅支持用户身份）（主题 `lark-okr-comment-solve-reopen`） — 暂时解决和恢复评论
