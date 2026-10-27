// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	guidePageRunes    = 20000
	guidePageMaxRunes = 65536
)

// addGuide 注册 feishu_guide：领域总览、领域指南与主题。具名与紧凑视图都提供。
func (s *Service) addGuide(server *mcp.Server) {
	s.addUtility(server, "feishu_guide", "查看当前接入开放的飞书领域与用法指南。无参数返回领域总览和入口操作；domain 返回该领域指南；topic 返回指南中“主题 x”的具体做法。长内容按 next_offset 继续读取。", schemaObject(map[string]any{
		"domain": map[string]any{"type": "string", "maxLength": 64, "description": "领域 ID，例如 docs、sheets、base、im。"},
		"topic":  map[string]any{"type": "string", "maxLength": 128, "description": "指南中引用的主题 ID。"},
		"offset": map[string]any{"type": "integer", "minimum": 0},
		"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": guidePageMaxRunes},
	}, nil), true, func(ctx context.Context, p Principal, args map[string]any) Result {
		c, _, err := s.currentConfig().checkPrincipal(p, false)
		if err != nil {
			return failed(err)
		}
		views := s.visibleDomains(c, p)
		domain, _ := args["domain"].(string)
		topic, _ := args["topic"].(string)
		if domain == "" && topic == "" {
			return Result{Status: "succeeded", Data: map[string]any{"tool_view": c.ToolView, "domains": views, "usage": guideUsage(c)}}
		}
		var view *domainView
		for i := range views {
			if views[i].Domain == domain {
				view = &views[i]
			}
		}
		if domain != "" && view == nil {
			return failed(fault("domain_unavailable", "当前接入未开放该领域；调用 feishu_guide（无参数）查看可用领域"))
		}
		if topic != "" {
			skill, title, content, ok := s.Guidance.topic(topic)
			if !ok {
				return failed(fault("unknown_topic", "未知主题；主题 ID 见领域指南中的“主题 x”"))
			}
			allowed := false
			for _, v := range views {
				if meta, _ := domainByID(v.Domain); meta.Skill == skill && (domain == "" || v.Domain == domain) {
					allowed = true
				}
			}
			if !allowed {
				return failed(fault("domain_unavailable", "该主题不属于指定领域，或所属领域未向当前接入开放"))
			}
			data := pageText(content, args)
			data["topic"], data["title"] = topic, title
			return Result{Status: "succeeded", Data: data}
		}
		meta, _ := domainByID(domain)
		content, ok := s.Guidance.guide(meta.Skill)
		if meta.Skill == "" || !ok {
			content = s.operationList(c, p, meta)
		}
		data := pageText(content, args)
		data["domain"] = view
		if skill, ok := s.Guidance.Skills[meta.Skill]; ok && data["offset"] == 0 {
			data["topics"] = skill.Topics
		}
		return Result{Status: "succeeded", Data: data}
	})
}

func guideUsage(c Client) string {
	if c.ToolView == "compact" {
		return "带 domain 读领域指南；用 feishu_tools_search 查找操作，feishu_tools_describe 查看参数与用法，feishu_execute_read 或 feishu_execute_write 执行。"
	}
	return "带 domain 读领域指南；直接调用具名工具，operation_id docs.+fetch 对应工具 feishu_docs_fetch。"
}

// operationList 为没有上游指南的领域列出可发现的操作和要点。
func (s *Service) operationList(c Client, p Principal, meta Domain) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s（%s）\n\n%s。\n\n", meta.Title, meta.ID, meta.Summary)
	for _, op := range s.Catalog.Operations {
		if opDomain(op) != meta.ID || !discoverable(c, p, op) {
			continue
		}
		fmt.Fprintf(&b, "- `%s`（%s）：%s\n", op.ID, op.Risk, op.Description)
		if g, ok := s.Guidance.Operations[op.ID]; ok {
			for _, line := range append(append([]string{}, g.UseWhen...), g.Tips...) {
				fmt.Fprintf(&b, "  - %s\n", line)
			}
		}
	}
	return b.String()
}

func pageText(content string, args map[string]any) map[string]any {
	runes := []rune(content)
	offset, limit := 0, guidePageRunes
	// 在浮点范围内先截断：超出 int 范围的值转换结果随平台不同，amd64 上会变成负数。
	if n, ok := args["offset"].(float64); ok {
		offset = int(min(max(n, 0), float64(len(runes))))
	}
	if n, ok := args["limit"].(float64); ok {
		limit = int(min(max(n, 1), guidePageMaxRunes))
	}
	start := min(offset, len(runes))
	end := min(start+limit, len(runes))
	data := map[string]any{"content": string(runes[start:end]), "offset": start, "eof": end >= len(runes), "length": len(runes)}
	if end < len(runes) {
		data["next_offset"] = end
	}
	return data
}

// instructions 生成接入级的 MCP instructions：宿主会在会话开始时提供给模型。
// 领域与数量按接入配置统计；提问者维度在每次调用时再收窄。
func (s *Service) instructions(c Client) string {
	names, total := []string{}, 0
	for _, d := range s.visibleDomains(c, Principal{}) {
		names = append(names, d.Title+" "+d.Domain)
		total += d.Operations
	}
	var b strings.Builder
	b.WriteString("飞书操作固定以应用（bot）身份执行，可用范围由接入配置决定。")
	if total > 0 {
		fmt.Fprintf(&b, "本接入开放 %d 个操作，领域：%s。", total, strings.Join(names, "、"))
	}
	b.WriteString("不确定怎样完成任务时先调用 feishu_guide：无参数看领域与入口操作，带 domain 读领域指南，带 topic 读具体做法。")
	if c.ToolView == "compact" {
		b.WriteString("用 feishu_tools_search 查找 operation_id（中英文均可，可按 domain 过滤），用 feishu_tools_describe 查看参数与用法；已知 operation_id 和参数时直接调用 feishu_execute_read 或 feishu_execute_write。")
	} else {
		b.WriteString("直接调用具名工具，参数以工具 schema 为准；指南中的 operation_id docs.+fetch 对应工具 feishu_docs_fetch。")
	}
	b.WriteString("写入在参数中提供稳定的 idempotency_key，重试时复用同一键和参数；结果为 unknown 时先用 feishu_job_get 核对，partial 只处理失败项。")
	b.WriteString(`文件输入用 {"file_id":"…"}，小文本先用 feishu_file_create 保存；输出填相对名称，结果中的 files 可直接复用。分页沿用返回的 page_token 或 cursor。`)
	return b.String()
}

// toolDescription 是具名工具的描述：目录描述加上指南中的适用与不适用场景。
func (s *Service) toolDescription(op Operation) string {
	trim := func(v string) string { return strings.TrimRight(strings.TrimSpace(v), "。.；; ") }
	parts := []string{}
	if d := trim(op.Description); d != "" {
		parts = append(parts, d)
	}
	if g, ok := s.Guidance.Operations[op.ID]; ok {
		if len(g.UseWhen) > 0 {
			parts = append(parts, "适用："+trim(firstSentence(g.UseWhen[0], 200)))
		}
		if len(g.AvoidWhen) > 0 {
			parts = append(parts, "不适用："+trim(firstSentence(g.AvoidWhen[0], 160)))
		}
	}
	return strings.Join(append(parts, "执行身份固定为飞书应用；支持 async 登记后台操作。"), "。")
}
