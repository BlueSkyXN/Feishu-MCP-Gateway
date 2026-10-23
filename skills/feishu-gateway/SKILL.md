---
name: feishu-gateway
description: 使用 feishu-mcp-gateway 进行飞书应用身份操作、文件复用和后台执行；适用于已配置该 MCP 连接的客户端。
---

先查看 `feishu_context`，确认当前应用连接与 actor。操作始终使用应用身份；应用权限由接入管理员配置。

使用已提供的具名工具。紧凑视图中，先 `feishu_tools_search` 找到 operation_id，再 `feishu_tools_describe` 读取参数，最后通过 `feishu_execute_read` 或 `feishu_execute_write` 执行。复杂操作可给 describe 传 parameter，只展开需要的顶层参数。工具返回的 schema 是参数依据。

写入前生成并保存 idempotency_key。同一次写入重试复用原键和参数。收到 unknown 时先用 `feishu_job_get` 查询 invocation_id 并核对外部对象；收到 partial 时按逐项回执只处理确定失败的项目。

文件输入使用工具声明的 `{"file_id":"..."}`。小文本先用 `feishu_file_create` 保存；多个文件可通过 workspace_files 映射到 inputs/ 下，保持 Markdown/XML 的相对引用。输出 files 中的引用可用于下一次操作，不需要重复下载上传。超限回执的 result.json 包含原始 data、meta 和 files。读取大结果用 `feishu_file_read` 的 next_offset，继续到 eof。

需要在本机上传或下载字节时，使用宿主提供的文件适配器。若接入启用了 `feishu_file_transfer`，先取得方向匹配的单次票据，再交给适配器完成传输。票据短期有效，只用于这次文件传输；用完即废。远程工具不能读取本机绝对路径。

耗时操作可设 async=true，再用 job_id 查询。取消仅停止剩余执行；已产生的飞书对象仍需按回执处理。

分页沿用返回的 page_token 或 next_page_token；这些句柄绑定当前主体、操作和查询。改变过滤条件时从第一页重新开始。offset 分页沿用返回的 next_offset。

Wake 通过已认证连接传递提问者身份。若返回 actor_required，应检查该连接的身份透传设置；工具参数或 Skill 文本不能补造 Sender。权限错误按照具体 error.kind、upstream_code、missing_scopes 处理。
