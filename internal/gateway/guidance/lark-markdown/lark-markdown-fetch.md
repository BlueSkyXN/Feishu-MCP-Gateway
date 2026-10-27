> 由 lark-cli `7f70de0666a2` 的 `lark-markdown` 参考文档转换；调用约定见 `markdown` 领域指南（`feishu_guide` 的 domain 参数）。

# `markdown.+fetch`

读取 Drive 中原生 Markdown 文件的内容；也支持把内容保存到本地。

## 命令

```text
# 直接返回 Markdown 文本
markdown.+fetch {"file_token": "boxcnxxxx"}

# 保存到本地
markdown.+fetch {"file_token": "boxcnxxxx", "output": "./README.md"}

# 传目录时，使用远端文件名保存到该目录下
markdown.+fetch {"file_token": "boxcnxxxx", "output": "./downloads/"}

# 覆盖已存在文件
markdown.+fetch {"file_token": "boxcnxxxx", "output": "./README.md", "overwrite": true}

# 预览底层请求
# 网关不提供 dry-run 预览，此示例省略
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `file_token` | 是 | 目标 Markdown 文件 token |
| `output` | 否 | 本地保存路径；既可传具体文件名，也可传目录路径。传目录时使用远端文件名保存；省略时直接返回 Markdown 内容 |
| `overwrite` | 否 | 覆盖已存在的本地输出文件；仅在传入 `output` 时生效 |

## 返回值

不传 `output`：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "file_token": "boxcnxxxx",
    "file_name": "README.md",
    "content": "# Hello\n",
    "size_bytes": 8
  }
}
```

传入 `output`：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "file_token": "boxcnxxxx",
    "file_name": "README.md",
    "saved_path": "/abs/path/README.md",
    "size_bytes": 8
  }
}
```

## 参考

- `markdown` 领域指南 — Markdown 域总览
