// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
)

func (s *Service) addCompact(server *mcp.Server) {
	s.addUtility(server, "feishu_tools_search", "按关键词查找当前接入可用的操作；返回 operation_id 和简短描述。", schemaObject(map[string]any{"query": map[string]any{"type": "string", "maxLength": 256}, "after": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 50}}, nil), true, func(ctx context.Context, p Principal, args map[string]any) Result {
		c, _, err := s.currentConfig().checkPrincipal(p, false)
		if err != nil {
			return failed(err)
		}
		query, _ := args["query"].(string)
		after, _ := args["after"].(string)
		limit := 20
		if n, ok := args["limit"].(float64); ok {
			limit = int(n)
		}
		entries := []any{}
		more := false
		last := ""
		for _, op := range s.Catalog.Operations {
			if !visible(c, op) || op.Tool <= after {
				continue
			}
			haystack := strings.ToLower(op.ID + " " + op.Description + " " + op.Pack)
			match := true
			for _, word := range strings.Fields(strings.ToLower(query)) {
				if !strings.Contains(haystack, word) {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			if len(entries) == limit {
				more = true
				break
			}
			entries = append(entries, map[string]any{"operation_id": op.ID, "description": op.Description, "risk": op.Risk, "pack": op.Pack})
			last = op.Tool
		}
		data := map[string]any{"operations": entries, "has_more": more}
		if more {
			data["next_after"] = last
		}
		return Result{Status: "succeeded", Data: data}
	})
	s.addUtility(server, "feishu_tools_describe", "读取操作的完整参数、返回 schema、应用 scopes 和风险。", schemaObject(map[string]any{"operation_id": map[string]any{"type": "string"}, "parameter": map[string]any{"type": "string", "description": "可选：只展开这个顶层参数的 schema，适合复杂操作。"}}, []string{"operation_id"}), true, func(ctx context.Context, p Principal, args map[string]any) Result {
		c, _, err := s.currentConfig().checkPrincipal(p, false)
		if err != nil {
			return failed(err)
		}
		op, ok := s.Catalog.Operation(args["operation_id"].(string))
		if !ok || !visible(c, op) {
			return failed(fault("operation_denied", "操作未向当前接入开放"))
		}
		if name, _ := args["parameter"].(string); name != "" {
			schema, ok := object(op.InputSchema["properties"])[name]
			if !ok {
				return failed(fault("unknown_parameter", "该操作没有这个顶层参数"))
			}
			return Result{Status: "succeeded", Data: map[string]any{"operation_id": op.ID, "parameter": name, "schema": schema}}
		}
		return Result{Status: "succeeded", Data: map[string]any{"operation_id": op.ID, "input_schema": op.InputSchema, "output_schema": resultSchema(&op), "app_scopes": op.AppScopes, "conditional_app_scopes": op.ConditionalScopes, "risk": op.Risk, "description": op.Description}}
	})
	for _, mode := range []string{"read", "write"} {
		mode := mode
		s.addUtility(server, "feishu_execute_"+mode, "按 operation_id 和其 schema 执行操作。先 describe 获取参数；写入必须提供稳定的 idempotency_key。", schemaObject(map[string]any{"operation_id": map[string]any{"type": "string"}, "arguments": map[string]any{"type": "object"}}, []string{"operation_id", "arguments"}), false, func(ctx context.Context, p Principal, args map[string]any) Result {
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
