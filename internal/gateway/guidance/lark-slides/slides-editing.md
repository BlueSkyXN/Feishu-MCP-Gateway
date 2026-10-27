> 由 lark-cli `7f70de0666a2` 的 `lark-slides` 参考文档转换；调用约定见 `slides` 领域指南（`feishu_guide` 的 domain 参数）。

# 编辑已有 PPT：读-改-写闭环

局部编辑走 **shortcut `slides.+replace-slide`（主题 `lark-slides-replace-slide`）**（块级替换 / 插入），配合 `slides.+xml-get` 读原页拿 `block_id`。整页重建走 **`slides.+update-slide`（主题 `lark-slides-update-slide`）**，多页就每页各跑一次 —— 它原地覆盖并保留 `slide_id` 和页序；只有写进 `content` 且带原 id 的元素才会保留元素 id，遗漏的元素会被删除。

> 生成 XML 前**必读** 主题 `references-xml-xml-schema-quick-ref`。

## 决策树：block_replace vs block_insert

| 需求 | 推荐 action | 理由 |
|------|------------|------|
| 已知某块的 `block_id`，要换这块内容（改标题、换图、挪坐标） | `block_replace` | 精准替换，原子性好；`replacement` 根 `id` 由 CLI 自动注入为 `block_id` |
| 只加 1~N 个元素、不动现有布局 | `block_insert` | 新增不覆盖，可选 `insert_before_block_id` 指定位置 |
| 一次动多个元素（如：换标题 + 加图） | 单次 `parts` 里拼多条 | 整批作为原子事务，任一失败整批不生效；`block_replace` 和 `block_insert` 可混用 |
| 整页版式重建、整页坐标重排、改页面背景、删若干元素 | `slides.+update-slide`（每页一次） | 原地整页覆盖，`slide_id` 和页序不变；带原 `id` 的元素保留 id，不带 `id` 的作为新元素插入，遗漏的被删除 |

> **没有字段级 patch**：即便只想改一个 `shape` 的 `topLeftX`，也得把整个块的新 XML 写出来用 `block_replace`。这不是"微调"，是块级重写。

## 最小读-改-写闭环

```bash
PRES_ID="xml_presentation_id_here"
SID="slide_id_here"

# 2. 用 +replace-slide 直接改那个块（不需要搬原 XML）
```

`slide_id` / 页序不会变。`block_replace` 的 `replacement` 根元素 `id` 会自动注入为 `block_id`，用户手写 XML 时不需要自己加。

> **编写 `parts` 时只使用标准字段**：`block_replace` 使用 `action` + `block_id` + `replacement`（XML 字符串），`block_insert` 使用 `action` + `insertion`（可选 `insert_before_block_id`）。收到 unknown field 报错时应按上述结构修改字段名，而不是修改字段值。

## `revision_id` 参数

`revision_id` 默认 `-1`，表示基于当前最新版执行。传具体版本号时，服务端以该版本为 base 应用变更：

```bash

# 写时传该版本号，服务端以此为 base
```

注意：传不存在的版本号（超过当前 revision）会返回 3350002 not found；不确定时用 `-1` 即可。

## `tid` 事务锁

跨请求的并发事务 ID，多人协作长事务才用得上。**单人单次调用留空**即可。

## 两种 action 详解

### block_replace — 整块替换

适合"已知块 ID，要换这块整体内容"的场景。`replacement` 根元素的 `id="<block_id>"` 由 CLI 自动注入（用户手写的 XML 如果没带 `id` 直接省略即可；如果带了错的会被覆盖为正确值）。

字段说明：

| 字段 | 必填 | 说明 |
|------|------|------|
| `action` | 是 | 固定为 `block_replace` |
| `block_id` | 是 | 目标块的 3 位 short element ID（从 `slides.+xml-get` 返回的 XML 里读）|
| `replacement` | 是 | 新 XML 片段；根元素 `id` 会被 CLI 自动注入为 `block_id` |

### block_insert — 整块插入

适合"只想加一个元素，不动现有元素"的场景（典型：给已有页加图）。

字段说明：

| 字段 | 必填 | 说明 |
|------|------|------|
| `action` | 是 | 固定为 `block_insert` |
| `insertion` | 是 | 要插入的完整 XML 片段 |
| `insert_before_block_id` | 否 | 插到这个块之前；省略（不提供此字段）则追加到页面末尾 |

> **`<img>` 必须用 `file_token`**，不能用外链 URL——先 `slides.+media-upload`（file={"file_id": "<file_id>"}，presentation="$PRES_ID"） 拿 token。

### 批量 parts

一次 `parts` 最多 200 条，按数组顺序串行执行。`block_replace` 和 `block_insert` 可以在同一批次混用。举例：一次性把标题块替换、然后在末尾追加一个装饰图。

整批作为原子事务：任一条失败整批不生效。失败时后端通常返回 3350001；若响应中带 `failed_part_index` / `failed_reason` 字段，shortcut 会原样透传。

## 大 `parts` 用 jq 或 stdin 组装

`parts` 支持 `{"file_id": …}`（读文件）和 `-`（stdin）作为值来源，适合批量 XML 场景：

```bash

# 从 stdin 读
```

## 错误排查

| 现象 | 原因 | 对策 |
|------|------|------|
| 3350001，hint 含 "block_id not found" | `parts[i].block_id` 在当前页不存在 | 重新用 `slides.+xml-get` 拿最新 XML，按里面的 short ID 再填 |
| 3350002 not found | `revision_id` 传了不存在的版本号 | 用 `-1` 或 `slides.+xml-get` 返回的 `revision_id` |
| `<img>` 不显示 / 显示破图 | `src` 写了外链 URL | 换成通过 `slides.+media-upload` 拿到的 `file_token` |
| 3350001（block_replace 返回） | 正常情况下 CLI 已自动注入 `id` 和 `<content/>`；如果仍报错，确认 `block_id` 在当前页存在（重新 `slides.+xml-get`），检查 XML 结构是否合法；坐标是否超出 960×540 范围 | — |

## 相关文档

- 主题 `lark-slides-replace-slide` — `slides.+replace-slide` shortcut 参数详情
- 主题 `lark-slides-update-slide` — `slides.+update-slide` shortcut 参数详情（整页覆盖）
- 主题 `lark-slides-xml-presentations-get` — `slides.+xml-get` 参数与单页读取方法
- 主题 `lark-slides-media-upload` — 上传图片拿 file_token
- 主题 `references-xml-xml-schema-quick-ref` — XML 元素和属性速查
