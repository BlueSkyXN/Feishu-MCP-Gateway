> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# Group Chat Identity Rules

> Warning: The most common source of failure in group operations is choosing the wrong identity. Confirm the identity before performing the action.

## Basic Principles

- **If the user does not specify an identity:** infer the correct identity from context instead of relying on the default.

## Identity Selection by Operation

| Operation | Recommended Identity | Why |
|------|---------|-----------------------------------|
| Create group (`im.+chat-create`) | Depends on the scenario | Infer from context |
| Update group (`im.+chat-update`) | Owner identity | Permission changes require owner/admin privileges; owner transfer requires owner identity |

## Inferring the Owner

When an owner-level action is needed and the owner is unknown, infer in this order:

4. Still unclear -> ask the user to confirm who owns the group before making owner-level changes

### When the Owner Is Neither the Current User Nor the Bot

If the query shows that the owner is a third-party user (`owner_id` is neither the currently authorized user nor the bot), the current identity does not have owner privileges. In that case:

- Explain the limitation clearly to the user instead of retrying blindly.

## Common Pitfalls

### Inviting Members During Group Creation

If a bot creates a group and `users` includes users who are mutually invisible to the bot, the entire request fails with 232043. Use two steps instead:

1. Create the group with the bot first, excluding invisible users: `im.+chat-create`（name="Group Name"）
2. Add users later with a user-identity member-management flow

### Insufficient Privileges

- **232016 / 232002 / 232017:** the current identity is not the owner or an admin -> switch to the owner identity
- **232011:** the current user is not in the group -> use a group-member identity, or join the group first

## References

- `im` 领域指南 - all IM commands
