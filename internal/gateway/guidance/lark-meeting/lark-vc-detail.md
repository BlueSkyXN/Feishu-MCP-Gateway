> 由 lark-cli `7f70de0666a2` 的 `lark-meeting` 参考文档转换；调用约定见 `vc` 领域指南（`feishu_guide` 的 domain 参数）。

# `vc.+detail`

## 命令

```text
# 单个 / 批量（逗号分隔，最多 50 个）
vc.+detail {"meeting_ids": "<meeting_id1>,<meeting_id2>"}

# 应用身份（只能查应用有权限的会议）
vc.+detail {"meeting_ids": "<meeting_id1>,<meeting_id2>"}
```

## 输出字段

| 字段 | 说明 |
|------|------|
| `meeting_id` | 会议 ID |
| `meeting_no` | 会议 9 位号码 |
| `topic` | 会议主题 |
| `start_time` | 开始时间 |
| `end_time` | 结束时间 |
| `note_id` | 关联的纪要 ID。 |
| `minute_token` | 关联的妙记 Token。 |

## 相关场景
- 查询会议及其产物（主题 `query-meeting-and-artifacts`）
