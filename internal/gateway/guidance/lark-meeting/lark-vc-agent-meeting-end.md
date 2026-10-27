> 由 lark-cli `7f70de0666a2` 的 `lark-meeting` 参考文档转换；调用约定见 `vc` 领域指南（`feishu_guide` 的 domain 参数）。

# `vc.+meeting-end`

当前 Host 应用 Bot 结束会议。

```text
vc.+meeting-end {"meeting_id": "7628568141510692381"}
vc.+meeting-end {"meeting_id": "7628568141510692381"}
```

正常执行必须显式传入 管理员预授权（high_risk_operations）；dry-run（网关不提供，改为先读取核对） 不会结束会议。

## 参数

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `meeting_id` | 是 | 长数字 Meeting ID，不是 9 位会议号。 |

仅支持应用身份，调用 `POST /open-apis/vc/v1/bots/end`；仅当前 Host Bot 可结束进行中的会议。

所需应用 Scope：`vc:meeting.bot.manage:write`。

## 常见失败原因

- 当前应用 Bot 不在会议中：先使用同一应用 Bot 发起或加入该 Calendar 会议，再执行结束。
- 应用 Bot 在会中但不是当前 Host：将 Host 转交给该 Bot，或由当前 Host/Owner 结束会议。
- 会议未启用 Agent 会议能力：确认会议设置及会议 Owner 的必要灰度开关。
