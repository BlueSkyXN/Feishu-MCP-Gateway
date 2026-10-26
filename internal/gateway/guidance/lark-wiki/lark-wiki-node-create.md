> 由 lark-cli `7f70de0666a2` 的 `lark-wiki` 参考文档转换；调用约定见 `wiki` 领域指南（`feishu_guide` 的 domain 参数）。

# `wiki.+node-create`

在飞书知识库中创建一个新节点，并自动解析目标知识空间。该 shortcut 对原生 `wiki.nodes.create` 做了一层更适合日常使用的封装：可以直接指定 `space_id`，也可以从父节点自动反查所属空间；在 `user` 身份下，如果同时省略 `space_id` 和 `parent_node_token`，还会自动回退到个人知识库 `my_library`。

## 命令

```text
# 在个人知识库根目录下创建一个 docx 节点（user 身份默认回退到 my_library）
wiki.+node-create {"title": "项目计划"}

# 在指定知识空间中创建一个 docx 节点
wiki.+node-create {"space_id": "<SPACE_ID>", "title": "项目计划"}

# 在指定父节点下创建一个子节点
wiki.+node-create {"parent_node_token": "<PARENT_NODE_TOKEN>", "title": "迭代记录"}

# 显式指定创建到个人知识库（仅 user 身份；bot 不支持 `--space-id my_library`）
wiki.+node-create {"space_id": "my_library", "title": "学习笔记"}

# 创建一个快捷方式节点（shortcut）
wiki.+node-create {"parent_node_token": "<PARENT_NODE_TOKEN>", "node_type": "shortcut", "origin_node_token": "<ORIGIN_NODE_TOKEN>", "title": "原文档快捷方式"}

# 创建非 docx 类型节点
wiki.+node-create {"space_id": "<SPACE_ID>", "obj_type": "sheet", "title": "周报数据"}

# 预览底层调用链
wiki.+node-create {"title": "Roadmap"}
```

## 返回值

成功后会返回一个 JSON 对象，常见字段包括：

- `resolved_space_id`：最终用于创建的真实知识空间 ID
- `resolved_by`：空间解析来源，可能是 `explicit_space_id`、`parent_node_token`、`my_library`
- `node_token`：新建知识库节点 token
- `obj_token`：节点关联对象 token
- `obj_type`：节点关联对象类型
- `node_type`：节点类型
- `title`：节点标题

> [!IMPORTANT]
>
> 以应用身份创建时，结果里会额外返回 `permission_grant` 字段，明确说明授权结果：
> - `status = granted`：当前 CLI 用户已获得该知识库节点的可管理权限
> - `status = failed`：节点已创建成功，但自动授权用户失败；会带上失败原因，并提示稍后重试或继续使用 bot 身份处理该节点
>
> `permission_grant.perm = full_access` 表示该资源已授予“可管理权限”
>
> **不要擅自执行 owner 转移。** 如果用户需要把 owner 转给自己，必须单独确认。

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `space_id` | 否 | 目标知识空间 ID；`user` 身份可传特殊值 `my_library` 表示个人知识库，`bot` 身份不支持该值 |
| `parent_node_token` | 否 | 父知识库节点 token 或文档 obj_token；在解析出的 Wiki 节点下创建新节点 |
| `title` | 否 | 节点标题 |
| `node_type` | 否 | 节点类型，默认 `origin`；可选值：`origin`、`shortcut` |
| `obj_type` | 否 | 节点对应对象类型，默认 `docx`；可选值：`sheet`、`mindnote`、`bitable`、`file`、`docx`、`slides`。`file` 仅支持 `shortcut` 节点 |
| `origin_node_token` | 否 | 当 `node_type=shortcut` 时必填，表示快捷方式指向的源节点 token |

## 空间解析规则

