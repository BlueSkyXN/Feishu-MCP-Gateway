> 由 lark-cli `7f70de0666a2` 的 `lark-meeting` 参考文档转换；调用约定见 `vc` 领域指南（`feishu_guide` 的 domain 参数）。

# `vc.+meeting-list-active`

列出当前进行中的会议，用来发现 `vc.+meeting-events` 需要的长数字 `meeting_id`。

本 skill 对应 shortcut：`vc.+meeting-list-active`（调用 `GET /open-apis/vc/v1/bots/user_active_meeting`）。

## 命令

```text

# 查询指定用户当前参加、且应用机器人也在会中的会议
vc.+meeting-list-active {"user_id": "ou_xxx"}
```

## 身份语义

| 身份 | 命令 | 返回范围 | 后续事件读取 |
| ---- | ---- | -------- | ------------ |
| 应用身份 | `user_id=<user_open_id>` | 目标用户正在参加、且应用机器人也在会中的会议 | 继续 `vc.+meeting-events` |

应用身份返回空，不代表目标用户不在任何会议中，只能说明没有找到“目标用户在会中且应用机器人也在会中”的当前会。

## 多会议选择

- 如果返回多个会议，不要自动挑第一个。
- 向用户展示每个候选的 `meeting_title` / `meeting_no` / `meeting_id`，等待用户选择。
- 选择后用同一身份执行 `vc.+meeting-events` 读取事件。

## 9 位会议号匹配

用户提供 9 位会议号但没有明确要求应用机器人入会时，把会议号当作 active meeting 的筛选条件，而不是写操作指令。

匹配规则：

- 在返回会议中匹配 `meeting_no == <9位会议号>`。
- 匹配到唯一会议：取该项的长数字 `meeting_id`，后续用同一身份调用 `vc.+meeting-events`。
- 匹配到多个会议：展示候选，让用户选择。
- 没有匹配：说明当前身份没有发现该会议号对应的 active meeting；不要自动调用 `vc.+meeting-join`，除非用户明确要求应用机器人入会。

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `user_id is required when` | 应用身份未传目标用户 | 传入目标用户 open_id |
| 应用身份返回空列表 | 没有满足“目标用户在会中且应用机器人也在会中”的当前会 | 先让应用机器人入会，或确认 `user_id` 和会议状态 |
| `user_id` 格式错误 | 传入了 internal user_id 或其他非 `ou_...` 值 | 改传目标用户 open_id |

## 相关场景
- 会中事件与会中互动（主题 `live-meeting-interact`）
- 应用机器人参会与会中互动（主题 `live-meeting-attend`）
