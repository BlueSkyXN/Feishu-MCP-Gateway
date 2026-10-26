> 由 lark-cli `7f70de0666a2` 的 `lark-okr` 参考文档转换；调用约定见 `okr` 领域指南（`feishu_guide` 的 domain 参数）。

# `okr.+batch-create`

批量创建 OKR 目标（Objective）和关键结果（Key Result）。

## 推荐命令

```bash

# 预览 API 调用（Dry-run）
```
- mention 是可选参数，不需要使用“@”提及其他用户时不传入。
  - 传入的 mention 参数会以 @对应用户的形式，添加在文本后。
- Objective 的 notes / notes_mention 是可选参数，用于创建目标备注；KR 不支持备注。
- Objective 的 category_id 是可选参数；也可以通过 `category_id` 给所有未显式设置分类的 Objective 指定默认分类。

## 参数

| 参数               | 必填 | 默认值       | 说明                                                         |
|------------------|----|-----------|------------------------------------------------------------|
| `cycle_id`     | 是  | —         | OKR 周期 ID（int64 类型）                                        |
| `input`        | 是  | —         | JSON 数组格式的 Objective 列表。支持 `@文件路径` 从文件读取或 `-` 从 stdin 读取。 |
| `category_id`  | 否  | —         | 默认 Objective 分类 ID。仅用于 input 中未设置 `category_id` 的 Objective。通常不需要传入，见下方“分类提示”。 |
| `user_id_type` | 否  | `open_id` | mention 中使用的用户 ID 类型：`open_id` \| `union_id` \| `user_id`  |
| dry-run（网关不提供，改为先读取核对）      | 否  | —         | 预览 API 调用而不实际执行                                            |
|        | 否  | `json`    | 输出格式                                                       |

## 输入格式

```json
[
  {
    "text": "Objective 内容",
    "mention": ["ou_xxxxxxxx", "ou_yyyyyyyy"],
    "notes": "Objective 备注",
    "notes_mention": ["ou_xxxxxxxx"],
    "category_id": "7249339036661170180",
    "krs": [
      {
        "text": "KR 内容",
        "mention": ["ou_zzzzzzzz"]
      }
    ]
  }
]
```

字段说明：

- `text`：Objective 或 KR 内容，必填。
- `mention`：追加到内容后的用户 mention，可选。
- `notes`：Objective 备注文本，可选，仅 Objective 支持。
- `notes_mention`：追加到 Objective 备注后的用户 mention，可选，仅在 `notes` 存在时有意义。
- `category_id`：Objective 分类 ID，可选；会覆盖命令级 `category_id`。
- `krs`：当前 Objective 下要创建的 KR 列表，可选。

## 工作流程

1. 使用 `okr.+cycle-list` 获取可用的 OKR 周期 ID
2. 构造 `input` JSON 数组，包含要创建的 Objective 和 KR
3. 执行 `okr.+batch-create`（cycle_id="<id>"，input="..."）

## 输出

成功返回 JSON：

```json
{
  "ok": true,
  "data": {
    "created": [
      {
        "objective_id": "7000000000000000002",
        "krs": ["7000000000000000003", "7000000000000000004"]
      },
      {
        "objective_id": "7000000000000000005",
        "krs": ["7000000000000000006"]
      }
    ]
  }
}
```

## 参考

- OKR 业务实体（主题 `lark-okr-entities`） -- OKR 实体结构定义
