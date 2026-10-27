> 由 lark-cli `7f70de0666a2` 的 `lark-slides` 参考文档转换；调用约定见 `slides` 领域指南（`feishu_guide` 的 domain 参数）。

# `slides.+replace-slide`（块级替换 / 插入）

对指定页面的已知块做替换或插入。先用 `slides.+xml-get` 获取最新 `block_id`，再用 `slides.+replace-slide` 写入；该操作不改变页面顺序。

`block_replace` 使用 `block_id` 和 `replacement`；追加元素使用 `block_insert` 和 `insertion`。完整 parts 结构、验证和限制见 主题 `lark-slides-replace-slide`。
