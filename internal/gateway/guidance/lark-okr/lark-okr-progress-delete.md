> 由 lark-cli `7f70de0666a2` 的 `lark-okr` 参考文档转换；调用约定见 `okr` 领域指南（`feishu_guide` 的 domain 参数）。

# `okr.+progress-delete`

根据 ID 删除一条 OKR 进展记录。此操作为高风险操作，删除后不可恢复。

## 推荐命令

```text
# 删除指定 ID 的进展记录
okr.+progress-delete {"progress_id": "1234567890123456789"}

# 预览 API 调用而不实际执行
okr.+progress-delete {"progress_id": "1234567890123456789"}
```

## 参数

| 参数              | 必填 | 默认值    | 说明                    |
|-----------------|----|--------|-----------------------|
| `progress_id` | 是  | —      | 进展记录 ID（int64 类型，正整数） |

## 工作流程

1. 使用 `okr.+progress-get` 确认要删除的进展记录 ID 和内容。
2. 执行 `okr.+progress-delete`（progress_id="1234567890123456789"）。
3. 报告结果：已删除的进展记录 ID。

> **注意**：此操作不可恢复，建议在删除前先用 `okr.+progress-get` 确认记录内容。

## 输出

返回 JSON：

```json
{
  "deleted": true,
  "progress_id": "1234567890123456789"
}
```

## 参考

- `okr` 领域指南 -- 所有 OKR 命令(shortcut 和 API 接口)
