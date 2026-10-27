> 由 lark-cli `7f70de0666a2` 的 `lark-contact` 参考文档转换；调用约定见 `contact` 领域指南（`feishu_guide` 的 domain 参数）。

# `contact.+get-user`

按 ID 取用户基本信息(姓名等)。

```text
# 取自己

# bot 按 ID 取他人
contact.+get-user {"user_id": "ou_xxx"}

# 按 union_id / user_id 取(默认 open_id)
contact.+get-user {"user_id": "<id>", "user_id_type": "union_id"}
```

## 注意事项

- **user 身份按 ID 取他人请用 `contact.+search-user`（网关不提供：仅支持用户身份）**,字段比本命令多(部门 / 邮箱 / 是否激活等)。本命令的 user 模式只回很少字段。
- **应用身份 必须传 `user_id`**:不传会直接报错(只有 user 身份能省略 `user_id` 取自己)。
