> 由 lark-cli `7f70de0666a2` 的 `lark-drive` 参考文档转换；调用约定见 `drive` 领域指南（`feishu_guide` 的 domain 参数）。

# `drive.+version-history`

## 命令

```text
drive.+version-history {"file_token": "boxcnxxxxxxxx"}

drive.+version-history {"file_token": "boxcnxxxxxxxx", "limit": 50, "cursor": "1777013761763"}

drive.+version-history {"file_token": "boxcnxxxxxxxx"}
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `file_token` | 是 | 目标文件 token |
| `limit` | 否 | 返回条数上限，范围 `1-200`，默认 `20` |
| `cursor` | 否 | 分页游标；取上一页返回的 `next_cursor` 回填 |

## 关键行为

- shortcut 内部固定传 `only_tag=true`
- 返回 `has_more=true` 时，使用 `next_cursor` 继续翻页
- `versions[].version` 是传给 `drive.+version-get` / `drive.+version-revert` / `drive.+version-delete` 的长数字版本串；`tag` 只是展示序号，不能替代 `version`
- `versions[].is_deleted` 为布尔值，表示该历史版本是否已被删除

## 返回值

```json
{
  "ok": true,
  "identity": "bot",
  "data": {
    "versions": [
      {
        "version": "7633658129540910621",
        "name": "report.md",
        "edited_at": "1777013761763",
        "edited_by": "ou_xxx",
        "size_bytes": "12345",
        "action_type": "upload",
        "is_deleted": false,
        "tag": 7
      }
    ],
    "has_more": true,
    "next_cursor": "1777013761763"
  }
}
```

## 参考

- `drive` 领域指南 -- 云空间（云盘/云存储）全部命令
