> 由 Feishu-MCP-Gateway 从 lark-cli `7f70de0666a2` 的 `lark-contact` Skill 自动转换。
> - `docs.+fetch` 这类标识是 operation_id：具名视图调用对应工具（如 `feishu_docs_fetch`），紧凑视图用 `feishu_execute_read` / `feishu_execute_write` 传 operation_id 和 arguments。
> - CLI 参数 `--foo-bar` 对应 JSON 参数 `foo_bar`；代码块里的 `operation_id {…}` 行是参数示例。
> - 文中的“主题 `x`”用 `feishu_guide`（topic="x"）读取。
> - 网关固定以应用（bot）身份执行，没有 dry-run；高风险操作需要管理员预授权。
> - 本地文件改用 `{"file_id": "…"}`（先 `feishu_file_create` 或文件传输），输出路径填受控工作区内的相对名称；文中的本地脚本、jq、stdin 在网关中不可用。
> - 标注“网关不提供”的操作不能调用；参数以 `feishu_tools_describe` 或具名工具 schema 为准。

## 选哪个命令

**user 身份和 bot 身份是两条完全独立的路径**。先确定当前身份,再按下表选命令:

| 想做什么 | user 身份 | bot 身份 |
|---|---|---|
| 按姓名 / 邮箱搜员工拿 open_id | `contact.+search-user`（网关不提供：仅支持用户身份）（主题 `lark-contact-search-user`） | 不支持 |
| 按关键词搜索当前用户可见的机器人 / 智能体 | `contact.+search-bot`（网关不提供：仅支持用户身份）（主题 `lark-contact-search-bot`） | 不支持 |
| 已知 open_id 取他人资料 | `contact.+search-user`（网关不提供：仅支持用户身份） | `contact.+get-user`（user_id="<id>"）（主题 `lark-contact-get-user`） |
| 查看自己 | `contact.+get-user` 或 `contact.+search-user`（网关不提供：仅支持用户身份） | 不支持 |
| 查同事的个人状态 / 签名 | `user_profiles batch_query` | 不支持 |

已知 open_id 只是想发消息 / 排日程,不必经过 contact —— 直接 `im` 领域指南 / `calendar` 领域指南。

### 名字没说清是人还是机器人 / 智能体

用户给的名字常常不表明类型。例如「和 reviewDuck 约个会」里的 reviewDuck 可能是同事昵称,也可能是机器人。
- 名字含 bot / agent / AI / 助手 / 机器人 / 智能体 / assistant 等明显特征时,反过来先搜机器人更快
- 不确定的话两边都搜一下

## 典型场景

找张三给他发消息:先搜,确认 open_id,再发:

```text
im.+messages-send {"user_id": "ou_xxx", "text": "Hi!"}
```

批量查同事的个人状态 / 个性签名(先用 schema 看参数)。

```text
# contact.user_profiles.batch_query（网关不提供：仅支持用户身份）
```

搜索命中多条且后续操作有副作用(发消息、邀请会议等),把候选列给用户挑;不要擅自选第一条。

## 搜索机器人 / 智能体

`contact.+search-bot`（网关不提供：仅支持用户身份） 使用 user 身份按关键词搜索当前用户可见的机器人,返回 `ou_` 开头的机器人 open_id。参数细节等见 `lark-contact-search-bot.md`（主题 `lark-contact-search-bot`）。

## 注意事项

- **跨租户用户**(`is_cross_tenant=true`)多数业务字段为空字符串,这是飞书可见性规则,下游做空值兜底。
- **ID 类型**:`contact.+get-user` 可通过 `user_id_type` 使用 `open_id`、`union_id` 或 `user_id`;`contact.+search-user`（网关不提供：仅支持用户身份） 使用用户 open_id;`contact.+search-bot`（网关不提供：仅支持用户身份） 不支持按 ID 查询,它按关键词搜索并返回机器人 open_id。

## 不在本 skill 范围

- 发消息 / 查聊天记录 → `im` 领域指南
- 排日程 / 邀请会议 → `calendar` 领域指南
- 部门树 / 按部门列员工 / 组织架构 → `lark-openapi-explorer`（CLI 专用指南，网关不提供） 查找原生接口
