> 由 lark-cli `7f70de0666a2` 的 `lark-drive` 参考文档转换；调用约定见 `drive` 领域指南（`feishu_guide` 的 domain 参数）。

# `drive.+create-folder`（创建云空间/云盘/云存储文件夹）

## 命令

```text
# 在根目录创建文件夹
drive.+create-folder {"name": "周报归档"}

# 在指定父文件夹下创建子文件夹
drive.+create-folder {"folder_token": "<PARENT_FOLDER_TOKEN>", "name": "2026-W16"}

# 预览底层调用
drive.+create-folder {"folder_token": "<PARENT_FOLDER_TOKEN>", "name": "分析资料"}
```

## 返回值

成功后会返回一个 JSON 对象，常见字段包括：

- `folder_token`：新建文件夹 token，可直接用于后续 `drive.+move`、`drive.+upload` 等命令
- `url`：新建文件夹链接（如果接口返回）
- `name`：文件夹名称
- `parent_folder_token`：父文件夹 token；为空字符串表示创建在根目录

> [!IMPORTANT]
>
> 以应用身份创建时，结果里会额外返回 `permission_grant` 字段，明确说明授权结果：
> - `status = granted`：当前 CLI 用户已获得该文件夹的可管理权限
> - `status = failed`：文件夹已创建成功，但自动授权用户失败；会带上失败原因，并提示稍后重试或继续使用 bot 身份处理该文件夹
>
> `permission_grant.perm = full_access` 表示该资源已授予“可管理权限”。
>
> **不要擅自执行 owner 转移。** 如果用户需要把 owner 转给自己，必须单独确认。

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `name` | 是 | 文件夹名称，不能为空，最长 256 字节 |
| `folder_token` | 否 | 父文件夹 token；省略时表示在调用者根目录创建 |

## 行为说明

- **根目录创建**：不传 `folder_token` 时，shortcut 会向 API 显式传空字符串 `folder_token=""`，让后端按“根目录”语义创建
- **原生 API 仍可用**：如果用户明确要求按底层 API 字段调用，仍可继续使用 `drive.files.create_folder`

## 推荐场景

- 用户说“在云空间（云盘/云存储）新建一个文件夹 / 目录”时，优先使用 `drive.+create-folder`
- 用户给了父文件夹链接或 token，需要在其下继续分层建目录时，传 `folder_token`
- 如果后续还要上传文件、移动文件、建子目录，优先复用返回值里的 `folder_token`

> [!CAUTION]
> `drive.+create-folder` 是**写入操作**，执行前必须确认用户意图。

## 参考

- `drive` 领域指南 -- 云空间（云盘/云存储）全部命令
