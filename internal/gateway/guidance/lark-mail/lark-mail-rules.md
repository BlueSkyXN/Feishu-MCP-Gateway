> 由 lark-cli `7f70de0666a2` 的 `lark-mail` 参考文档转换；调用约定见 `mail` 领域指南（`feishu_guide` 的 domain 参数）。

# 收信规则 Shortcut

管理自动处理收到邮件的规则。优先使用 `mail +rule-*` shortcut，通过稳定英文 alias 编写条件和动作；只有需要当前 shortcut 尚未建模的服务端字段时，才回退到 `mail user_mailbox.rules` 原子 raw 命令。规则写操作需使用真实 `rule_id`，不要猜测 ID。创建、更新、删除规则需要按 SKILL.md 的高风险写规则获得用户确认并传 管理员预授权（high_risk_operations）；启停和排序是普通写操作，免 管理员预授权（high_risk_operations）。

## 常用 shortcut

```bash

# 调整顺序：完整顺序或单条移动二选一
```

## Alias 速查

条件 grammar:

```text
--condition field:op:value
--condition field:op
--condition field
```

常用字段：`from`/`sender`、`to`/`recipient`、`cc`、`to_or_cc`、`subject`/`title`、`body`、`attachment_name`、`attachment_type`、`any_address`、`all_mail`/`all`、`external`、`spam`、`not_spam`、`has_attachment`。

常用操作符：`contains`/`include`、`not_contains`/`exclude`、`starts_with`/`prefix`、`ends_with`/`suffix`、`equals`/`eq`/`is`、`not_equals`/`ne`、`contains_self`/`self`、`empty`/`is_empty`。

动作 grammar:

```text
--action kind
--action kind:key=value
--action kind:json={"key":"value"}
```

常用动作：`archive`、`delete_mail`/`trash`、`mark_read`/`read`、`move_spam`/`spam`、`not_spam`/`never_spam`、`star`/`flag`、`mute_notification`/`mute`、`move_folder:folder_id=<id>`。

`conditions` / `actions` 支持 JSON 或 `{"file_id": …}`。JSON 示例：

```json
[
  {"field":"subject","operator":"contains","value":"Alpha"},
  {"field":"has_attachment"}
]
```

## Unknown raw 策略

- 读路径宽容：`mail.+rule-list` / `mail.+rule-get` 遇到未知枚举或扩展字段仍输出规则，`unknowns[]` 会说明无法识别的 raw 片段，`raw` 会保留原始规则。
- 更新规则：`mail.+rule-update` 是“传什么改什么”。只改名称、启停、match 或 stop-after-match 时保留未触碰的 raw；传入新的 `condition(s)` 时替换 condition items，未传 `match` 就保留当前 match_type；传入新的 `action(s)` 时替换 action items。
- 输入校验：用户输入 alias/语义字符串时必须能映射到当前 shortcut 支持的枚举，否则报错；用户直接输入当前 shortcut 不认识的枚举数字，也报错。
- raw fallback：需要写入当前 shortcut 尚未建模的服务端字段时，读取 `raw` 后使用原子 `user_mailbox.rules` 命令。

## 原子 raw fallback：主题包含文本 → 标记为已读

```bash

# 3. 删除规则
```

Quick codes above: condition `type=6` = subject, `operator=1` = contains, action `type=3` = mark as read.

## 原生 API

收信规则走 `user_mailbox.rules` 资源。参数不确定时先运行：

```text
# feishu_tools_describe
```
