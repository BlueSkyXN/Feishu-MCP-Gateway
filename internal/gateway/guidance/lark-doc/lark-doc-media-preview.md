> 由 lark-cli `7f70de0666a2` 的 `lark-doc` 参考文档转换；调用约定见 `docs` 领域指南（`feishu_guide` 的 domain 参数）。

# `docs.+media-preview`（预览文档素材）

优先用于查看、预览文档中的图片或文件素材（`file_token`）。命令会把素材保存到本地路径，便于后续打开查看内容。

## 选择规则

- 用户说“看一下素材 / 图片 / 附件”“预览一下”时，优先使用 `docs.+media-preview`
- 用户明确说“下载”时，使用 `docs.+media-download`（主题 `lark-doc-media-download`）
- 如果目标明确是画板 / whiteboard / 画板缩略图，不要使用 `docs.+media-preview`，改用 `docs.+media-download`（type="whiteboard"）

## 命令

```text
# 预览图片/文件素材
docs.+media-preview {"token": "Z1Fjxxxxxxxx", "output": "./asset"}

# 指定输出文件名（带扩展名则不会自动补全）
docs.+media-preview {"token": "Z1Fjxxxxxxxx", "output": "./asset.png"}
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `token=<token>` | 是 | 素材 token，即 `file_token` |
| `output=<path>` | 是 | 本地保存路径；不带扩展名会自动补全 |

## token 从哪里来

- 若你是从文档内容里提取：`lark-doc-fetch` 返回的内容里可能包含：
  - 图片：`<img token="..." .../>`
  - 文件：`<source token="..." name="..."/>`

## 参考

- lark-doc-fetch（主题 `lark-doc-fetch`） — 获取文档内容（用于提取 token）
- lark-doc-media-download（主题 `lark-doc-media-download`） — 明确下载素材，或下载画板缩略图
