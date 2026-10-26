> 由 lark-cli `7f70de0666a2` 的 `lark-meeting` 参考文档转换；调用约定见 `vc` 领域指南（`feishu_guide` 的 domain 参数）。

# 生成和修改妙记、管理妙记权限

## 从本地音视频生成妙记

标准链路是 Drive 上传 → Minutes 创建 → 按需读取产物。不要改用 ffmpeg、Whisper 或其他本地 ASR。

### 上传音视频到 Drive

确认本地媒体路径以及用户最终需要妙记链接、逐字稿、总结、待办还是章节。源文件须符合 `minutes.+upload`（网关不提供：仅支持用户身份）（主题 `lark-minutes-upload`） 的格式要求，时长不超过 6 小时，大小不超过 6 GB。

按 `drive` 领域指南 的路径和写操作规则执行 `drive.+upload`，取得 `file_token`。上传参数和文件限制见 `lark-drive-upload`（主题 `lark-drive-upload`）。

### 使用 file_token 创建妙记

从返回的 `minute_url` 路径最后一段提取 `minute_token`，去掉 query 参数。创建参数、支持格式和异步语义见 `lark-minutes-upload`（主题 `lark-minutes-upload`）。`minutes.+upload`（网关不提供：仅支持用户身份） 成功仅表示异步创建请求已提交；报告返回的 `minute_url` 和可解析的 `minute_token`。未执行 `minutes.+detail` 并确认就绪前，不得声称妙记产物已生成或可用。用户只要求发起创建或返回链接时，到此停止。

### 等待并读取妙记产物

上传后立即读取产物时必须加 `--wait-ready`：

将 `--transcript` 替换或扩展为用户需要的 `--summary`、`--todo`、`--chapter` 或 `--keyword`。创建任务仍在处理中时，按返回状态和重试提示轮询；不要重复上传或重复创建妙记。

用户要求独立提炼或复盘时，读取 Transcript 并基于原始发言分析，不要复述现成 Summary。产物 flags 和等待行为见 `lark-minutes-detail`（主题 `lark-minutes-detail`）。

### 从失败阶段恢复

- Drive 上传成功但 Minutes 创建失败：保留并报告 `file_token`，从创建妙记继续，不要重复上传。
- Minutes 创建成功但产物未就绪：保留 `minute_token` 并重试查询，不要重新创建妙记。
- 不得把 Drive 上传成功误报为妙记创建成功；明确报告失败发生在上传、创建还是产物生成阶段。

## 增删改 AI 待办

妙记 AI 待办不是飞书任务。上下文包含妙记 URL / `minute_token` 并要求修改妙记待办时，禁止改走 `task` 领域；用户同时指定了负责人（包括"负责人是我"）也不改变归属。

- 多条新增优先使用 `--todos` 批量提交。
- 待办 ID、批量结构和部分成功语义见 `lark-minutes-todo`（主题 `lark-minutes-todo`）。

### 指定负责人

妙记待办表示负责人的既定写法是把 `@姓名` 作为纯文本写进待办内容，不存在独立的负责人字段：

- 用户直接给出姓名时不做任何查找，原文拼成 `@姓名`。
- 姓名解析只影响追加的 `@` 文本，绝不能阻塞或取消待办创建；不要为处理负责人改走 `task` 领域 或做进一步通讯录搜索。
- 回复只陈述结果（妙记、待办内容、负责人、完成状态），不要解释接口字段限制，也不要建议改用 `task` 领域 来"明确负责人"。

完整规则和示例见 `lark-minutes-todo`（主题 `lark-minutes-todo`） 的「负责人 / `@` 提及」。

## 批量替换逐字稿关键词

`minutes.+word-replace`（网关不提供：仅支持用户身份） 修改的是妙记「转写／逐字稿／文字记录」中的字面词条（同一产物的不同叫法）。

`--minute-token` 必须是妙记 token（例如 `obcn...`），合法输入只有两种：`minutes.+search` / `vc.+recording` 等接口返回的 `token` 字段，或从妙记 URL 里提取最后一段路径并去掉 query 参数后的裸 token。禁止直接传妙记标题、URL 里的 slug、会议主题或完整妙记 URL；不确定时先跑 `minutes.+search` / `vc.+recording` 拿到 `minute_token` 再执行替换。

多组替换放在同一个 JSON 数组中。具体参数运行 `minutes.+word-replace`（网关不提供：仅支持用户身份）。