- **优先级**：`space_id` > `parent_node_token` > `my_library`
- **显式 space**：传了 `space_id` 时，shortcut 会直接使用该空间；如果该值是 `my_library`，则仅 `user` 身份可用，并会先调用 `GET /open-apis/wiki/v2/spaces/my_library` 解析成真实 `space_id`
- **父节点推断**：未传 `space_id` 但传了 `parent_node_token` 时，会先调用 `GET /open-apis/wiki/v2/spaces/node_by_token` 获取父节点，再读取其 `space_id`
- **父节点类型**：`parent_node_token` 接受 Wiki `node_token` 或已挂载到 Wiki 的文档 `obj_token`，创建时使用查询返回的 `node_token`；显式传空间时也会查询并校验父节点空间。
- **个人知识库回退**：`user` 身份下，如果 `space_id` 和 `parent_node_token` 都没传，会自动解析 `my_library`
- **bot 身份限制**：`bot` 身份既没有“个人知识库”回退语义，也不支持显式传 `space_id=my_library`；请改用真实 `space_id` 或 `parent_node_token`

## 节点类型与对象类型

| `node_type` | 支持的 `obj_type` |
|-------------|-------------------|
| `origin` | `sheet`、`mindnote`、`bitable`、`docx`、`slides` |
| `shortcut` | `sheet`、`mindnote`、`bitable`、`file`、`docx`、`slides` |

- `node_type=shortcut` 时，必须同时提供 `origin_node_token`
- `node_type=origin` 时，不能传 `origin_node_token`
- `obj_type=file` 仅支持 `node_type=shortcut`；实体节点不支持创建 `file` 类型
- `shortcut` 节点只是知识库中的快捷方式入口；真正被引用的节点由 `origin_node_token` 指定
- 如果 `wiki.+node-create` 因上述组合返回参数校验错误，禁止改用 raw `wiki.nodes.create` 或直接调用 OpenAPI 绕过校验；应修正 `node_type`、`obj_type` 或 `origin_node_token`

```text
# 创建一个指向文件的快捷方式节点
wiki.+node-create {"space_id": "<SPACE_ID>", "node_type": "shortcut", "obj_type": "file", "origin_node_token": "<ORIGIN_NODE_TOKEN>"}
```

## 一致性校验

- 如果同时传了 `space_id` 和 `parent_node_token`，shortcut 会校验父节点所属空间是否与 `space_id` 一致
- 如果两者解析出的空间不一致，命令会直接返回验证错误，而不会继续创建
- 对于 `my_library`，`user` 身份下也会先解析出真实 `space_id` 后再做这层校验

## 行为说明

- **默认对象类型**：不传 `obj_type` 时默认创建 `docx` 节点
- **默认节点类型**：不传 `node_type` 时默认创建普通节点 `origin`
- **dry-run 编排**：
  - 仅传 `title`：会展示 `my_library` 解析 + 创建节点 两步调用
  - 仅传 `parent_node_token`：会展示“查询父节点 -> 创建节点”两步调用
  - 同时需要 `my_library` 和父节点时：会展示三步调用链
- **输出结果**：成功后会返回 `resolved_space_id`、`resolved_by`、`node_token`、`obj_token`、`obj_type`、`node_type`、`title` 等字段，便于后续继续操作
- **结构限制**：返回 `131003` 表示触发了知识空间总节点数、目录深度或单个父节点直属子节点数等结构限制。这不是瞬时错误，禁止使用相同参数重试。根据上游错误信息选择更浅或其他父节点、重新组织现有节点，或清理/改用其他知识空间；不要在无法确认具体限制时盲目增加中间层级。

## 推荐场景

- 用户说“在我的知识库里新建一篇页面”时，优先用 `wiki.+node-create`（title="..."）
- 用户已经给出父页面链接或 `parent_node_token` 时，优先传 `parent_node_token`，让 shortcut 自动推导空间
- 需要创建知识库快捷方式时，使用 `node_type shortcut origin_node_token <token>`

> [!CAUTION]
> `wiki.+node-create` 是**写入操作**，执行前必须确认用户意图。

## 参考

- `wiki` 领域指南 -- 知识库全部命令
