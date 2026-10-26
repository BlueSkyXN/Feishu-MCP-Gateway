> 由 Feishu-MCP-Gateway 从 lark-cli `7f70de0666a2` 的 `lark-okr` Skill 自动转换。
> - `docs.+fetch` 这类标识是 operation_id：具名视图调用对应工具（如 `feishu_docs_fetch`），紧凑视图用 `feishu_execute_read` / `feishu_execute_write` 传 operation_id 和 arguments。
> - CLI 参数 `--foo-bar` 对应 JSON 参数 `foo_bar`；代码块里的 `operation_id {…}` 行是参数示例。
> - 文中的“主题 `x`”用 `feishu_guide`（topic="x"）读取。
> - 网关固定以应用（bot）身份执行，没有 dry-run；高风险操作需要管理员预授权。
> - 本地文件改用 `{"file_id": "…"}`（先 `feishu_file_create` 或文件传输），输出路径填受控工作区内的相对名称；文中的本地脚本、jq、stdin 在网关中不可用。
> - 标注“网关不提供”的操作不能调用；参数以 `feishu_tools_describe` 或具名工具 schema 为准。

# okr (v2)

## 快速决策

| 用户需求                     | 操作路径                                                                                                                | 参考文档                                                                                                                                                                                                             |
|------------------------------|-------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 查看自己/他人的 OKR          | 获取用户 ID -> `okr.+cycle-list` -> `okr.+cycle-detail` -> 按需查指标/进展记录                                                  | `cycle-list`（主题 `lark-okr-cycle-list`）, `cycle-detail`（主题 `lark-okr-cycle-detail`）, `indicators`（主题 `lark-okr-indicators`）, `progress-list`（主题 `lark-okr-progress-list`） |
| 为自己写一组 OKR             | 优先用 `okr.+batch-create` 创建 Objective/KR 骨架                                                                           | `batch-create`（主题 `lark-okr-batch-create`）, `contentblock`（主题 `lark-okr-contentblock`）                                                                                                         |
| 只新增一条 O 或单条 KR       | 用 `okr.+create`                                                                                                            | `create`（主题 `lark-okr-create`）                                                                                                                                                                            |
| 编辑内容/备注/截止时间       | 用 `okr.+patch`                                                                                                             | `patch`（主题 `lark-okr-patch`）                                                                                                                                                                              |
| 修改 OKR 分数                | 只有用户明确说“分数”“评分”“打分”“score”时才用 `okr.+patch`；分数不是进度/完成度                                     | `patch`（主题 `lark-okr-patch`）                                                                                                                                                                              |
| 调整顺序或权重               | 用 `okr.+reorder` / `okr.+weight`                                                                                               | `reorder`（主题 `lark-okr-reorder`）, `weight`（主题 `lark-okr-weight`）                                                                                                                               |
| 更新数字进度/完成度          | 百分比或不带单位数字用 `okr.+indicator-update`；需要改单位/目标值时查指标后用 `indicators patch`                            | `indicator-update`（主题 `lark-okr-indicator-update`）, `indicators`（主题 `lark-okr-indicators`）                                                                                                     |
| 写文字进展                   | 用 `okr.+progress-create`；如果文本和数字都有，百分比或默认单位可使用 `progress_percent` 统一改，非百分比单位更新量化指标 | `progress-create`（主题 `lark-okr-progress-create`）, `progress-list`（主题 `lark-okr-progress-list`）, `progress-update`（主题 `lark-okr-progress-update`）                                    |
| 对齐目标                     | 直接按对齐关系工作流处理                                                                                                | `alignments`（主题 `lark-okr-alignments`）                                                                                                                                                                    |
| 查询/创建/修改/解决 OKR 评论 | 获取周期下全部评论聚合用 `okr.+comment-detail`，查询单个 O/KR/进展或仅查询周期全局评论用 `okr.+comment-list`；                | `comment`（主题 `lark-okr-comment-list`）, `comment-create`（主题 `lark-okr-comment-create`）, `comment-solve-reopen`（主题 `lark-okr-comment-solve-reopen`）                                   |

获取当前用户用 `contact.+get-user`；按姓名/邮箱查他人用 `contact.+search-user`（网关不提供：仅支持用户身份），拿到 `open_id` 后再查 OKR。

最常用 OKR 命令示例：

```bash

# 更新数字进度/完成度
```

分数和进度不要混用：用户说“进度”“完成度”“当前做到 75%”时，通常是在改量化指标或写进展记录，不是在改 `score`。只有明确要求修改 OKR 分数/评分/打分时，才使用 `okr.+patch`（主题 `lark-okr-patch`）；`score` 取值是 0-1，最多一位小数。

