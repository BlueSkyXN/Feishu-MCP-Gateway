> 由 lark-cli `7f70de0666a2` 的 `lark-slides` 参考文档转换；调用约定见 `slides` 领域指南（`feishu_guide` 的 domain 参数）。

# `slides.+create`（创建飞书幻灯片）

创建一个新的飞书幻灯片演示文稿，可选一步添加页面内容。

提交源必须是直接生成的单页 `<slide>` XML。禁止从完整 `<presentation>` XML 解析、拆分、重序列化出 slide 数组再提交。

本命令只从零创建演示文稿，没有导入本地 PPT 文件的参数。要把已有 PPTX 变成 Slides，用 `drive.+import`（file="<x.pptx>"，type="slides"），再在导入结果上编辑，流程见 template-editing.md（主题 `template-editing`）。

## 创建方式选择

| 场景 | 推荐方式 |
|------|----------|
| 不超过 10 页 | 每页存一个 XML 文件，`slides.+create`（slide=["<内容>", "<内容>"]） 一步创建 |
| 超过 10 页 | **两步创建**：先 `slides.+create` 创建空白 PPT，再用 `slides.+add-slide`（主题 `lark-slides-add-slide`） 逐页添加 |
| 已有 PPT 继续追加或插入页面 | 使用 `slides.+add-slide`（主题 `lark-slides-add-slide`），必要时配合 `before_slide_id` |

> [!IMPORTANT]
> `slides.+create` 带页面时底层会逐页创建，不是原子操作。中途失败时先记录 `xml_presentation_id`，回读确认当前状态，再继续修复或追加。

**CRITICAL — 提交前必须先跑版式 lint**：把待提交的 `<slide>` XML 存成本地文件，运行 `scripts/xml_lint.py`，`summary.error_count` 必须为 0。

## 命令

```text
# 创建空白 PPT
slides.+create {"title": "项目汇报"}

# 预览（不执行）
slides.+create {"title": "项目汇报", "slide": ["<内容>"]}
```

## 返回值

工具成功执行后，返回一个 JSON 对象，包含以下字段：

- **`xml_presentation_id`**（string）：演示文稿的唯一标识符，后续添加页面时需要此 ID
- **`title`**（string）：演示文稿标题
- **`url`**（string，可选）：演示文稿的在线链接，如有返回则务必展示给用户（需要 drive 相关权限；若获取失败则不返回此字段）
- **`revision_id`**（integer）：演示文稿版本号
- **`slide_ids`**（string[]，可选）：带页面创建时返回，成功添加的页面 ID 列表
- **`slides_added`**（integer，可选）：带页面创建时返回，成功添加的页面数量
- **`images_uploaded`**（integer，可选）：页面 XML 中含 `@<本地路径>` 占位符时返回，已上传的去重后图片数量

> [!IMPORTANT]
> 不带页面参数时，`slides.+create` 只创建空白演示文稿。创建后用 `slides.+add-slide`（主题 `lark-slides-add-slide`） 逐页添加 slide 内容。
>
> 带了页面时，CLI 先创建空白演示文稿，再逐页添加页面。如果某一页添加失败，CLI 会停止并报错，已创建的演示文稿和已添加的页面会保留。
>
>
> 以应用身份创建时，结果里会额外返回 `permission_grant` 字段，明确说明授权结果：
> - `status = granted`：当前 CLI 用户已获得该演示文稿的可管理权限
> - `status = skipped`：本地没有可用的当前用户 `open_id`，因此不会自动授权
> - `status = failed`：演示文稿已创建成功，但自动授权用户失败
>
> **不要擅自执行 owner 转移。** 如果用户需要把 owner 转给自己，必须单独确认。

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `title` | 否 | 演示文稿标题（不传则默认 "Untitled"） |
| `slide` | 否 | 一页 `<slide>` XML，或 `@路径`；可重复，最多 10 次。格式见[页面输入形式](#页面输入形式) |
| `slides` | 否 | 页面 XML 的 JSON 字符串数组，最多 10 个；支持 `@文件` 和 `-`（stdin）。格式见[页面输入形式](#页面输入形式) |

10 页是 CLI 的上限，服务端每次只接收一页。超过 10 页时先用 `slides.+create` 创建空白 PPT，再用 `slides.+add-slide`（主题 `lark-slides-add-slide`） 逐页添加。

两种形式的每一页都会在发请求前校验成「单个完整的 `<slide>` 文档」。不合格的页在创建演示文稿之前报错并指出页序号，不会留下空壳演示文稿。

## 页面输入形式

页面内容有 `slide` 和 `slides` 两种传法，二选一，同时传会报错。

### `slide`：一页一个文件

可重复，重复次数即页数，出现顺序即页序。值是一页完整的 `<slide>` XML，或读取该 XML 的 `@路径`。

文件内容就是这一页 XML 本身，外面没有引号或方括号：

```xml
<slide xmlns="https://www.larkoffice.com/sml/2.0">
  <data>…第1页…</data>
</slide>
```

文件内容不需要转义：引号、换行、中文原样写。

### `slides`：一个 JSON 数组

值是 JSON 字符串数组，每个元素是一整页 XML，支持 `@文件` 和 `-`（stdin）。

文件内容是一个 JSON 文档，XML 以 JSON 字符串出现，其中的 `"` 写作 `\"`，换行写作 `\n`：

```json
[
  "<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data>…第1页…</data></slide>",
  "<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data>…第2页…</data></slide>"
]
```

数组元素是页面 XML 原文；请求封装和逐页提交由 `slides.+create` 完成。

> [!WARNING]
> `slides=[...]` 的风险点主要在 shell 参数传递，而不是单纯页数。即使只有 1 页，只要 XML 足够复杂，也建议改用 `slide=@page-01.xml` 逐页传文件。

## 本地图片：`@<path>` 占位符

`<img>` 元素的 `src` 属性如果以 `@` 开头，CLI 会把它当作本地文件路径，自动上传到当前演示文稿，并把占位符替换为返回的 `file_token`。

`slide-01.xml`：

```xml
<slide xmlns="https://www.larkoffice.com/sml/2.0">
  <data>
    <img src="@./assets/chart.png" topLeftX="100" topLeftY="100" width="320" height="180"/>
  </data>
</slide>
```

行为：

- 同一份图被多次引用时**只上传一次**（按路径去重）
- 单张图片最大 20 MB（媒体上传不支持分片）
- 校验阶段就会检查所有占位符文件存在及大小；缺文件或超限直接报错，不会创建空白 PPT 占位
- 创空白 PPT → 上传所有图 → 替换 token → 逐页创建 slide，按这个顺序执行

> [!IMPORTANT]

## 创建后续步骤

创建空白 PPT 时，`slides.+create` 返回的 `xml_presentation_id` 用于后续操作：

```bash
# 第 1 步：创建空白 PPT
PRES_ID=$(lark-cli slides +create --title "项目汇报" --jq '.data.xml_presentation_id')

# 第 2 步：逐页添加（--slide 支持 @file，复杂 XML 优先走文件）
```

## 常见错误

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 400 | 参数错误 | 检查参数格式是否正确 |
| 403 | 权限不足 | 检查是否拥有 `slides:presentation:create` 和 `slides:presentation:write_only` scope |

## 相关命令

- `slides.+add-slide`（主题 `lark-slides-add-slide`） — 追加/插入单页（两步创建的第二步）
- `slides.+xml-get`（主题 `lark-slides-xml-presentations-get`） — 读取 PPT 内容并保存到本地文件
