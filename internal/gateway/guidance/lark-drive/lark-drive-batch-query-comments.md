> 由 lark-cli `7f70de0666a2` 的 `lark-drive` 参考文档转换；调用约定见 `drive` 领域指南（`feishu_guide` 的 domain 参数）。

# `drive.+batch-query-comments`

按评论 ID 批量获取评论卡片。已知 comment_id 时用它精确取；要分页遍历、全量统计或找最新/最早评论，用 主题 `lark-drive-list-comments`（主题 `lark-drive-list-comments`）。

## 命令

```text
# 推荐：完整 URL + 评论 ID（逗号分隔或重复 --comment-ids，单次上限 100）
drive.+batch-query-comments {"url": "https://example.larksuite.com/docx/<DOCX_TOKEN>", "comment_ids": ["<id1>", "<id2>"]}
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `url` | 与 `token` 二选一 | 推荐入口。支持 doc/docx/sheet/file/slides/base/bitable/apps/wiki URL；apps 妙搭 URL 使用 `/page/<token>`；wiki URL 会自动解析到真实文档。 |
| `token` | 与 `url` 二选一 | 裸 token 或 URL。裸 token 必须搭配 `type`；wiki token 使用 `type=wiki`。 |
| `type` | 裸 token 时必填 | 传 token 对应类型：`doc`、`docx`、`sheet`、`file`、`slides`、`bitable`、`base`、`apps`、`wiki`。wiki token 使用 `wiki`；传 `base` 时，CLI 会按 `bitable` 类型处理。 |
| `comment_ids` | 是 | 评论 ID，逗号分隔或重复传，单次最多 100 个；来自 `drive.+list-comments` 的 `items[].comment_id` |
| `need_reaction` | 否 | 返回评论卡片上的 reaction 数据，见 主题 `lark-drive-reactions`（主题 `lark-drive-reactions`） |
| `need_relation` | 否 | docx 评论定位关系；仅 docx 生效，非 docx 静默忽略，见 主题 `lark-drive-comment-location`（主题 `lark-drive-comment-location`） |

## 行为说明

- `need_relation` 通过请求 **body** 发送（`drive.+list-comments` 是 query param），只在解析后的目标是 docx 时发送；该参数未收录于平台 metadata，但服务端支持，返回 `items[].relation` 及块位置。
- 输出的 `items` 始终是 JSON 数组（服务端省略时归一化为 `[]`），外层补 `file_token`、`file_type`、`count`。

## 输出

```json
{
  "file_token": "docx_token",
  "file_type": "docx",
  "items": [],
  "count": 0
}
```

`items` 是命中的评论卡片数组（外层补 `file_token`/`file_type`，wiki 输入再加 `wiki_token`）；`count` 是命中数。

## 参考

- lark-drive-list-comments（主题 `lark-drive-list-comments`） -- 分页获取评论列表
- lark-drive-comment-location（主题 `lark-drive-comment-location`） -- `need_relation` 评论定位
