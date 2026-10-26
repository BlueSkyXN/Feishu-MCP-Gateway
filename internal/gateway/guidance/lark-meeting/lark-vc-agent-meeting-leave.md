> 由 lark-cli `7f70de0666a2` 的 `lark-meeting` 参考文档转换；调用约定见 `vc` 领域指南（`feishu_guide` 的 domain 参数）。

# `vc.+meeting-leave`

通过 `meeting_id` 离开当前身份所在的视频会议（bot leave）。这是一次**写操作**，会实际把当前身份从会议中移出。

本 skill 对应 shortcut：`vc.+meeting-leave`（调用 `POST /open-apis/vc/v1/bots/leave`）。

## 命令

```bash
# 通过 meeting_id 离会
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `meeting_id=<id>` | 是 | 会议 ID（**不是 9 位会议号**） |
| dry-run（网关不提供，改为先读取核对） | 否 | 预览 API 调用，不实际离会；meeting_id 或身份不确定时先用它确认请求 |

## 核心约束

### 3. 当前身份必须在会议中

应用机器人必须已经在该会议中，否则接口会报错。如果 `meeting_id` 来自 `vc.+meeting-list-active`，必须确认这是应用身份发现到的会议。

### 4. 离会立即生效，对其他参会人可见

机器人会立刻从参会列表消失；若会议启用了录制/纪要，bot 的参会时段到此截止。只有在用户明确要求退出 / 离开 / 结束参会时才调用；如需要重新入会，再跑 `vc.+meeting-join` 即可（非真正"不可逆"）。

## 输出结果

接口成功返回时，默认输出：`Left meeting <meeting-id> successfully.`。
 返回标准 `{ok, identity, data}` 信封，例如 `{"ok":true,"identity":"bot","data":{}}`，不是带 `code` / `msg` 的 API 原始响应体。

## 如何获取输入参数

| 输入参数 | 获取方式 |
|---------|---------|

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `meeting not found` / `invalid meeting_id` | 误传了 9 位会议号 | 必须使用 `meeting.id`，不是会议号 |
| `not in meeting` | 当前身份并不在该会议中 | 确认先 `vc.+meeting-join` 成功 |

## 提示

- 只有用户明确要求退出 / 离开 / 结束参会时才调用；离会会让机器人从参会列表消失，对其他参会人可见。若需要重新入会直接再 `vc.+meeting-join`，不是真正的"不可逆"。参数格式不确定时可选 dry-run（网关不提供，改为先读取核对） 预览。
- `meeting_id` 如果来自 `vc.+meeting-list-active`，必须来自应用身份，并确认应用机器人就在该会议中。不要用 9 位会议号。

## 相关场景
- 应用机器人参会与会中互动（主题 `live-meeting-attend`）
