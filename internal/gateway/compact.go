// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Service) addCompact(server *mcp.Server) {
	s.addUtility(server, "feishu_tools_search", "按任务或关键词查找当前可用的操作，中英文均可；返回 operation_id、领域和用途。可用 domain、risk 过滤；query 为空时按领域浏览。", schemaObject(map[string]any{
		"query":  map[string]any{"type": "string", "maxLength": 256},
		"domain": map[string]any{"type": "string", "maxLength": 64, "description": "只在该领域内查找，例如 docs、sheets、im；领域列表见 feishu_guide。"},
		"risk":   map[string]any{"type": "string", "enum": []string{"read", "write"}},
		"after":  map[string]any{"type": "string", "description": "上一页返回的 next_after。"},
		"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 50},
	}, nil), true, func(ctx context.Context, p Principal, args map[string]any) Result {
		c, _, err := s.currentConfig().checkPrincipal(p, false)
		if err != nil {
			return failed(err)
		}
		query, _ := args["query"].(string)
		domain, _ := args["domain"].(string)
		risk, _ := args["risk"].(string)
		if _, ok := domainByID(domain); domain != "" && !ok {
			return failed(fault("unknown_domain", "未知领域；调用 feishu_guide 查看当前开放的领域"))
		}
		offset, limit := 0, 20
		if after, _ := args["after"].(string); after != "" {
			n, err := strconv.Atoi(after)
			if err != nil || n < 0 {
				return failed(fault("invalid_cursor", "after 需要使用上次返回的 next_after"))
			}
			offset = n
		}
		if n, ok := args["limit"].(float64); ok {
			limit = int(n)
		}
		hits, partial := s.search.search(query, func(op Operation) bool {
			return discoverable(c, p, op) && (domain == "" || opDomain(op) == domain) && (risk == "" || (risk == "read") == (op.Risk == "read"))
		})
		start, end := min(offset, len(hits)), min(offset+limit, len(hits))
		entries := []any{}
		for _, h := range hits[start:end] {
			entries = append(entries, s.operationSummary(h.entry.op))
		}
		data := map[string]any{"operations": entries, "total": len(hits), "has_more": end < len(hits)}
		if end < len(hits) {
			data["next_after"] = strconv.Itoa(end)
		}
		if partial {
			data["match"] = "partial"
		}
		if len(hits) == 0 {
			data["hint"] = "没有匹配的操作。可换用英文关键词或领域名、按 domain 浏览，或调用 feishu_guide 查看当前开放的领域。"
			data["domains"] = s.visibleDomains(c, p)
		}
		return Result{Status: "succeeded", Data: data}
	})
	s.addUtility(server, "feishu_tools_describe", "读取操作的参数 schema、返回结构、用法指南（适用场景、前置条件、提示与示例）、应用 scopes 和风险。", schemaObject(map[string]any{"operation_id": map[string]any{"type": "string"}, "parameter": map[string]any{"type": "string", "description": "可选：只展开这个顶层参数的 schema，适合复杂操作。"}}, []string{"operation_id"}), true, func(ctx context.Context, p Principal, args map[string]any) Result {
		c, _, err := s.currentConfig().checkPrincipal(p, false)
		if err != nil {
			return failed(err)
		}
		op, ok := s.Catalog.Operation(args["operation_id"].(string))
		if !ok || !discoverable(c, p, op) {
			return failed(fault("operation_denied", "操作未向当前接入开放"))
		}
		if name, _ := args["parameter"].(string); name != "" {
			schema, ok := object(op.InputSchema["properties"])[name]
			if !ok {
				return failed(fault("unknown_parameter", "该操作没有这个顶层参数"))
			}
			return Result{Status: "succeeded", Data: map[string]any{"operation_id": op.ID, "parameter": name, "schema": schema}}
		}
		data := map[string]any{"operation_id": op.ID, "domain": opDomain(op), "kind": op.Kind, "input_schema": op.InputSchema, "output_schema": resultSchema(&op), "app_scopes": op.AppScopes, "conditional_app_scopes": op.ConditionalScopes, "risk": op.Risk, "description": op.Description}
		if g, ok := s.operationGuidance(c, p, op); ok {
			data["guidance"] = g
		}
		return Result{Status: "succeeded", Data: data}
	})
	for _, mode := range []string{"read", "write"} {
		mode := mode
		description := "执行读取操作：传 operation_id 和 arguments。已知 operation_id 与参数时直接调用；参数不确定时先 feishu_tools_describe。"
		if mode == "write" {
			description = "执行写入操作：传 operation_id 和 arguments，arguments 中提供稳定的 idempotency_key，重试时复用同一键和参数。已知 operation_id 与参数时直接调用；参数不确定时先 feishu_tools_describe。"
		}
		s.addUtility(server, "feishu_execute_"+mode, description, schemaObject(map[string]any{"operation_id": map[string]any{"type": "string"}, "arguments": map[string]any{"type": "object"}}, []string{"operation_id", "arguments"}), false, func(ctx context.Context, p Principal, args map[string]any) Result {
			op, ok := s.Catalog.Operation(args["operation_id"].(string))
			if !ok {
				return failed(fault("unknown_operation", "操作未登记"))
			}
			if (mode == "read") != (op.Risk == "read") {
				return failed(fault("wrong_executor", "请使用与操作读写类型对应的执行入口"))
			}
			return s.Invoke(ctx, p, op.ID, object(args["arguments"]))
		})
	}
}

// operationSummary 是搜索结果中的单个操作：只给选择所需的信息，完整参数用 describe 获取。
func (s *Service) operationSummary(op Operation) map[string]any {
	v := map[string]any{"operation_id": op.ID, "domain": opDomain(op), "kind": op.Kind, "risk": op.Risk, "pack": op.Pack, "description": truncateRunes(op.Description, 240)}
	if g, ok := s.Guidance.Operations[op.ID]; ok && len(g.UseWhen) > 0 {
		v["use_when"] = firstSentence(g.UseWhen[0], 200)
	}
	return v
}

// operationGuidance 返回当前请求可用的指南：相关操作和示例只保留可发现的部分。
func (s *Service) operationGuidance(c Client, p Principal, op Operation) (OperationGuidance, bool) {
	g, ok := s.Guidance.Operations[op.ID]
	if !ok {
		return g, false
	}
	related := []string{}
	for _, id := range g.Related {
		if other, ok := s.Catalog.Operation(id); ok && discoverable(c, p, other) {
			related = append(related, id)
		}
	}
	examples := []GuidanceExample{}
	for _, ex := range g.Examples {
		if other, ok := s.Catalog.Operation(ex.OperationID); ok && discoverable(c, p, other) {
			examples = append(examples, ex)
		}
	}
	g.Related, g.Examples = related, examples
	return g, true
}
