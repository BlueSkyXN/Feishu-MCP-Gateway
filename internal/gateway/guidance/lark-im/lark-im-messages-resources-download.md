> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# `im.+messages-resources-download`

Download an image or file attached to a message. Use the `message_id` and resource key returned by a message-reading command; do not guess or combine identifiers from different messages.

> **Note:** read-only message commands render resource keys in message content, but they do not download binaries automatically. Use this command whenever you need to fetch the actual image/file bytes or save them to a specific path.

Shortcut: `im.+messages-resources-download`.

## Commands

```text
# Download an image (save to the current directory)
im.+messages-resources-download {"message_id": "om_xxx", "file_key": "img_v3_xxx", "type": "image"}

# Download a file
im.+messages-resources-download {"message_id": "om_xxx", "file_key": "file_v3_xxx", "type": "file"}

# Specify the output path
im.+messages-resources-download {"message_id": "om_xxx", "file_key": "img_v3_xxx", "type": "image", "output": "./photo.png"}

# Download as a bot
im.+messages-resources-download {"message_id": "om_xxx", "file_key": "img_v3_xxx", "type": "image"}

# Preview the request without executing it
# 网关不提供 dry-run 预览，此示例省略
```

## Parameters

| Parameter | Required | Description |
|------|------|------|
| `message_id=<id>` | Yes | Message ID (`om_xxx` format) |
| `file_key=<key>` | Yes | Resource key (`img_xxx` or `file_xxx`) |
| `type=<type>` | Yes | Resource type: `image` or `file` |
| `output=<path>` | No | Output path, relative or absolute, that must resolve inside the built-in allowed roots (the working directory, `/tmp`, `~/files`); system and credential directories stay refused. When omitted, the command uses the attachment name when available and otherwise falls back to the resource key |

## Choose `type`

Different resource markers in message content correspond to different `file_key` and `type` values:

| Message Type | Marker in Content | `file_key` Format | `type` |
|---------|-------------|---------------|--------|
| Image | `img_xxx` | `img_xxx` | `image` |
| File | `file_xxx` | `file_xxx` | `file` |
| Audio | `file_xxx` | `file_xxx` | `file` |
| Video | `file_xxx` | `file_xxx` | `file` |

Stickers cannot be downloaded with this command.

A folder itself cannot be downloaded: expand it with `im.files.folder` (params={"recursive": true}) first (see `im` domain指南), then download the files it contains.

## Output

On success, read:

| Field | Meaning |
|------|---------|
| `data.saved_path` | Saved local path |
| `data.size_bytes` | Saved byte count |

## Usage Scenario

### Scenario: Extract and download an image from a message

```text
# Step 1: Fetch messages and find one containing an image
im.+chat-messages-list {"chat_id": "oc_xxx"}
# In the response you see: { "msg_type": "image", "content": "{\"image_key\":\"img_v3_xxx\"}" }

# Step 2: Download the image
im.+messages-resources-download {"message_id": "om_xxx", "file_key": "img_v3_xxx", "type": "image"}
```

## Common Errors and Troubleshooting

| Symptom | Root Cause | Solution |
|---------|---------|---------|
| Resource does not match the message | `file_key` and `message_id` came from different messages | Read the message again and use its matching identifiers |
| Attachment unavailable | The message or resource is deleted, hidden, restricted, or inaccessible to the caller | Do not retry unchanged; report the exact CLI error |
| Retryable network error | The transfer did not complete | Retry the same command |

## References

- `im` domain指南 - all message-related commands
