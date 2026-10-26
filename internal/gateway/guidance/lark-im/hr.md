> 由 lark-cli `7f70de0666a2` 的 `lark-im` 参考文档转换；调用约定见 `im` 领域指南（`feishu_guide` 的 domain 参数）。

# 分割线 `hr`

分隔卡片内容的水平线。**Card 2.0**（1.0 同名 `hr`）。

## 最小示例

```json
{ "tag": "hr" }
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `hr` |
| `margin` | 否 | String | 0 | 外边距，范围 [-99,99]px，如 `"8px 0"` |
| `element_id` | 否 | String | / | 组件唯一标识，字母开头、≤20 字符 |
