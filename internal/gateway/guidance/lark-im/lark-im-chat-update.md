> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+chat-update`

This skill maps to the shortcut: `im.+chat-update` (internally calls `PUT /open-apis/im/v1/chats/:chat_id`).

## Commands

```text
# Update the group name
im.+chat-update {"chat_id": "oc_xxx", "name": "New Group Name"}

# Update the group description
im.+chat-update {"chat_id": "oc_xxx", "description": "Updated group description"}

# Update multiple fields at once
im.+chat-update {"chat_id": "oc_xxx", "name": "Q2 Project Team", "description": "Owns Q2 goal tracking"}

# Preview the request without executing it
im.+chat-update {"chat_id": "oc_xxx", "name": "Test"}
```

## Parameters

### Required

| Parameter | Description |
|------|------|
| `chat_id=<oc_xxx>` | Group ID |

### Optional Fields

| Parameter | Limits | Description |
|------|------|------|
| `name=<name>` | Max 60 characters | Group name |
| `description=<text>` | Max 100 characters | Group description |

## Usage Scenarios

### Scenario 1: Rename a group and update its description

```text
im.+chat-update {"chat_id": "oc_xxx", "name": "Q2 Project Team", "description": "Owns Q2 goal tracking"}
```

## Common Errors and Troubleshooting

| Symptom | Root Cause | Solution |
|---------|---------|---------|
| `invalid chat_id: expected chat ID (oc_xxx)` | Invalid chat_id format | Use a valid `oc_xxx` chat ID |
| `name exceeds the maximum of 60 characters` | Group name too long | Shorten the name to 60 characters or fewer |
| `description exceeds the maximum of 100 characters` | Group description too long | Shorten the description to 100 characters or fewer |
| `at least one field must be specified to update` | No update field was provided | Specify at least one field to update |
| Not in the group (232011) | The current user is not a member of the group | Use a member identity (app identity) or join the group first |

## AI Usage Guidance

### Identity Selection

Infer the group owner from context whenever possible (for example, if a bot just created the group, the owner is the bot) and use the matching identity directly. If ownership is unclear, query the group first and confirm `owner_id`.

Identity choice should follow Group Chat Identity Rules (topic `lark-im-chat-identity`): if the user explicitly specifies an identity, use it directly; otherwise infer the owner identity from context.

## References

- `im` domain指南 - all IM commands
