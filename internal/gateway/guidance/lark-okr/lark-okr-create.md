> 由 lark-cli `7f70de0666a2` 的 `lark-okr` 参考文档转换；调用约定见 `okr` 领域指南（`feishu_guide` 的 domain 参数）。

# `okr.+create`

创建单个 OKR 目标（Objective）或关键结果（Key Result）。这是单条写入场景的首选 shortcut；如果需要一次创建多个 Objective 及其 KR，可使用 `okr.+batch-create`（主题 `lark-okr-batch-create`）。

## 推荐命令

```bash
# 在指定周期下创建一个 Objective（默认 simple 风格）

# 在已有 Objective 下创建一个 KR

# 使用 richtext 风格创建 Objective（完整 ContentBlock JSON）

# 预览 API 调用而不实际执行
```

## 参数

| 参数               | 必填 | 默认值       | 说明                                                                                                                 |
|------------------|----|-----------|--------------------------------------------------------------------------------------------------------------------|
| `level`        | 是  | —         | 创建层级：`objective`（创建目标）\| `key-result`（在已有目标下创建 KR）                                                                 |
| `cycle_id`     | 条件 | —         | OKR 周期 ID（int64 类型）。当 `level=objective` 时**必填**。                                                                 |
| `objective_id` | 条件 | —         | Objective ID（int64 类型）。当 `level=key-result` 时**必填**。                                                             |
| `style`        | 否  | `simple`  | 内容输入风格：`simple`（半纯文本 JSON，推荐） \| `richtext`（完整 ContentBlock JSON）。请参考 ContentBlock 格式（主题 `lark-okr-contentblock`）。 |
| `content`      | 是  | —         | 内容。根据 `style` 指定格式。支持 `@文件路径` 从文件读取或 `-` 从 stdin 读取。                                                             |
| `notes`        | 否  | —         | Objective 备注，仅 `level=objective` 支持。根据 `style` 指定格式，支持 `@文件路径` 或 `-` 从 stdin 读取。                               |
| `category_id`  | 否  | —         | Objective 分类 ID，仅 `level=objective` 支持。通常不需要传入，见下方“分类提示”。                                                        |
| `user_id_type` | 否  | `open_id` | 用户 ID 类型：`open_id` \| `union_id` \| `user_id`。影响 mention 中用户 ID 的解释方式。                                             |

## 输入格式

### `style=simple`（默认）

推荐大多数创建场景使用 `simple` 风格。`content` 和 `notes` 都使用 `SemiPlainContent` JSON：

```json
{
  "text": "提升北极星指标",
  "mention": ["ou_xxxxxxxx"]
}
```

规则：

- `text` 必填，且不能为空白字符串
- `mention` 可选；如果传入，数组中的每个用户 ID 都不能为空字符串
- `notes` 仅适用于 Objective；创建 KR 时传 `notes` 会报错
- 同一条命令只有一个 flag 可以使用 `-` 读取 stdin；如果 `content=-`，`notes` 请使用内联 JSON 或 `@文件路径`

### `style=richtext`

当你需要精确控制段落结构、插入文档链接，或使用完整富文本块结构时，使用 `richtext` 风格：

```json
{
  "blocks": [
    {
      "block_element_type": "paragraph",
      "paragraph": {
        "elements": [
          {
            "paragraph_element_type": "textRun",
            "text_run": {
              "text": "建立跨部门协作机制"
            }
          }
        ]
      }
    }
  ]
}
```

规则：

- `blocks` 至少需要有一个非空段落或图片块
- 不能传空 `blocks`，也不能传只有空段落元素的内容
- 更多结构说明见 ContentBlock 富文本格式（主题 `lark-okr-contentblock`）

## 工作流程

1. 如果要创建 Objective，先使用 `okr.+cycle-list` 获取目标周期的 `cycle_id`。
2. 如果要给已有 Objective 新增 KR，先通过 `okr.+cycle-detail` 或其他 OKR 查询命令拿到 `objective_id`。
3. 选择输入风格：
   - **推荐**：`simple`，适合普通文本和 mention。
   - 需要复杂富文本时：`richtext`。
4. 执行 `okr.+create`。
5. 报告结果：
   - 创建 Objective 时返回新的 `objective_id`
   - 创建 KR 时返回新的 `key_result_id`，并附带父 `objective_id`

## Dry-run 对应接口

- `level=objective`：
  - `POST /open-apis/okr/v2/cycles/:cycle_id/objectives`
- `level=key-result`：
  - `POST /open-apis/okr/v2/objectives/:objective_id/key_results`

## 输出

### 创建 Objective 成功

```json
{
  "level": "objective",
  "objective_id": "7000000000000000002"
}
```

### 创建 KR 成功

```json
{
  "level": "key-result",
  "objective_id": "7000000000000000002",
  "key_result_id": "7000000000000000003"
}
```

## 常见错误与处理

- `level=objective` 但未传 `cycle_id`
  - 补充有效的周期 ID
- `level=key-result` 但未传 `objective_id`
  - 补充已有 Objective 的 ID
- `content` 为空、不是合法 JSON，或内容结构为空
  - 按 `style` 对应格式修正输入
- 在 `simple` 风格中传了 `docs` 或 `images`
  - 改用 `style=richtext`，或移除这些字段

## 何时用 `okr.+create`，何时用 `okr.+batch-create`

| 命令 | 适用场景 |
|------|----------|
| `okr.+create` | 创建单个 Objective，或向已有 Objective 新增单个 KR |
| `okr.+batch-create` | 一次创建多个 Objective，并可同时为每个 Objective 创建多个 KR |

## 参考

- `okr` 领域指南 -- 所有 OKR 命令
- OKR 业务实体（主题 `lark-okr-entities`） -- Objective、KR、周期等基础概念
- ContentBlock 格式（主题 `lark-okr-contentblock`） -- content/notes 字段的另一种输入风格，支持完整富文本格式
- `okr.+batch-create`（主题 `lark-okr-batch-create`） -- 批量创建多个 Objective / KR