进度判断规则：用户说“进度”“完成度”时，先判断是否是量化数字。数字进度通常对应量化指标；不可量化文本对应进展记录。需要修改指标单位时看 `lark-okr-indicators.md`（主题 `lark-okr-indicators`）

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli okr +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut                                                         | 说明                                                                                                          |
|------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| `okr.+cycle-list`（主题 `lark-okr-cycle-list`）               | 分页获取特定用户的 OKR 周期列表，可以用 `time_range` 对当前页后置筛选                                       |
| `okr.+cycle-detail`（主题 `lark-okr-cycle-detail`）           | 获取特定 OKR 中所有目标和关键结果的内容                                                                       |
| `okr.+create`（主题 `lark-okr-create`）                       | 创建单个 Objective（可带备注），或向已有 Objective 新增 KR                                                    |
| `okr.+progress-list`（主题 `lark-okr-progress-list`）         | 分页获取目标或关键结果的进展记录列表                                                                          |
| `okr.+progress-get`（主题 `lark-okr-progress-get`）           | 根据 ID 获取单条 OKR 进展记录                                                                                 |
| `okr.+progress-create`（主题 `lark-okr-progress-create`）     | 为目标或关键结果创建进展记录                                                                                  |
| `okr.+progress-update`（主题 `lark-okr-progress-update`）     | 更新指定 ID 的进展记录内容                                                                                    |
| `okr.+progress-delete`（主题 `lark-okr-progress-delete`）     | 删除指定 ID 的进展记录（不可恢复）                                                                            |
| `okr.+upload-image`（主题 `lark-okr-image-upload`）           | 上传图片用于 OKR 进展记录的富文本内容                                                                         |
| `okr.+batch-create`（主题 `lark-okr-batch-create`）           | 批量创建 Objective（可带备注）和 KR                                                                           |
| `okr.+reorder`（主题 `lark-okr-reorder`）                     | 调整 Objective 或 KR 的顺位                                                                                   |
| `okr.+weight`（主题 `lark-okr-weight`）                       | 调整 Objective 或 KR 的权重                                                                                   |
| `okr.+indicator-update`（主题 `lark-okr-indicator-update`）   | 更新 Objective 或 KR 的当前进度指标。更复杂的量化指标操作见 量化指标管理（主题 `lark-okr-indicators`） |
| `okr.+patch`（主题 `lark-okr-patch`）                         | 部分更新 Objective 或 KR（content、notes、score、deadline）                                                   |
| `okr.+comment-detail`（主题 `lark-okr-comment-detail`）       | 获取周期下 Cycle/Objective/KeyResult/Progress 的全部评论                                                      |
| `okr.+comment-list`（主题 `lark-okr-comment-list`）           | 分页获取单个 OKR 实体下的评论                                                                                 |
| `okr.+comment-get`（主题 `lark-okr-comment-get`）             | 获取单条评论详情                                                                                              |

### 创建场景选择

- **单条创建优先用 `okr.+create`（主题 `lark-okr-create`）**：适合创建一个 Objective，或给已有 Objective 增加一个 KR。
- **批量创建用 `okr.+batch-create`（主题 `lark-okr-batch-create`）**：适合一次创建多个 Objective，并可同时附带多个 KR。
- 如果你只需要修改已有 Objective / KR 的内容、备注、分数或截止时间，使用 `okr.+patch`（主题 `lark-okr-patch`）。

## 格式说明

- `OKR 业务实体`（主题 `lark-okr-entities`） 获取 OKR 实体结构，定义和关系，帮助你更好的使用 OKR 功能
- `ContentBlock 富文本格式`（主题 `lark-okr-contentblock`） — Objective/KeyResult/Progress 中 Content/Note
  字段使用的富文本格式说明，以及简化的半纯文本（SemiPlainContent）格式的进一步说明。
- **强烈建议** 在操作 OKR 前，阅读`OKR 业务实体`（主题 `lark-okr-entities`）以了解基础概念

## API Resources

### alignments

- `delete` — 删除对齐关系
- `get` — 获取对齐关系

> **操作指南：** OKR 对齐关系管理（主题 `lark-okr-alignments`） 包含 list/create/delete 完整工作流

### categories

- `list` — 批量获取分类

### cycles

- `list` — 批量获取用户周期

### cycle.objectives

- `list` — 批量获取用户周期下的目标

### indicators

- `patch` — 更新量化指标

> **操作指南：** OKR 量化指标管理（主题 `lark-okr-indicators`） 包含目标/KR 指标查询和 patch 更新完整工作流

### key_results

- `delete` — 删除关键结果
- `get` — 获取关键结果
- `patch` — 更新关键结果

### key_result.indicators

- `list` — 获取关键结果的量化指标

> **操作指南：** OKR 量化指标管理（主题 `lark-okr-indicators`）

### objectives

- `delete` — 删除目标
- `get` — 获取目标
- `key_results_position` — 更新全部关键结果的位置
    - 请求中必须携带对应周期下全部关键结果的 ID，否则会参数校验失败。以传入的关键结果ID顺序重新排列关键结果。
- `key_results_weight` — 更新全部关键结果的权重
    - 类似 `objectives_weight`, 请求中必须同时修改对应目标下全部关键结果的权重，且所有权重值的和必须等于 1 ，否则会参数校验失败。
- `patch` — 更新目标

### objective.alignments

- `create` — 创建对齐关系
    - 对齐不允许对齐自己的目标，且发起对齐的目标和被对齐的目标所在周期时间上必须有重叠，否则会参数校验失败。
- `list` — 批量获取目标下的对齐关系

### objective.indicators

- `list` — 获取目标的量化指标

### objective.key_results

- `list` — 批量获取目标下的关键结果

## 不在本 skill 范围

- 待办任务管理 → 使用 `task` 领域指南
- 日程安排 → 使用 `calendar` 领域指南
- 绩效评估 → 使用 `lark-openapi-explorer`（CLI 专用指南，网关不提供） 查找原生接口
