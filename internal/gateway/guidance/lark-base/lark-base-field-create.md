> 由 lark-cli `7f70de0666a2` 的 `lark-base` 参考文档转换；调用约定见 `base` 领域指南（`feishu_guide` 的 domain 参数）。

# `base.+field-create`

创建一个或多个字段；同一表的多个字段优先使用一次 JSON 数组输入。

`formula` / `lookup` 创建前读取对应 guide；涉及跨表引用时同时读取目标表结构。

## 推荐命令

```text
base.+field-create {"base_token": "<base_token>", "table_id": "<table_id>", "json": {"name": "预算", "type": "number", "style": {"type": "plain", "precision": 2}}}

base.+field-create {"base_token": "<base_token>", "table_id": "<table_id>", "json": {"name": "状态", "type": "select", "multiple": false, "default_value": ["Todo"], "options": [{"name": "Todo", "hue": "Blue", "lightness": "Lighter"}, {"name": "Done", "hue": "Green", "lightness": "Light"}]}}

# 多个字段复用相同字段 JSON 形状，一次传非空数组
base.+field-create {"base_token": "<base_token>", "table_id": "<table_id>", "json": [{"name": "备注", "type": "text"}, {"name": "优先级", "type": "select", "multiple": false, "options": [{"name": "高"}, {"name": "低"}]}]}
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `base_token=<token>` | 是 | Base Token |
| `table_id=<id_or_name>` | 是 | 表 ID 或表名 |
| `json=<body>` | 是 | 单个字段 JSON 对象，或多个字段对象组成的非空数组 |

## API 入参详情

**HTTP 方法和路径：**

```
POST /open-apis/base/v3/bases/:base_token/tables/:table_id/fields
```

## JSON 值规范

- `json` 接受单个字段 **JSON 对象**，也接受多个字段对象组成的非空数组；不要再套 `fields` 等外层对象。
- 数组按顺序创建字段，遇到首个失败即停止且不自动回滚；部分失败时保留 `items` 中的 `created` 项，按 `hint` 修正后只提交 `failed` 和 `not_attempted` 项，并保持依赖顺序。
- 每个字段对象最少包含：`name`、`type`。
- 所有字段类型都支持可选 `description`；支持纯文本，也支持 Markdown 链接，如 `协作约定可参考[团队字段约定](https://example.com/field-spec)`。
- 需要字段默认值时传 `default_value`，直接使用字段对应 CellValue；`datetime` / `user` 的动态填充用 `$slot`。完整规则见 Field Schema（主题 `lark-base-field-schema`）。
- `type` 不同，必填子字段不同：
  - `select`：`multiple` 控制是否多选，`options` 定义静态选项，`dynamic_options_source` 定义动态选项来源。静态与动态选项配置二选一，不能同时传。
  - `link`：必须有 `link_table`，可选 `bidirectional`、`bidirectional_link_field_name`。
  - `formula`：必须有 `expression`；先读 formula guide，再创建。
  - `lookup`：必须有 `from`、`select`、`where`；先读 lookup guide，再创建。

**正确（`base.+field-create`）**

```json
{
  "name": "状态",
  "type": "select",
  "multiple": false,
  "default_value": ["Todo"],
  "options": [
    { "name": "Todo", "hue": "Blue", "lightness": "Lighter" },
    { "name": "Done", "hue": "Green", "lightness": "Light" }
  ]
}
```

## 参考

- Field Schema（主题 `lark-base-field-schema`） — 字段 JSON 规范（推荐）
- Formula Field（主题 `lark-base-field-formula`） — 创建公式必读
- Lookup Field（主题 `lark-base-field-lookup`） — 创建查找引用必读
