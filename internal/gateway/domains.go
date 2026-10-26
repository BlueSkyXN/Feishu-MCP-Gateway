// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

// Domain 是面向 Agent 的业务领域：ID 与 Operation 的服务前缀一致。
// Skill 指向导入的上游指南；Featured 是该领域常见任务的入口操作，用于总览、搜索排序和 Skill 路由表。
type Domain struct {
	ID       string
	Title    string
	Summary  string
	Skill    string
	Aliases  []string
	Featured []string
}

var domains = []Domain{
	{ID: "docs", Title: "云文档", Summary: "读取、创建、编辑云文档（Docx）", Skill: "lark-doc",
		Aliases:  []string{"文档", "云文档", "docx", "doc", "document", "documents"},
		Featured: []string{"docs.+fetch", "docs.+create", "docs.+update"}},
	{ID: "sheets", Title: "电子表格", Summary: "读写单元格、工作表结构、图表、透视表与导入导出", Skill: "lark-sheets",
		Aliases:  []string{"表格", "电子表格", "工作表", "单元格", "spreadsheet", "spreadsheets", "sheet", "excel", "xlsx", "csv", "cell", "cells"},
		Featured: []string{"sheets.+workbook-info", "sheets.+csv-get", "sheets.+cells-get", "sheets.+csv-put", "sheets.+table-put", "sheets.+cells-set"}},
	{ID: "base", Title: "多维表格", Summary: "数据表、字段、记录、视图与仪表盘", Skill: "lark-base",
		Aliases:  []string{"多维表格", "多维表", "数据表", "bitable", "base", "record", "records"},
		Featured: []string{"base.+table-list", "base.+field-list", "base.+record-list", "base.+record-search", "base.+record-get", "base.+record-upsert", "base.+record-batch-create"}},
	{ID: "drive", Title: "云空间", Summary: "文件与文件夹、上传下载、导入导出、评论与权限", Skill: "lark-drive",
		Aliases:  []string{"云空间", "云盘", "云存储", "文件夹", "drive", "folder", "folders"},
		Featured: []string{"drive.+search", "drive.+upload", "drive.+download", "drive.+export", "drive.+import", "drive.files.list"}},
	{ID: "wiki", Title: "知识库", Summary: "知识空间、节点层级与成员", Skill: "lark-wiki",
		Aliases:  []string{"知识库", "知识空间", "wiki", "节点"},
		Featured: []string{"wiki.+node-get", "wiki.+node-list", "wiki.+space-list", "wiki.+node-create"}},
	{ID: "im", Title: "消息与群聊", Summary: "发送和回复消息、读取聊天记录、管理群聊、下载消息资源", Skill: "lark-im",
		Aliases:  []string{"消息", "群聊", "聊天", "群", "im", "chat", "chats", "message", "messages"},
		Featured: []string{"im.+messages-send", "im.+messages-reply", "im.+chat-messages-list", "im.+messages-mget", "im.+chat-search", "im.+messages-resources-download"}},
	{ID: "calendar", Title: "日历", Summary: "日程、参会人、忙闲与会议室", Skill: "lark-calendar",
		Aliases:  []string{"日历", "日程", "会议室", "忙闲", "calendar", "schedule", "agenda"},
		Featured: []string{"calendar.+agenda", "calendar.+create", "calendar.+update", "calendar.+freebusy", "calendar.+room-find"}},
	{ID: "task", Title: "任务", Summary: "任务、子任务、清单与提醒", Skill: "lark-task",
		Aliases:  []string{"任务", "待办", "清单", "task", "tasks", "todo", "tasklist"},
		Featured: []string{"task.+create", "task.+update", "task.+complete", "task.+comment", "task.+tasklist-create"}},
	{ID: "mail", Title: "邮箱", Summary: "邮件、会话、草稿与收信规则", Skill: "lark-mail",
		Aliases:  []string{"邮件", "邮箱", "mail", "email", "mailbox"},
		Featured: []string{"mail.+messages", "mail.+message", "mail.+thread", "mail.+triage"}},
	{ID: "okr", Title: "OKR", Summary: "周期、目标、关键结果与进展", Skill: "lark-okr",
		Aliases:  []string{"okr", "目标", "关键结果", "objective", "objectives"},
		Featured: []string{"okr.+cycle-list", "okr.+cycle-detail", "okr.+progress-create"}},
	{ID: "slides", Title: "幻灯片", Summary: "创建和编辑演示文稿与页面", Skill: "lark-slides",
		Aliases:  []string{"幻灯片", "演示文稿", "ppt", "slides", "slide", "presentation"},
		Featured: []string{"slides.+create", "slides.+xml-get", "slides.+add-slide", "slides.+update-slide"}},
	{ID: "vc", Title: "视频会议", Summary: "会议记录、会议产物与进行中的会议", Skill: "lark-meeting",
		Aliases:  []string{"视频会议", "会议", "vc", "meeting", "meetings"},
		Featured: []string{"vc.+search", "vc.+detail", "vc.+notes", "vc.+recording"}},
	{ID: "minutes", Title: "妙记", Summary: "妙记搜索、详情与媒体下载", Skill: "lark-meeting",
		Aliases:  []string{"妙记", "录制", "转写", "minutes", "transcript"},
		Featured: []string{"minutes.+search", "minutes.+detail", "minutes.+download"}},
	{ID: "note", Title: "会议纪要", Summary: "按 note_id 读取纪要信息", Skill: "lark-meeting",
		Aliases:  []string{"纪要", "会议纪要", "note", "notes"},
		Featured: []string{"note.+detail"}},
	{ID: "whiteboard", Title: "画板", Summary: "查询、导出和更新文档中的画板", Skill: "lark-whiteboard",
		Aliases:  []string{"画板", "whiteboard", "board", "mermaid", "plantuml"},
		Featured: []string{"whiteboard.+query", "whiteboard.+update", "whiteboard.+export"}},
	{ID: "markdown", Title: "Markdown 文件", Summary: "云空间中的原生 Markdown 文件", Skill: "lark-markdown",
		Aliases:  []string{"markdown", "md"},
		Featured: []string{"markdown.+fetch", "markdown.+create", "markdown.+patch"}},
	{ID: "mindnotes", Title: "思维笔记", Summary: "思维笔记节点", Skill: "lark-doc",
		Aliases:  []string{"思维笔记", "思维导图", "mindnote", "mindnotes", "mindmap"},
		Featured: []string{"mindnotes.nodes.list", "mindnotes.nodes.create"}},
	{ID: "contact", Title: "通讯录", Summary: "按 ID 查询用户信息", Skill: "lark-contact",
		Aliases:  []string{"通讯录", "联系人", "contact", "contacts"},
		Featured: []string{"contact.+get-user"}},
	{ID: "attendance", Title: "考勤", Summary: "考勤打卡结果",
		Aliases:  []string{"考勤", "打卡", "attendance"},
		Featured: []string{"attendance.user_tasks.query"}},
	{ID: "application", Title: "应用斜杠命令", Summary: "当前应用的 / 命令",
		Aliases:  []string{"斜杠命令", "slash", "application"},
		Featured: []string{"application.+slash-command-list", "application.+slash-command-create", "application.+slash-command-update"}},
	{ID: "apps", Title: "妙搭应用", Summary: "妙搭用户 ID 与飞书 ID 转换",
		Aliases:  []string{"妙搭", "miaoda", "apps"},
		Featured: []string{"apps.+user-id-convert"}},
	{ID: "events", Title: "事件", Summary: "读取应用已接收的事件回调",
		Aliases:  []string{"事件", "回调", "订阅", "event", "events", "callback", "subscribe"},
		Featured: []string{"events.read"}},
}

func domainByID(id string) (Domain, bool) {
	for _, d := range domains {
		if d.ID == id {
			return d, true
		}
	}
	return Domain{}, false
}

// opDomain 返回操作所属领域，即 command 的第一段。
func opDomain(op Operation) string { return op.Command[0] }
