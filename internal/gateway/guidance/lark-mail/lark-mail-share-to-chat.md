> 由 lark-cli `7f70de0666a2` 的 `lark-mail` 参考文档转换；调用约定见 `mail` 领域指南（`feishu_guide` 的 domain 参数）。

# `mail.+share-to-chat` (not available in the gateway: user identity only)

将邮件以卡片形式分享到飞书 IM 会话（群聊或个人对话）。内部两步完成：创建分享凭证 → 发送卡片到 IM。

**依赖 Scope：** `mail:user_mailbox.message:readonly`、`im:message`、`im:message.send_as_user`

## 命令

```text
# 分享单封邮件到群聊（默认 receive-id-type=chat_id）
mail.+share-to-chat  # 网关不提供：仅支持用户身份

# 分享整个会话到群聊
mail.+share-to-chat  # 网关不提供：仅支持用户身份

# 通过邮箱分享给个人
mail.+share-to-chat  # 网关不提供：仅支持用户身份

# Dry Run
mail.+share-to-chat  # 网关不提供：仅支持用户身份
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `message_id=<id>` | 否（二选一） | 要分享的邮件 ID，与 `thread_id` 互斥 |
| `thread_id=<id>` | 否（二选一） | 要分享的邮件会话 ID，与 `message_id` 互斥 |
| `--receive-id <id>` | 是 | 目标接收者 ID，类型由 `--receive-id-type` 决定 |
| `--receive-id-type <type>` | 否 | 接收者 ID 类型（默认 `chat_id`）。可选：`chat_id` / `open_id` / `user_id` / `union_id` / `email` |
| `mailbox=<email>` | 否 | 邮箱地址（默认 `me`） |

## 返回值

```json
{
  "ok": true,
  "data": {
    "card_id": "550e8400-e29b-41d4-a716-446655440000",
    "im_message_id": "om_dc13264520392913993dd051dba21dcf"
  }
}
```

## 典型场景

### 场景 1：用户说"帮我把这封邮件分享到项目群"

```text
# Step 1: 搜索群聊获取 chat_id
im.+chat-search {"query": "项目群"}

# Step 2: 分享邮件
mail.+share-to-chat  # 网关不提供：仅支持用户身份
```

### 场景 2：分享整个邮件会话

```text
mail.+share-to-chat  # 网关不提供：仅支持用户身份
```

### 场景 3：通过邮箱分享给个人

```text
mail.+share-to-chat  # 网关不提供：仅支持用户身份
```

## 常见错误

| 症状 | 原因 | 解决 |
|------|------|------|
| `either message_id or thread_id is required` | 两个参数都未传 | 传入其中一个 |
| `message_id and thread_id are mutually exclusive` | 两个参数同时传 | 只传一个 |
| 403 `user not in chat` | 用户不在目标会话中 | 确认用户是群成员 |
| 404 `message not found` | 邮件 ID 无效 | 确认邮件 ID 正确 |

## 相关命令

- `im.+chat-search` — 搜索群聊获取 chat_id
- `mail.+message` — 查看邮件内容
- `mail.+thread` — 查看邮件会话
