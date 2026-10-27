> 由 Feishu-MCP-Gateway 从 lark-cli `7f70de0666a2` 的 `lark-calendar` Skill 自动转换。
> - `docs.+fetch` 这类标识是 operation_id：具名视图调用对应工具（如 `feishu_docs_fetch`），紧凑视图用 `feishu_execute_read` / `feishu_execute_write` 传 operation_id 和 arguments。
> - CLI 参数 `--foo-bar` 对应 JSON 参数 `foo_bar`；代码块里的 `operation_id {…}` 行是参数示例。
> - 文中的“主题 `x`”用 `feishu_guide`（topic="x"）读取。
> - 网关固定以应用（bot）身份执行，没有 dry-run；高风险操作需要管理员预授权。
> - 本地文件改用 `{"file_id": "…"}`（先 `feishu_file_create` 或文件传输），输出路径填受控工作区内的相对名称；文中的本地脚本、jq、stdin 在网关中不可用。
> - 标注“网关不提供”的操作不能调用；参数以 `feishu_tools_describe` 或具名工具 schema 为准。
> - 操作结果中的 `next_command` 等后续命令是 CLI 写法，按上述规则换成对应的 operation_id 和参数再调用。

# calendar (v4)

**CRITICAL — 凡涉及预约日程/会议室、调整时间或查询/搜索会议室，第一步 MUST 读 主题 `lark-calendar-schedule-meeting`。仅编辑字段（改标题/描述）或增删参会人（不涉及时间和会议室）时可跳过，直接读 主题 `lark-calendar-update`。**

## 身份

按**日程归属**选身份：

- 查看/管理 bot 自己创建/拥有的日程 → 应用身份

**对话人称映射**：「我」= 登录用户，「你」= 应用（bot）；作为字段取值的人称（参会人、会议 owner 等）不参与身份判定，如「你创建日程，邀请我、会议 owner 为我」→ 应用身份 创建，登录用户仅作参会人与会议 owner。

```text
# bot 自建或参与的日程 → bot
calendar.+agenda
```

## Shortcuts

| Shortcut | 说明 |
|----------|------|
| `calendar.+agenda` | 查看日程安排（默认今天） |
| `calendar.+meeting`（网关不提供：仅支持用户身份）（主题 `lark-calendar-meeting`） | 通过日程事件 ID 获取关联的视频会议信息（meeting_id、meeting_note），日程开过视频会议才会有meeting_id,**注意**: 视频会议链接获取走+get命令 |
| `calendar.+create`（主题 `lark-calendar-create`） | 创建日程并邀请参会人（ISO 8601 时间） |
| `calendar.+update`（主题 `lark-calendar-update`） | 更新既有日程字段，或独立增量添加/移除参会人和会议室；重复性日程/例外必须传 `apply_to`（详见 重复性日程操作规范（主题 `lark-calendar-recurring`）） |
| `calendar.+delete` | 删除日程；重复性日程/例外必须传 `apply_to`（详见 重复性日程操作规范（主题 `lark-calendar-recurring`）） |
| `calendar.+freebusy` | 查询主日历的忙闲/RSVP状态/空闲时间段。(**如需预约/推荐时间段**走 `calendar.+suggestion`——它综合工作时间、忙碌区间和休息时间推荐。) |
| `calendar.+room-find`（主题 `lark-calendar-room-find`） | 针对一个或多个**明确的**时间块查找可用会议室（无明确时间时禁止直接调用，需先走 `calendar.+suggestion`） |
| `calendar.+rsvp`（主题 `lark-calendar-rsvp`） | 回复日程（接受/拒绝/待定） |
| `calendar.+join-event`（主题 `lark-calendar-join-event`） | 凭分享 token 加入日程（分享链接/二维码/分享卡片/RSVP 卡片） |
| `calendar.+suggestion`（主题 `lark-calendar-suggestion`） | 根据非明确时间或一段时间范围，推荐多个可用时间块方案 |
| `calendar.+transfer`（主题 `lark-calendar-transfer`） | 把日程组织者转让给另一个用户或机器人；不可逆，需 管理员预授权（high_risk_operations） |
| `calendar.+list-attendees`（主题 `lark-calendar-list-attendees`） | 列出日程的参与人和会议室（支持按 `type` 过滤：user / resource / chat / third_party） |

### `calendar.+get` — 单日程详情

