> 由 lark-cli `7f70de0666a2` 的 `lark-mail` 参考文档转换；调用约定见 `mail` 领域指南（`feishu_guide` 的 domain 参数）。

# `mail.+signature` (not available in the gateway: user identity only)

查看邮箱签名列表或详情。返回签名的类型、默认使用情况、内容预览等信息。TENANT（企业）签名的模板变量会被自动替换为实际值。

本 skill 对应 shortcut：`mail.+signature`（网关不提供：仅支持用户身份）。

## 命令

```text
# 列出所有签名
mail.+signature  # 网关不提供：仅支持用户身份

# 查看某个签名的详情（渲染后的内容预览、模板变量值、图片信息）
mail.+signature  # 网关不提供：仅支持用户身份

# 指定邮箱
mail.+signature  # 网关不提供：仅支持用户身份
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--from <email>` | 否 | 邮箱地址（默认 `me`） |
| `--detail <id>` | 否 | 签名 ID，查看详情。省略则列出所有签名 |

## 返回值

**列表模式：**

```json
{
  "ok": true,
  "data": {
    "signatures": [
      {
        "id": "<签名ID>",
        "name": "个人签名",
        "type": "USER",
        "content_preview": "这是我的签名内容 [image] 超链接哈哈"
      },
      {
        "id": "<签名ID>",
        "name": "企业签名",
        "type": "TENANT",
        "is_send_default": true,
        "is_reply_default": true,
        "content_preview": "企业签名 姓名：陈煌 部门：研发团队"
      }
    ]
  }
}
```

**详情模式（`--detail`）：**

```json
{
  "ok": true,
  "data": {
    "id": "<签名ID>",
    "name": "企业签名",
    "type": "TENANT",
    "is_send_default": true,
    "is_reply_default": true,
    "images": [
      {"cid": "76CEB29E-...", "file_key": "121011...", "image_name": "image.png"}
    ],
    "template_vars": {"B-NAME": "陈煌", "B-DEPARTMENT": "研发团队"},
    "content_preview": "企业签名 姓名：陈煌 部门：研发团队"
  }
}
```

## 字段说明

| 字段 | 说明 |
|------|------|
| `type` | `USER`（用户签名，可编辑）或 `TENANT`（企业签名，管理员模板控制） |
| `is_send_default` | 是否为新邮件的默认签名 |
| `is_reply_default` | 是否为回复/转发的默认签名 |
| `images` | 签名内联图片元数据（仅详情模式） |
| `template_vars` | TENANT 签名的模板变量已替换值（仅详情模式） |
| `content_preview` | 签名内容的纯文本预览（`<img>` 显示为 `[image]`，最长 200 字符） |

## 与 compose shortcut 配合

获取签名 ID 后，可在发送/回复/转发时附加签名：

```text
# 查看签名列表获取 ID
mail.+signature  # 网关不提供：仅支持用户身份

# 在发送邮件时附加签名
mail.+send  # 网关不提供：仅支持用户身份
```
