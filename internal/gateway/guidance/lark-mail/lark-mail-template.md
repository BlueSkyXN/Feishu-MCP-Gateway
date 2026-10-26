> 由 lark-cli `7f70de0666a2` 的 `lark-mail` 参考文档转换；调用约定见 `mail` 领域指南（`feishu_guide` 的 domain 参数）。

# mail templates

邮件模板指飞书 OAPI 的个人邮件模板系统（用户邮箱里的"我的模板"），可在飞书客户端管理。它不同于仓库 `../assets/templates/` 下的静态 HTML 模板库；静态 HTML 模板只是在写单封邮件时可复制参考的本地素材。

## 何时使用

- 创建 / 更新长期复用的邮件框架：用 `mail.+template-create` / `mail.+template-update`。
- 使用已有模板发信：在 `mail.+send`（网关不提供：仅支持用户身份） / `mail.+draft-create`（网关不提供：仅支持用户身份） / `mail.+reply`（网关不提供：仅支持用户身份） / `mail.+reply-all`（网关不提供：仅支持用户身份） / `mail.+forward`（网关不提供：仅支持用户身份） 中传 `template_id=<id>`。
- 列表 / 获取 / 删除个人模板：走原生 API `user_mailbox.templates {list|get|delete}`。

## 管理模板

- `mail.+template-create`（主题 `lark-mail-template-create`） — 创建新模板。`name` 必填；正文通过 `template_content` 或 `template_content_file` 二选一；支持 HTML 内嵌图片自动上传到 Drive。
- `mail.+template-update`（主题 `lark-mail-template-update`） — 全量替换式更新（**后端无乐观锁，last-write-wins**）。支持 `inspect`（只读 projection）/ `print_patch_template`（patch 骨架）/ `patch_file`（结构化 patch）/ 扁平 `--set-*` flag。

## 套用模板

`mail.+send`（网关不提供：仅支持用户身份） / `mail.+draft-create`（网关不提供：仅支持用户身份） / `mail.+reply`（网关不提供：仅支持用户身份） / `mail.+reply-all`（网关不提供：仅支持用户身份） / `mail.+forward`（网关不提供：仅支持用户身份） 均支持 `template_id=<id>`。`template_id` 必须是**十进制整数字符串**。

### 创建模板后立即发信 checklist

3. 返回 `message_id` 后调用 `user_mailbox.messages send_status` 汇报投递状态。

## 合并规则

| # | 场景 | 合并策略 |
|---|------|----------|
| Q1 to/cc/bcc | 全部 5 个 shortcut | 用户 `to/cc/bcc` 先覆盖草稿原有值，再与模板 tos/ccs/bccs **无去重追加** |
| Q2 subject | `mail.+send`（网关不提供：仅支持用户身份） / `mail.+draft-create`（网关不提供：仅支持用户身份） | 用户 `subject` > 草稿 subject > 模板 subject |
|  | `mail.+reply`（网关不提供：仅支持用户身份） / `mail.+reply-all`（网关不提供：仅支持用户身份） / `mail.+forward`（网关不提供：仅支持用户身份） | 用户 `subject` 覆盖自动 Re:/Fw:；否则保持 Re:/Fw: + 原邮件 subject。**模板 subject 被忽略**（保留会话线索） |
| Q3 body | `mail.+send`（网关不提供：仅支持用户身份） / `mail.+draft-create`（网关不提供：仅支持用户身份） | 空草稿 body → 用模板；非空 HTML → `draftBody + <br><br> + tplContent`；非空 plain-text → `\n\n` 拼接 |
|  | `mail.+reply`（网关不提供：仅支持用户身份） / `mail.+reply-all`（网关不提供：仅支持用户身份） / `mail.+forward`（网关不提供：仅支持用户身份） | 模板内容注入 `<blockquote>` 之前；无 blockquote 则追加；plain-text 模板走 emlbuilder plain-text 追加 |
| Q4 附件 | 全部 5 个 shortcut | 模板 inline（SMALL）由 CLI 走 `user_mailbox.template.attachments.download_url` 下载后以 MIME part 注入；SMALL 非 inline 同样注入；LARGE（`attachment_type=2`）不下载，只把 `file_key` 放到 `X-Lms-Large-Attachment-Ids` header 让服务端渲染下载卡片 |
| Q5 cid 冲突 | inline 图片 | cid 由 UUID v4 生成（碰撞概率 ~ 2^-122），不显式检测 |

**Warning**：`mail.+reply`（网关不提供：仅支持用户身份） / `mail.+reply-all`（网关不提供：仅支持用户身份） + 模板且模板自带 tos/ccs/bccs 时，CLI 在 stderr 打印：`warning: template to/cc/bcc are appended without de-duplication; you may see repeated recipients. Use to/cc/bcc to override, or run +template-update to clear template addresses.`

## Size 约束

- 单模板 `template_content` <= 3 MB。
- `body + inline + SMALL` 累计 <= 25 MB。
- 超过 25 MB 后，该批次剩余非 inline 附件切换为 LARGE；inline 不能切换。

## 原生 API

```text
mail.user_mailbox.templates.list {"params": {"user_mailbox_id": "me"}}
mail.user_mailbox.templates.get {"params": {"user_mailbox_id": "me", "template_id": "<id>"}}
mail.user_mailbox.templates.delete {"params": {"user_mailbox_id": "me", "template_id": "<id>"}}
```