通过 `calendar_id` + `event_id` 获取**单个日程**详情。

```text
# calendar_id不传，默认primary
calendar.+get {"calendar_id": "<calendar_id>", "event_id": "<event_id>"}
```

日程描述统一使用 `description` 一个字段，按 **Markdown** 富文本处理。读取日程时 `description` 返回 Markdown 富文本（仅有纯文本描述时返回该纯文本）；创建/更新日程时也通过 `description` 传入 Markdown。

> `calendar.+get` 返回不含参会人和会议室。需要参与人视角（用户 / 会议室 / 群 / 三方邮箱）请调用 `calendar.+list-attendees`（主题 `lark-calendar-list-attendees`）。

### `calendar.+search-event` — 按关键词、时间范围和参会人搜索日程

仅返回基础字段（`event_id`/`summary`/`start`/`end` 等），需要详情请走 `calendar.+get`。

```text
# page-size 每页数量，默认 30 可选
calendar.+search-event {"query": "周会", "start": "2026-04-20", "end": "2026-04-27", "attendee_ids": "ou_user1,oc_chat1,omm_room1", "page_token": "<page_token>", "page_size": "30"}
```

`attendee_ids` 的多值语义：**同类型内为 OR（并集）**——只要日程命中列表中的任意一个同类型 ID，就会返回。

- `attendee_ids=ou_A,ou_B` = A **或** B 参加的日程（**不是** A 和 B 都参加的）。

### `calendar.+delete` — 删除日程

```text
# calendar_id不传，默认primary
calendar.+delete {"calendar_id": "<calendar_id>", "event_id": "<event_id>", "notify": true}
```

### `calendar.+agenda` — 查看近期日程安排

默认查询当天。结果应整理为按日期分组、按开始时间升序的易读时间线。

```text
# calendar-id 日历 ID（默认primary）可选
calendar.+agenda {"start": "2026-03-10", "end": "2026-03-17", "calendar_id": "<calendar_id>"}
```

注意：
- 已取消的日程自动过滤；无日程时直接告知"日程清空"。
- 时间范围超过 40 天会自动拆分查询并合并结果。

### `calendar.+freebusy` — 查询主日历忙闲时段 / 事件 / 公共空闲

`calendar.+freebusy` 一个入口承担四种视角：几何计算类（`busy` / `free` / `common_free`）走自动合并；事件维度类（`raw_busy`）保留每条上游日程 + `rsvp_status`。

```text

# 查询忙碌时间段（去重并合并相邻/重叠段）
calendar.+freebusy {"start": "2026-03-11", "end": "2026-03-11", "user_id": ["ou_a", "ou_b"], "type": "busy"}

# 看别人有几个会、每个会的起止 + rsvp（不合并相邻/重叠段，带rsvp状态）
calendar.+freebusy {"start": "2026-03-11", "end": "2026-03-11", "user_id": ["ou_a", "ou_b"], "type": "raw_busy"}

# 查询用户空闲时间段
calendar.+freebusy {"start": "2026-03-11", "end": "2026-03-11", "user_id": ["ou_a", "ou_b"], "type": "free"}

# 多人公共空闲时间段（推荐替代手工合并）
calendar.+freebusy {"start": "2026-03-11T09:00:00+08:00", "end": "2026-03-11T18:00:00+08:00", "user_id": ["ou_a", "ou_b"], "type": "common_free", "min_duration": "30m"}
```

用法提示：
- **`calendar.+freebusy` 只适用于查询忙碌/空闲时间段这一事实**。如果目标是"给会议**推荐**一个合适的时间段"（单人或多人），必须优先使用 `calendar.+suggestion`（主题 `lark-calendar-suggestion`）——它会综合**工作时间段、忙碌时间段、休息时间段**来推荐，`calendar.+freebusy` 只回答"哪些区间空着"，不判断该区间是否适合排会。
- **多人公共空闲**：只想拿"哪些区间共同没被占"→ `type common_free [min_duration <dur>]`；想拿"推荐的会议时间段"→ 走 `calendar.+suggestion`。

## 前置条件路由

> **先判断是否重复性日程**：若操作对象是重复性日程，必须先读 重复性日程操作规范（主题 `lark-calendar-recurring`），并在用户未明确范围时先确认「仅此次/全部/此次及后续」（不要默认仅此次），再按下表进入具体操作流程。

