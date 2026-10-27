> 由 lark-cli `7f70de0666a2` 的 `lark-okr` 参考文档转换；调用约定见 `okr` 领域指南（`feishu_guide` 的 domain 参数）。

# `okr.+comment-create` (not available in the gateway: user identity only)

创建一条 OKR 评论，或回复已有的评论。只支持 user 身份。

## 推荐命令

```text
# 在周期下创建实体级评论。
okr.+comment-create  # 网关不提供：仅支持用户身份

# 在 Objective 正文中创建指定文本的划词评论。
okr.+comment-create  # 网关不提供：仅支持用户身份

# 在 Objective 正文中创建划词评论。
okr.+comment-create  # 网关不提供：仅支持用户身份

# 在 KeyResult 的已有划词评论串中追加回复。
okr.+comment-create  # 网关不提供：仅支持用户身份

# 使用 richtext 文件作为评论正文。
okr.+comment-create  # 网关不提供：仅支持用户身份
```

```text
# 写入前预览创建评论的 URL、参数和请求体。
okr.+comment-create  # 网关不提供：仅支持用户身份
```

## 常用表述

以下是一些用户需求中常见的表述:

- 全局评论/周期评论/OKR评论: 指 OKR 周期的实体级评论，当用户要求创建全局评论，或对某个周期的 OKR 进行评论（不特指某个 Objective 或 KeyResult 时），可以创建周期实体级评论。
- 划词评论: 指 Objective/KeyResult 下的划词评论。需要注意，Objective/KeyResult 下不能创建实体级评论（必须携带 selected-text 或 select-all）。若用户没有特别指定需评论的段落，使用 --select-all

## 参数

| 参数             | 必填 | 默认值  | 说明                                                                                                          |
|------------------|------|---------|---------------------------------------------------------------------------------------------------------------|
| `target_type`    | 是   | —       | cycle、progress、objective 或 key_result。                                                                    |
| `target_id`      | 是   | —       | 评论对象 ID，int64 正整数。                                                                                   |
| `content`        | 是   | —       | 评论正文；输入风格：`simple`（半纯文本 JSON，推荐） \| `richtext`（完整 ContentBlock JSON），支持 @文件路径。 |
| --selected-text  | 否   | —       | Objective/KeyResult 新建划词时的完整纯文本。                                                                  |
| --select-all     | 否   | false   | Objective/KeyResult 划词时选择全文。                                                                          |
| --ref-comment-id | 否   | —       | 回复 Progress/Cycle 评论，或将 Objective/KeyResult 评论挂入已有划词串。                                       |
| `style`          | 否   | simple  | 输入/输出风格：simple 或 richtext。                                                                           |
| `user_id_type`   | 否   | open_id | open_id、union_id、user_id 或 user_key。                                                                      |

## 评论场景参数组合

| 场景                           | target-type             | 必须传                                                        | 不能传                                                |
|--------------------------------|-------------------------|---------------------------------------------------------------|-------------------------------------------------------|
| 创建周期/进展实体级评论        | cycle 或 progress       | `content`                                                   | `--selected-text`、`--select-all`、`--ref-comment-id` |
| 回复周期/进展已有评论          | cycle 或 progress       | `content`、`--ref-comment-id`                               | `--selected-text`、`--select-all`                     |
| 创建 Objective/KR 划词评论     | objective 或 key_result | `content`，并在 `--selected-text` / `--select-all` 中二选一 | `--ref-comment-id`                                    |
| 追加到 Objective/KR 划词评论串 | objective 或 key_result | `content`、`--ref-comment-id`                               | `--selected-text`、`--select-all`                     |

## 工作流程

1. 确定评论 target：使用 `okr.+cycle-detail`（主题 `lark-okr-cycle-detail`） 获取 Objective/KeyResult ID，使用 `okr.+progress-list`（主题 `lark-okr-progress-list`） 获取 Progress ID；已有评论串时使用 `okr.+comment-list`（主题 `lark-okr-comment-list`） 或 `okr.+comment-get`（主题 `lark-okr-comment-get`） 获取 comment-id。
2. 根据 target-type 选择评论形式：
   - cycle/progress：不传 selected-text 或 select-all；需要回复时传 ref-comment-id。
   - objective/key_result：在 selected-text、select-all、ref-comment-id 中选择且只能选择一个；selected-text 和 select-all 互斥，二者也都和 ref-comment-id 互斥。
3. 准备 content：content 是业务必填，通常建议使用 simple 格式，需要精确控制 @用户的位置时，可以使用 richtext 格式，参考 ContentBlock 格式（主题 `lark-okr-contentblock`）
4. 执行命令；真实写入前可以先使用 dry-run（网关不提供，改为先读取核对） 检查 URL、query 和 body。
5. 在创建(而非回复) Objective/KeyResult 划词评论时，若用户未指定评论的具体位置，通常可以使用 select-all 而非自行指定 selected-text，除非用户需求中明确了具体的段落。
   - 若需使用 selected-text 精确选择划词选区时，只可传入正文中真实存在的连续纯文本片段；不要包含或跨越 mention 占位符，否则无法命中具体内容。
   - selected-text 会选择对应文本的首个命中。若 selected-text 未匹配到内容，会 fallback 至选择全文。

## 输出

创建成功返回 JSON：

```json
{
  "comment_id": "7000000000000000004",
  "selection_id": "8000000000000000002"
}
```

- comment_id 是新评论 ID。
- selection_id 只在创建划词评论时返回，用于识别评论串。
- 创建接口不直接返回完整 Comment；需要详情时使用 `okr.+comment-get`（主题 `lark-okr-comment-get`）。

## 注意事项

- Objective/KeyResult 的 ref-comment-id 只用于定位已有划词串，不会在新评论的 ref_comment_id 字段建立引用关系。
- `--ref-comment-id` 必须传评论实体自身的 `id`，不能传 `selection.id`。`selection.id` 只用于识别同一个划词评论串；如果要回复某个划词串，应先从 `okr.+comment-list` 或 `okr.+comment-detail` 中找到该串内任意一条 Comment 的 `id`，再将这个 `id` 传给 `--ref-comment-id`。
- Progress/Cycle 是实体级评论；Progress 的 ref-comment-id 会建立普通评论之间的引用关系。
- 评论的 content 不支持 docs/images 字段，建议使用 simple 格式填写

## 参考

- `okr` 领域指南 — OKR 命令、路由和通用约定
- OKR 实体定义（主题 `lark-okr-entities`） — Comment、评论串和 target 类型
- ContentBlock 格式（主题 `lark-okr-contentblock`） — simple/richtext 输入格式
- `okr.+comment-list`（主题 `lark-okr-comment-list`） — 查询已有评论和 selection.id
- `okr.+comment-get`（主题 `lark-okr-comment-get`） — 获取评论详情
- `okr.+comment-solve`（网关不提供：仅支持用户身份） / `okr.+comment-reopen`（网关不提供：仅支持用户身份）（主题 `lark-okr-comment-solve-reopen`） — 管理评论状态
