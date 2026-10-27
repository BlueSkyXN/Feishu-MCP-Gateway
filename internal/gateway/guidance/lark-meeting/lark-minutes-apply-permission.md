> 由 lark-cli `7f70de0666a2` 的 `lark-meeting` 参考文档转换；调用约定见 `vc` 领域指南（`feishu_guide` 的 domain 参数）。

# `minutes.+apply-permission`

向妙记所有者发起查看或编辑权限申请。**写操作**，只在用户明确要求申请权限时才调用；调用后不代表立即获得权限，只是提交了一条申请。

## 命令

```text

# 以 bot 身份申请编辑权限
minutes.+apply-permission {"minute_token": "obcnxxxxxxxxxxxxxxxxxxxx", "perm": "edit"}

# 预览 API 调用
minutes.+apply-permission {"minute_token": "obcnxxxxxxxxxxxxxxxxxxxx", "perm": "view"}
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--minute-token <token>` | 是 | 妙记 Token |
| `--perm <view\|edit>` | 是 | 申请的权限：`view`（查看）或 `edit`（编辑） |

## user / bot 身份与权限语义

- **bot**：以应用身份向妙记所有者申请，代表"这个应用"而不是某个用户。同意后应用（bot）获得对应权限，不会让触发申请的用户本人获得权限。
- 两种身份的申请互不代表：user 身份申请通过后 bot 仍然无权限，反之亦然。

## 核心约束

### 2. missing scope 与资源 ACL 是两类不同问题

- **资源 ACL**（scope 都有，但对**这一条具体妙记**没有查看/编辑权限）：这才是 `+apply-permission` 要解决的场景。

先看错误的 `error.subtype` 是 `missing_scope` 还是资源级别的权限拒绝，再决定要不要调用本命令。

### 3. 只有用户明确要求才发起申请

遇到无权限错误时，先把"当前身份对这条妙记没有权限"的事实告知用户；只有用户明确说"帮我申请查看/编辑权限"时才调用本命令。不要在检测到无权限后自动发起申请。

## 所需权限

| 身份 | 所需权限 |
|------|---------|
| user / bot | `minutes:permission:apply` |

## 输出结果

```json
{
  "minute_token": "obcnxxxxxxxxxxxxxxxxxxxx",
  "perm": "view"
}
```

| 字段 | 说明 |
|------|------|
| `minute_token` | 妙记 Token |
| `perm` | 申请的权限（`view` / `edit`） |

## 如何获取 minute_token

| 来源 | 获取方式 |
|------|---------|
| 妙记 URL | 从 URL 末尾提取，如 `https://sample.feishu.cn/minutes/obcnxxxxxxxxxxxxxxxxxxxx` |
| 妙记搜索 | `minutes.+search`（query="关键词"） |

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--perm` 不是 `view`/`edit` | 参数值不合法 | 只能传 `view` 或 `edit` |
| `missing required scope(s)` | 当前身份缺少 `minutes:permission:apply` | 见上方「missing scope 与资源 ACL」 |
| 申请后仍无权限 | 所有者尚未同意 | 这是异步申请，需等待所有者处理；不代表命令执行失败 |

## 相关场景
- 生成和修改妙记（主题 `create-and-edit-minutes`）
