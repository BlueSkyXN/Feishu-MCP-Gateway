> 由 lark-cli `7f70de0666a2` 的 `lark-drive` 参考文档转换；调用约定见 `drive` 领域指南（`feishu_guide` 的 domain 参数）。

# `drive.+version-revert`

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `file_token` | 是 | 目标文件 token |
| `version` | 是 | `drive.+version-history` 返回的长数字 `version` 字段，不是 `tag` |

## 返回值

无额外业务字段，以命令成功 / 失败为准。

## 参考

- `drive` 领域指南 -- 云空间（云盘/云存储）全部命令
