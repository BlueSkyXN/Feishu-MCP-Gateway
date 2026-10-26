> 由 lark-cli `7f70de0666a2` 的 `lark-drive` 参考文档转换；调用约定见 `drive` 领域指南（`feishu_guide` 的 domain 参数）。

# 权限治理 Command Patterns

本文只提供 `permission_governance` workflow 的具体 `lark-cli` 命令样例。只有进入对应 state 且需要拼装命令时才读取本文；命令可用范围仍以 主题 `lark-drive-workflow-permission-governance`（主题 `lark-drive-workflow-permission-governance`） 的 `Command Map` 为准。

## 目录

- `目标解析`
- `目标发现`
- `事实读取`
- `写前确认与执行`

## 目标解析

`drive.+inspect` 支持 Drive folder，并且是受支持 Drive URL 的统一解析入口。对文件夹自身权限设置，先通过 `drive.+inspect` 解析 URL，或直接使用 `drive.+permission-get-setting`（token="<folder_url>"）；传 bare folder token 时必须显式传 `type=folder`。

`/wiki/space/<space_id>` URL 是 Wiki space 范围，不要用 `drive.+inspect` 当作单文档解析；直接提取 `space_id` 后进入 `DISCOVER_TARGETS`。

## 目标发现

发现 Wiki space / node 下目标：

解析返回时使用 `data.nodes`，不要读取顶层 `items`。`--page-limit 0` 表示当前层分页不设页数上限；`--page-all` 只覆盖当前 `space-id` / `parent-node-token` 范围内的分页，不会递归子节点。节点 `has_child=true` 时，必须继续以该节点的 `node_token` 作为 `--parent-node-token` 递归读取。

发现 Drive folder 下目标：

## 事实读取

读取 metadata：

读取权限设置：

裸 folder token 必须显式传 `type=folder`：

通过 URL 读取权限设置时可以省略 `type`：

按需读取直接协作者/授权成员列表：

`fields` 默认不传；只有需要名称、协作者类型、头像或外部标签时才显式传。它只声明期望返回的字段，不授予字段级权限：请求用户的 `name` / `avatar` 时还需 `contact:user.base:readonly`（“获取用户基本信息”）。字段权限或数据可见性不足时，接口仍可能成功但省略相应字段；缺字段不能解释为空值。

按需读取访问统计：

按需读取最近访问记录：

## 写前确认与执行

patch 前检查 manage-public permission：

patch 前读取当前 schema：

```text
# feishu_tools_describe（operation_id="drive.permission.public.patch"）
```

只 patch 当前 schema 支持的字段；对 Wiki 目标，必须省略 schema 明确标注为 Wiki 不支持的字段。

显式确认后 patch public permission：

显式确认后申请访问权限：

owner 转移前读取当前 schema：

```text
# feishu_tools_describe（operation_id="drive.permission.members.transfer_owner"）
```

显式确认后转移 owner：

`member_type` 只能使用当前 schema 支持的值：`email`、`openid`、`userid`、`appid`。如果用户只给姓名，必须先解析为明确身份或要求用户补充；不要猜测 `member_id`。批量 owner 转移必须逐个目标顺序执行。

secure label 写前枚举可用标签：

当用户给出的是标签名称、密级文案或不确定的 label ID 时，必须先枚举并解析为 `label-id`；写入确认里展示目标标签名称和 ID。找不到唯一标签时，停止并让用户选择，不要猜测。

显式确认后更新 secure label：