| 场景 | 前置要求 |
|------|----------|
| 预约日程/会议、调整时间、查会议室 | 先读 主题 `lark-calendar-schedule-meeting` |
| 仅编辑字段（标题/描述）或增删参会人 | 先定位 `event_id`，再读 主题 `lark-calendar-update` |
| 调用任何 Shortcut | 先读其对应 reference 文档 |

## 写操作反馈

创建、更新、删除、RSVP 等写操作完成后，直接基于命令返回结果反馈用户；不要为了“确认是否生效”主动发起二次查询。只有用户明确要求复查，或命令返回信息不足以回答用户问题时，才需要再查询。

## 核心概念

- **日程实例（Instance）**：重复性日程展开后的具体时间实例。「仅此次」操作时使用具体实例的 `event_id`；「全部」或「此次及后续」操作时需对原重复性日程操作（使用原日程 `event_id`），并按需处理例外。
- **重复性日程例外（Exception）**：对重复性日程某次实例做过「仅此次」编辑后产生的独立日程（拥有独立 `event_id`）。删除/更新「全部」时必须同时处理例外，否则例外会残留。
- **全天日程（All-day Event）**：只按日期占用、没有具体起止时刻的日程，结束日期是包含在日程时间内的。
- **时间块 vs 时间范围**：时间块是具体确定的连续时间段（如 `14:00~15:00`），时间范围是泛指（如"今天下午"）。`calendar.+room-find` 必须基于确定时间块，不能基于模糊范围。
- **会议室（Room）**："room"不是"房间"，是"会议室"。会议室是日程的一种参与人（resource attendee），不能脱离日程单独预定。
- **日程会议 ID（Meeting ID）**：日程的历史视频会议 ID，在日程上开过视频会议才会有。
- **日程分享链接 vs 会议链接**：两者是不同事物，不可混用。
  - 日程分享链接：`https://<domain>/calendar/share?token=<token>`，指向日程本身，用于分享日程详情。**分享日程给某个人、某个群或粘贴到文档中，需要的都是这个日程分享链接（通过 `calendar.events.share_info` 获取），不是 applink**；禁止自己拼接 applink 或用 applink 代替。
  - 会议链接：`https://<domain>/j/<number>`，指向视频会议入口；同一重复性日程序列的所有实例共用同一个会议链接。

## 术语映射

用户日常说的"帮我约个日历""查一下今天的日历"，实际意图是针对**日程（Event）**的创建或查询，而非操作日历（Calendar）容器本身。自动将口语化的"日历"意图映射为"日程"操作。

## 意图路由

**日程与会议的关系**：用户口中的「会议」通常不区分日程和视频会议。定义、三种查询意图（当前/未来/过去）的分流规则见 日程与视频会议的关系（主题 `lark-calendar-meeting-relation`）。

| 用户意图 | 路由到 |
|----------|--------|
| 查询过去的会议（"昨天的会议""上周的会"）/今天有哪些会议 / 当前正在开的会议 | 先读 日程与视频会议的关系（主题 `lark-calendar-meeting-relation`） |
| 未来的会议 / 明天/下周的会议 | 本 skill：视频会议不存在于未来，等价于查日程 |
| 按关键词搜索日程 | 本 skill（`calendar.+search-event`） |
| 从日程获取关联的视频会议 ID 或用户绑定的会议纪要文档 | 本 skill（`calendar.+meeting`（网关不提供：仅支持用户身份）（主题 `lark-calendar-meeting`）） |
| 查看日程的参会人 / 会议室（含 `type=resource` 只看会议室） | 本 skill（`calendar.+list-attendees`（主题 `lark-calendar-list-attendees`）） |
| 把日程分享给某人 / 群 / 粘贴到文档 | 本 skill：先 `calendar.events.share_info` 取**日程分享链接**，再走 `im` 领域指南 发送或粘贴该链接；**分享日程给某个人、某个群或粘贴到文档中，需要的都是日程分享链接，不是 applink**，不要自己拼接或用 applink 代替 |
| 从日程进一步拿 AI 智能纪要 / 逐字稿 / 妙记产物 | 先 `calendar.+meeting`（网关不提供：仅支持用户身份） 取 `meeting_id`，再进入 `vc` 领域指南：`vc.+detail`（主题 `lark-vc-detail`） → `note.+detail`（主题 `lark-note-detail`） / `minutes.+detail`（主题 `lark-minutes-detail`） |
| 预约/改约日程、调整时间、添加/更换会议室、查会议室 | 先判断新建 vs 编辑，再进入 schedule-meeting 工作流（主题 `lark-calendar-schedule-meeting`） |
| 仅编辑日程字段（标题/描述）或增删参会人（不涉及时间和会议室） | 先定位 `event_id`，再读 `calendar.+update`（主题 `lark-calendar-update`） 执行变更 |
| 编辑/删除重复性日程（「改这个重复日程」「删掉后面的」「全部取消」等） | 先读 重复性日程操作规范（主题 `lark-calendar-recurring`）；`calendar.+update` / `calendar.+delete` 均通过 `apply_to=single|all|this-and-following` 指定范围 |

