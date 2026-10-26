> 由 lark-cli `7f70de0666a2` 的 `lark-drive` 参考文档转换；调用约定见 `drive` 领域指南（`feishu_guide` 的 domain 参数）。

# `drive.+export`

把 `doc` / `docx` / `sheet` / `bitable` / `slides`（也支持 Wiki URL / Wiki node token 自动解包）导出到本地文件。这个 shortcut 内置有限轮询：

- 如果导出任务在轮询窗口内完成，会直接下载到本地目录
- 如果轮询结束仍未完成，会返回 `ticket`、`ready=false`、`timed_out=true` 和 `next_command`
- 后续继续查结果时，改用 `drive.+task_result`（scenario="export"）
- 拿到 `file_token` 后，改用 `drive.+export-download`

## 命令

```text
# 推荐：直接传 URL，CLI 自动解析类型和 token
drive.+export {"url": "https://example.feishu.cn/docx/<DOCX_TOKEN>", "file_extension": "pdf"}

# Wiki URL 也推荐直接传，CLI 会先解析到底层 obj_token/obj_type
drive.+export {"url": "https://example.feishu.cn/wiki/<WIKI_NODE_TOKEN>", "file_extension": "pdf"}

# 只有裸 Wiki node token 时，显式传 --doc-type wiki，让 CLI 先解析到底层文档类型
drive.+export {"token": "<WIKI_NODE_TOKEN>", "doc_type": "wiki", "file_extension": "pdf"}

# 导出新版文档为 pdf，默认保存到当前目录
drive.+export {"token": "<DOCX_TOKEN>", "doc_type": "docx", "file_extension": "pdf"}

# 导出旧版文档为 docx
drive.+export {"token": "<DOC_TOKEN>", "doc_type": "doc", "file_extension": "docx"}

# 注意：markdown 只支持 docx
drive.+export {"token": "<DOCX_TOKEN>", "doc_type": "docx", "file_extension": "markdown"}

# 导出电子表格为 xlsx
drive.+export {"token": "<SHEET_TOKEN>", "doc_type": "sheet", "file_extension": "xlsx", "output_dir": "./exports"}

# 导出幻灯片为 pptx
drive.+export {"token": "<SLIDES_TOKEN>", "doc_type": "slides", "file_extension": "pptx", "output_dir": "./exports"}

# 导出幻灯片为 pdf
drive.+export {"token": "<SLIDES_TOKEN>", "doc_type": "slides", "file_extension": "pdf", "output_dir": "./exports"}

# 指定本地文件名（会按导出格式自动补扩展名）
drive.+export {"token": "<DOCX_TOKEN>", "doc_type": "docx", "file_extension": "pdf", "file_name": "weekly-report.pdf", "output_dir": "./exports"}

# 导出电子表格或多维表格为 csv 时，必须传 sub_id
drive.+export {"token": "<SHEET_OR_BITABLE_TOKEN>", "doc_type": "<sheet|bitable>", "file_extension": "csv", "sub_id": "<SUB_ID>", "output_dir": "./exports"}

# 导出多维表格为 .base 快照（只支持 bitable）
drive.+export {"token": "<BITABLE_TOKEN>", "doc_type": "bitable", "file_extension": "base", "output_dir": "./exports"}

# 导出多维表格结构为 .base 快照（仅导出表结构，不导出记录数据）
drive.+export {"token": "<BITABLE_TOKEN>", "doc_type": "bitable", "file_extension": "base", "only_schema": true, "output_dir": "./exports"}

# 允许覆盖已存在文件
drive.+export {"token": "<DOCX_TOKEN>", "doc_type": "docx", "file_extension": "pdf", "overwrite": true}
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `url` | 与 `token` 二选一 | 源文档 URL，推荐优先使用；CLI 自动解析类型和 token，Wiki URL 会解析到底层 `obj_token/obj_type` |
| `token` | 与 `url` 二选一 | 源文档裸 token；裸 token 必须同时传 `doc_type`。裸 Wiki node token 必须传 `doc_type=wiki`，CLI 会先解析到底层 `obj_token/obj_type` |
| `doc_type` | 条件必填 | 源文档类型：`doc` / `docx` / `sheet` / `bitable` / `slides` / `wiki`；仅当使用裸 `token` 时必填，使用 `url` 时自动推断。`wiki` 只用于裸 Wiki node token，解析后会按真实底层类型发起导出 |
| `file_extension` | 是 | 导出格式：`docx` / `pdf` / `xlsx` / `csv` / `markdown` / `base` / `pptx` |
| `sub_id` | 条件必填 | 当 `sheet` / `bitable` 导出为 `csv` 时必填 |
| `only_schema` | 否 | 仅当 `doc_type bitable file_extension base` 时可用；只导出多维表格结构，不导出记录数据 |
| `file_name` | 否 | 覆盖默认本地文件名；如未带扩展名，会按 `file_extension` 自动补齐 |
| `output_dir` | 否 | 本地输出目录，默认当前目录 |
| `overwrite` | 否 | 覆盖已存在文件 |

## 关键约束

- 推荐优先传 `url`，不要从 URL 手工拆 token 和 type；尤其是 Wiki URL，CLI 会自动解包到底层资源
- `url` 和 `token` 互斥
- 裸 `token` 必须传 `doc_type`；裸 Wiki node token 使用 `doc_type=wiki`
- `doc` 支持导出为 `docx` / `pdf`
- `docx` 支持导出为 `docx` / `pdf` / `markdown`
- `sheet` 支持导出为 `xlsx` / `csv`
- `bitable` 支持导出为 `xlsx` / `csv` / `base`
- `slides` 支持导出为 `pptx` / `pdf`
- `csv` 只支持 `sheet` / `bitable`，且必须带 `sub_id`
- `only_schema` 只支持 `bitable` 导出为 `.base`，用于仅导出表结构
- 如果格式不匹配，CLI 会返回 typed validation error，并在 `hint` 中给出可重试的 `file_extension` 建议；例如 `docx + csv` 会提示改用 `docx/pdf/markdown`，或改传 sheet/bitable URL
- shortcut 内部固定有限轮询：最多 10 次，每次间隔 5 秒
- 创建导出任务时收到 `rate_limit` / `99991400` 不会生成 `ticket`；至少等待 1 分钟后重跑原 `drive.+export`，持续限频时从 1 分钟开始指数退避
- 状态轮询一旦收到 `rate_limit` / `99991400` 会立即停止，不会继续消耗剩余轮询次数；错误会保留原始 typed metadata，并在 `hint` 中提供已有 `ticket` 的续查命令
- 轮询超时不是失败；会返回 `ticket`、`timed_out=true` 和 `next_command`，供后续继续查询

## 错误码处理

| 错误码 | 含义 | 处理方式 |
|--------|------|----------|
| `1069914` | token 非法或 token/type 不匹配；常见原因是把 Wiki node token 当作底层 `docx` / `sheet` / `bitable` token 使用，没有传 `doc_type=wiki` | 优先改用 `url <Wiki URL>`；只有裸 Wiki token 时，用 `token <WIKI_NODE_TOKEN> doc_type wiki`。不确定 token 类型时，先用 `drive.+inspect`（url="<TOKEN>"，type="wiki"） 检查是否能解包为 Wiki node；如果不是 Wiki token，再检查 token 来源、`doc_type` 是否与实际资源类型一致 |
| `99991400` / `rate_limit` | OpenAPI 请求频率受限 | 立即停止并按错误 `hint` 处理：没有 `ticket` 时，至少等待 1 分钟后重跑原 `drive.+export`；已有 `ticket` 时，只执行 `drive.+task_result`（scenario="export"） 续查，不要重复创建任务。持续限频时从 1 分钟开始指数退避 |
| `9499` + `too many request(s)` | 导出任务接口的另一种限频响应；同一个 `9499` 在其它 Drive 接口也可能表示参数类型错误，CLI 会结合服务端消息区分 | 按 `rate_limit` 处理：立即停止，等待至少 1 分钟并指数退避；已有 `ticket` 时只续查该任务，不要重新创建 |

## 推荐续跑方式

```text
# 第一步：先尝试直接导出
drive.+export {"url": "<DOCX_URL>", "file_extension": "pdf", "file_name": "weekly-report.pdf"}

# 如果返回 ready=false / timed_out=true，再继续查
drive.+task_result {"scenario": "export", "ticket": "<TICKET>", "file_token": "<DOCX_TOKEN>"}

# 查到 file_token 后下载
drive.+export-download {"file_token": "<EXPORTED_FILE_TOKEN>", "file_name": "weekly-report.pdf", "output_dir": "./exports"}
```

## 参考

- `drive` 领域指南 -- 云空间（云盘/云存储）全部命令