用户给出原词和目标词后直接替换：不要为了核对写法先读取 Transcript，也不要在替换成功后回读 Transcript 验证。接口逐词返回结果，按结果回报即可。

只要有一个关键词命中就是成功：`data.message` 列出 Succeeded 和 Failed 关键词。重试时只提交 Failed 的词，不要重复提交已成功的词，否则会把新词再替换一遍。

全部关键词都没命中才是失败（`not_found`）。这是参数问题而不是权限问题；如实告知用户哪些词没命中，请用户确认精确写法、大小写和空格后再决定是否重试，不要靠读取 Transcript 自行猜词。

## 替换逐字稿说话人

1. 调用 `GET /open-apis/minutes/v1/minutes/<token>/transcript/speakerlist`（CLI 通用 API 调用，网关不提供） 取得 `speaker_id`。
2. 按原说话人的显示名称精确匹配。存在同名候选时，结合 Transcript 展示候选并让用户确认，不要擅选。
3. 用户只提供目标姓名时，用 `contact` 领域指南 解析为 `ou_` open_id。

完整流程和参数见 `lark-minutes-speaker-replace`（主题 `lark-minutes-speaker-replace`）。

## 查看妙记授权列表

用户要查看妙记已授权给哪些成员，或查询某个成员当前的查看 / 编辑权限时，使用 Drive 协作者列表；这不是读取妙记内容，也不是为当前身份申请权限。先读取 `drive` 领域指南 和 `drive.+member-list`（主题 `lark-drive-member-list`）。

完整妙记 URL 可自动推断资源类型为 `minutes`；裸 `minute_token` 必须显式传 `type=minutes`。需要核对指定成员时，按 `member_id` 精确匹配返回的 `items[]`，不要按姓名或列表顺序猜测。

## 分配妙记权限

用户要求“把妙记分享给某人”“让某人可以查看 / 编辑”或“给某人授予权限”时，修改的是目标妙记的协作者权限，使用 `drive.+member-add`；禁止使用 `minutes.+apply-permission`，后者只为当前调用身份向妙记所有者申请权限。

先读取 `drive` 领域指南 和 `drive.+member-add`（主题 `lark-drive-member-add`）。目标成员只有展示名时，按 `contact` 领域指南 将其唯一解析为对应 ID；存在多个候选时请用户选择，不得猜测。

完整妙记 URL 可自动推断资源类型为 `minutes`；裸 `minute_token` 必须显式传 `type=minutes`。根据用户要求选择 `view` 或 `edit`；妙记不支持 `full_access`。只有妙记、目标成员和权限档位均已明确，且用户已明确要求执行授权时，才传 管理员预授权（high_risk_operations）。

写入后使用 `drive.+member-list` 按 `member_id` 回读验证。只有返回的目标成员权限与用户要求一致时，才能声明分配完成；若目标成员已有不同权限，不要仅根据 `member-add` 回执声称权限已被覆盖或降级。

## 为当前身份申请妙记权限

没有查看或编辑权限时，先说明权限事实。只有用户明确要求申请权限时才执行：

根据用户目标选择 `view` 或 `edit`，并必须沿用触发无权错误时的身份。这只是发起申请，不代表已经获得权限。身份和权限语义见 `lark-minutes-apply-permission`（主题 `lark-minutes-apply-permission`）。

## ASR/AI 额度不足

`minutes.+upload`（网关不提供：仅支持用户身份）、`minutes.+summary`（网关不提供：仅支持用户身份）、`minutes.+todo`（网关不提供：仅支持用户身份） 和 `minutes.+word-replace`（网关不提供：仅支持用户身份） 都可能返回 `quota_exceeded`，表示 ASR/AI 额度已耗尽。`+upload` 是额度不足以转写这个音视频，妙记根本没有创建；其余三个是该妙记生成时额度就已用尽、AI 产物未完整生成，写操作无法落库。

## 确认修改结果

修改前只读取目标相关字段，修改后用 `minutes.+detail` 或对应读取接口回读。命令自身已逐项返回写入结果时不再回读，例如 `minutes.+word-replace`（网关不提供：仅支持用户身份） 的 Succeeded / Failed 关键词。批量或多步修改逐项报告写前值、写后结果和失败原因；部分成功时不要回滚已成功项，除非命令明确承诺原子回滚。
