> 由 lark-cli `7f70de0666a2` 的 `lark-mail` 参考文档转换；调用约定见 `mail` 领域指南（`feishu_guide` 的 domain 参数）。

# mail send_as

使用公共邮箱或别名发信。适用于 `mail.+send`（网关不提供：仅支持用户身份） / `mail.+draft-create`（网关不提供：仅支持用户身份） / `mail.+reply`（网关不提供：仅支持用户身份） / `mail.+reply-all`（网关不提供：仅支持用户身份） / `mail.+forward`（网关不提供：仅支持用户身份） 等发信类 shortcut。

## 参数含义

- `mailbox` 指定邮件归属邮箱（如 `shared@example.com` 或 `me`），可通过 `accessible_mailboxes` 查询可用值。
- `--from` 指定 EML From 头里的发件人地址（别名、邮件组等），可通过 `send_as` 查询可用值。
- 不使用公共邮箱或别名时无需指定 `mailbox`，行为与默认发信一致。

## 查询可用邮箱和发信地址

```text
# 查询可访问的邮箱（主邮箱 + 公共邮箱）
mail.user_mailboxes.accessible_mailboxes {"params": {"user_mailbox_id": "me"}}

# 查询某个邮箱的可用发信地址（主地址、别名、邮件组）
mail.user_mailbox.settings.send_as {"params": {"user_mailbox_id": "me"}}
```

## 公共邮箱发信

```text
# --mailbox 指定公共邮箱，From 头自动使用该邮箱地址
mail.+send  # 网关不提供：仅支持用户身份
```

## 别名发信

```text
# --mailbox 指定所属邮箱，--from 指定别名地址
mail.+send  # 网关不提供：仅支持用户身份
```

## 相关命令

- `mail.+send`（网关不提供：仅支持用户身份） — 新邮件发信。
- `mail.+draft-create`（网关不提供：仅支持用户身份） — 新建草稿。
- `mail.+reply`（网关不提供：仅支持用户身份） / `mail.+reply-all`（网关不提供：仅支持用户身份） — 回复邮件。
- `mail.+forward`（网关不提供：仅支持用户身份） — 转发邮件。
