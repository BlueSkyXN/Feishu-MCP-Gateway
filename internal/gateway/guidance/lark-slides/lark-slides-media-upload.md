> 由 lark-cli `7f70de0666a2` 的 `lark-slides` 参考文档转换；调用约定见 `slides` 领域指南（`feishu_guide` 的 domain 参数）。

# `slides.+media-upload`（上传本地图片到飞书幻灯片）

把本地图片上传到指定演示文稿的 drive 媒体库，返回 `file_token`。**返回的 token 作为 `<img src="...">` 的值塞进 slide XML 即可显示图片。**

## 命令

```text
# 直接传 xml_presentation_id

# 传 slides URL 也行

# 传 wiki URL（CLI 自动通过 node_by_token 接口解析真实 token，校验 obj_type=slides）

# 预览（不实际上传）
# 网关不提供 dry-run 预览，此示例省略
```

## 返回值

```json
{
  "file_token": "boxcnXXXXXXXXXXXXXXXXXXXXXX",
  "file_name": "pic.png",
  "size": 12345,
  "presentation_id": "slidesXXXXXXXXXXXXXXXXXXXXXX"
}
```

- **`file_token`**：把它写进 `<img src="...">`
- **`file_name` / `size`**：上传文件元信息
- **`presentation_id`**：解析后的真实 `xml_presentation_id`（wiki URL 解析后会变化）

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `presentation` | 是 | `xml_presentation_id`、`/slides/<token>` URL，或 `/wiki/<token>` URL |

> [!IMPORTANT]

## 使用流程

> 新建 PPT（`slides.+create`（主题 `lark-slides-create`））或给已有 PPT 加新页（`slides.+add-slide`（主题 `lark-slides-add-slide`））都不需要单独上传：XML 里把 `<img src>` 写成 `@<本地路径>`，CLI 会自动上传并替换成 `file_token`。
> 本命令用于往**已有页**里加图，或需要自己拿着 `file_token` 拼 XML 的场景。

### 给已有 PPT 的已有页加图

拿到 `file_token` 后走 `slides.+replace-slide`（主题 `lark-slides-replace-slide`） 的 `block_insert`，不用搬原 XML、不改 `slide_id`、不打乱页序：

```bash
PRES_ID=xxx
SID=yyy       # 要加图的那一页

# 1) 上传图片拿 file_token
  --file ./pic.png --presentation $PRES_ID --jq '.data.file_token')

# 2) block_insert 到页末（或用 insert_before_block_id 指定插入位置）
```

注意事项：

1. **`<img>` 坐标避开现有元素** —— 先读现有元素 bbox 挑空白区；空间不够就先用 `block_replace` 挪动/缩小现有元素后再放图
2. **`<img>` 的 `width:height` 对齐原图比例** —— 比例不一致会被裁剪，参见 主题 `references-xml-xml-schema-quick-ref` `<img>` 说明

## 上传约束

`slides.+media-upload` 会处理 Slides 所需的媒体归属参数；调用者只需传入 `file` 和 `presentation`。单张图片最大 20 MB。

## 常见错误

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 1061002 | params error / 不支持的 parent_type | 使用 `slides.+media-upload`；它会采用 Slides 所需的 `parent_type` |
| 1061044 | parent node not exist | `presentation` 给的 token 不对，或不是 slides 类型 |
| 403 | 权限不足 | 检查 `docs:document.media:upload` scope；wiki URL 还需要 `wiki:node:read` |

## 相关命令

- `slides.+create`（主题 `lark-slides-create`） — 新建 PPT（支持 `@` 占位符自动上传图片）
- `slides.+replace-slide`（主题 `lark-slides-replace-slide`） — 给已有页加图 / 换图（`block_insert` / `block_replace`）
- `slides.+add-slide`（主题 `lark-slides-add-slide`） — 追加/插入单页（同样支持 `@` 占位符自动上传）