## 任务类型分流

处理"预约/改约日程、添加/移除参会人、添加/更换会议室、调整时间"时，必须先判断新建 vs 编辑：

- **编辑已有日程的强信号**：用户提到已存在的日程锚点（标题、时间段、`这个日程`、`这场会`）并表达修改动作（添加、移除、改到、换会议室、调整时间）。默认走编辑流，绝不能按新建处理。
- **新建日程**：用户表达新增意图（"新约一个会""创建一个日程""安排一次会议"），且没有指向既有日程的修改动作。

## 时间推断规范

- **星期的定义**：周一是一周的第一天，周日是最后一天。计算"下周一"等相对日期时，基于当前真实日期推算。
- **一天的范围**：用户提到"明天""今天"等泛指某天时，时间范围应覆盖整天，不要自行缩减。
- **历史时间约束**：不能预约已经完全过去的时间。唯一例外是"跨越当前时间"的日程（开始在过去、结束在未来）。

## 会议室规则

- 凡是"预定/查询/搜索可用会议室"，都必须进入 schedule-meeting 工作流（主题 `lark-calendar-schedule-meeting`），会议室参数规范详见 `calendar.+room-find`（主题 `lark-calendar-room-find`）。
- `calendar.+room-find` 的时间输入必须是确定时间块，不能是时间区间搜索。
- 用户仅要求"查会议室"但未提供明确时间时，必须先调用 `calendar.+suggestion` 获取可用时间块，再将时间块交给 `calendar.+room-find`。严禁猜测时间盲目调用。
- 编辑已有日程时，"添加会议室"默认是增量语义，保留已有会议室；只有用户明确说"更换会议室""移除会议室"时才删除旧会议室。

## API Resources

```bash
# 通用调用格式
calendar.<resource>.<method> {参数}

# 查询用户主日历
calendar.calendars.primary

# 返回形如 {{domain}}/calendar/share?token=<token> 的分享链接，不是 applink；直接把该链接发给对方（对方可凭链接中的 token 走 +join-event 加入）
calendar.events.share_info {"calendar_id": "<calendar_id>", "event_id": "<event_id>"}

# 删除日程
calendar.events.delete {"calendar_id": "<calendar_id>", "event_id": "<event_id>"}
```

> `calendar_id` 可以直接传 `primary`，代表当前调用身份的主日历 ID。

### 查询资源的方法列表以及方法的使用方式

- 列出某资源下的方法：`feishu_tools_describe`
- 查看方法的cli flag：`calendar.<resource>.<method> {参数} -h`
- 查看方法API参数：`feishu_tools_describe（operation_id="calendar.<resource>.<method>"）`

`<resource>` 为 `calendars`（日历本身）/ `events`（日程）/ `event.attendees`（参与人）/ `freebusys`（忙闲）。例：`feishu_tools_describe（operation_id="calendar.events.delete"）`。

## 常用其他域命令

```bash

# 搜索群聊，更多参数详见 lark-im
```

## 不在本 skill 范围

- 查询过去的视频会议记录 → `vc` 领域指南
- 待办任务管理 → `task` 领域指南
- 通讯录 → `contact` 领域指南
- 即时通讯 → `im` 领域指南
- 会议室物理设施管理 → 管理员后台

**注意（强制性）：**
- 涉及日期（时间）字符串与时间戳的相互转换时，务必调用系统命令或脚本代码等外部工具进行处理，以确保转换的绝对准确；换算**禁止依赖容器默认时区**（常为 UTC，会导致 8 小时偏移），必须显式指定目标时区。违者将导致严重的逻辑错误！
