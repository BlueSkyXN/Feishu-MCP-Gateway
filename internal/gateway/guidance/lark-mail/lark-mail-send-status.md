> 由 lark-cli `7f70de0666a2` 的 `lark-mail` 参考文档转换；调用约定见 `mail` 领域指南（`feishu_guide` 的 domain 参数）。

# 发送投递状态

发送后确认投递状态，处理发送拦截。命令选择见 `../SKILL.md`（`mail` 领域指南） 的“命令选择”章节。

## 查询时机

- 立即发送：发送成功并返回非空 `message_id` 后立即查询。
- 定时发送：不要立即查询；等预定发送时间后，再使用发送产生的 `message_id` 查询投递状态。

## 立即发送

邮件发送成功后，若响应中包含非空 `message_id`，必须调用 `send_status` 查询投递状态并向用户报告。

```text
mail.user_mailbox.messages.send_status  # 网关不提供：仅支持用户身份
```

返回每个收件人的投递状态（`status`）：

| status | 含义 |
|--------|------|
| 1 | 正在投递 |
| 2 | 投递失败重试 |
| 3 | 退信 |
| 4 | 投递成功 |
| 5 | 待审批 |
| 6 | 审批拒绝 |

向用户简要报告结果；如有退信、审批拒绝等异常状态，需要重点提示。

## 发送被拦截

若发送响应中包含 `automation_send_disable_reason` / `automation_send_disable_reference`，说明邮件未真正发出，而是被邮箱设置拦截。

- 直接向用户展示拦截原因和草稿打开链接。
- 不要继续假设已经发送成功。
- 不要调用 `send_status`。

## 相关命令

- `mail.+send`（网关不提供：仅支持用户身份） — 发送新邮件。
- `mail.+reply`（网关不提供：仅支持用户身份） / `mail.+reply-all`（网关不提供：仅支持用户身份） — 发送回复。
- `mail.+forward`（网关不提供：仅支持用户身份） — 发送转发。
