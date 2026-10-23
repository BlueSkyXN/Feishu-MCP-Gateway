// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type principalKey struct{}

func principal(ctx context.Context) (Principal, error) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	if !ok {
		return p, fault("unauthenticated", "请求缺少已认证上下文")
	}
	return p, nil
}
func boolean(b bool) *bool { return &b }

func mcpResult(r Result) *mcp.CallToolResult {
	result := &mcp.CallToolResult{StructuredContent: r, Content: []mcp.Content{&mcp.TextContent{Text: string(encode(r))}}, IsError: r.Error != nil || r.Status == "unknown"}
	for _, f := range r.Files {
		size := f.Size
		result.Content = append(result.Content, &mcp.ResourceLink{URI: f.URI, Name: f.Name, MIMEType: f.MIME, Size: &size})
	}
	return result
}

func (s *Service) mcpView(client Client) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "Feishu-MCP-Gateway", Version: "0.2.0-dev"}, &mcp.ServerOptions{
		Instructions:              "飞书操作始终以应用身份执行。写操作重试复用 idempotency_key；unknown 状态先核对回执。文件和 Job 标识只能在当前授权范围内使用。",
		SupportedProtocolVersions: []string{"2026-07-28", "2025-11-25"},
		Capabilities:              &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{ListChanged: false}, Resources: &mcp.ResourceCapabilities{ListChanged: false}},
		SetCacheable:              func(_ context.Context, _ mcp.Request, c *mcp.Cacheable) { c.CacheScope = "private"; c.TTLMs = 0 },
	})
	for _, operation := range s.Catalog.Operations {
		op := operation
		if client.ToolView == "compact" || !visible(client, op) {
			continue
		}
		server.AddTool(&mcp.Tool{Name: op.Tool, Description: op.Description + "。执行身份固定为飞书应用；支持 async 登记后台操作。", InputSchema: op.InputSchema, OutputSchema: resultSchema(&op), Annotations: &mcp.ToolAnnotations{
			// 所有操作都可能登记 Job/结果文件，保守描述本地副作用。
			ReadOnlyHint: false, DestructiveHint: boolean(op.Risk != "read"), OpenWorldHint: boolean(true), IdempotentHint: op.Risk != "read",
		}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			p, err := principal(ctx)
			if err != nil {
				return mcpResult(failed(err)), nil
			}
			var args map[string]any
			if err = decodeArguments(req.Params.Arguments, &args); err != nil {
				return mcpResult(failed(fault("invalid_arguments", "参数必须是 JSON 对象"))), nil
			}
			return mcpResult(s.Invoke(ctx, p, op.ID, args)), nil
		})
	}
	if client.ToolView == "compact" {
		s.addCompact(server)
	}
	if client.AllowTransferTickets {
		s.addTransferTool(server)
	}
	s.addUtility(server, "feishu_context", "查看当前接入的应用身份、模式和能力范围。", schemaObject(nil, nil), true, func(ctx context.Context, p Principal, args map[string]any) Result {
		c, a, err := s.currentConfig().checkPrincipal(p, false)
		if err != nil {
			return failed(err)
		}
		return Result{Status: "succeeded", Data: map[string]any{"app_connection_id": a.ID, "execution_identity": "bot", "identity_mode": c.IdentityMode, "actor": p.Actor, "packs": c.Packs, "catalog_source": s.Catalog.SourceCommit}}
	})
	for _, action := range []string{"get", "cancel"} {
		action := action
		s.addUtility(server, "feishu_job_"+action, "查询执行回执或请求取消剩余工作；取消不撤销已完成的飞书操作。", schemaObject(map[string]any{"job_id": map[string]any{"type": "string", "minLength": 1}}, []string{"job_id"}), action == "get", func(ctx context.Context, p Principal, args map[string]any) Result {
			return s.Job(p, args["job_id"].(string), action == "cancel")
		})
	}
	s.addUtility(server, "feishu_file_create", "保存小文本为当前主体的受控文件；大文件使用已认证的文件传输接口。", schemaObject(map[string]any{"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 255}, "text": map[string]any{"type": "string", "maxLength": 65536}}, []string{"name", "text"}), false, func(ctx context.Context, p Principal, args map[string]any) Result {
		if _, _, err := s.currentConfig().checkPrincipal(p, true); err != nil {
			return failed(err)
		}
		f, err := s.Store.PutFile(p, args["name"].(string), "text/plain; charset=utf-8", []byte(args["text"].(string)))
		if err != nil {
			return failed(err)
		}
		return Result{Status: "succeeded", Files: []File{f}}
	})
	s.addUtility(server, "feishu_file_list", "列出当前主体可访问的文件；after 使用上一页的 next_after。", schemaObject(map[string]any{"after": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}}, nil), true, func(ctx context.Context, p Principal, args map[string]any) Result {
		if _, _, err := s.currentConfig().checkPrincipal(p, true); err != nil {
			return failed(err)
		}
		after, _ := args["after"].(string)
		limit := 50
		if n, ok := args["limit"].(float64); ok {
			limit = int(n)
		}
		files, err := s.Store.ListFiles(p, after, limit+1)
		if err != nil {
			return failed(err)
		}
		more := len(files) > limit
		if more {
			files = files[:limit]
		}
		data := map[string]any{"has_more": more}
		if more {
			data["next_after"] = files[len(files)-1].ID
		}
		visible := []File{}
		for _, f := range files {
			if _, err := s.AuthorizeFile(p, f.ID); err == nil {
				visible = append(visible, f)
			}
		}
		return Result{Status: "succeeded", Files: visible, Data: data}
	})
	for _, action := range []string{"stat", "read", "delete"} {
		action := action
		props := map[string]any{"file_id": map[string]any{"type": "string", "minLength": 1}}
		if action == "read" {
			props["offset"] = map[string]any{"type": "integer", "minimum": 0}
			props["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 65536}
		}
		s.addUtility(server, "feishu_file_"+action, "访问当前主体的受控文件。read 最多读取 64 KiB，二进制或非完整 UTF-8 返回 base64。", schemaObject(props, []string{"file_id"}), action != "delete", func(ctx context.Context, p Principal, args map[string]any) Result {
			if _, _, err := s.currentConfig().checkPrincipal(p, true); err != nil {
				return failed(err)
			}
			id := args["file_id"].(string)
			if _, err := s.AuthorizeFile(p, id); err != nil {
				return failed(err)
			}
			if action == "delete" {
				if err := s.Store.DeleteFile(p, id); err != nil {
					return failed(err)
				}
				return Result{Status: "succeeded"}
			}
			f, err := s.Store.File(p, id)
			if err != nil {
				return failed(err)
			}
			if action == "stat" {
				return Result{Status: "succeeded", Files: []File{f}}
			}
			offset, limit := int64(0), 65536
			if n, ok := args["offset"].(float64); ok {
				offset = int64(n)
			}
			if n, ok := args["limit"].(float64); ok {
				limit = int(n)
			}
			_, b, err := s.Store.ReadFile(p, id, offset, limit)
			if err != nil {
				return failed(err)
			}
			data := map[string]any{"offset": offset, "next_offset": offset + int64(len(b)), "eof": offset+int64(len(b)) >= f.Size}
			if isText(f.MIME) && utf8.Valid(b) {
				data["text"] = string(b)
			} else {
				data["base64"] = base64.StdEncoding.EncodeToString(b)
			}
			return Result{Status: "succeeded", Data: data, Files: []File{f}}
		})
	}
	server.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "feishu-file://files/{file_id}", Name: "受控文件", Description: "当前主体可读取的操作产物和输入文件。"}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		p, err := principal(ctx)
		if err != nil {
			return nil, err
		}
		if _, _, err = s.currentConfig().checkPrincipal(p, true); err != nil {
			return nil, err
		}
		const prefix = "feishu-file://files/"
		uri := req.Params.URI
		if !strings.HasPrefix(uri, prefix) {
			return nil, fault("not_found", "未知文件资源")
		}
		id := strings.TrimPrefix(uri, prefix)
		f, err := s.AuthorizeFile(p, id)
		if err != nil {
			return nil, err
		}
		if f.Size > int64(min(s.config.MaxInlineBytes, 256<<10)) {
			return nil, fault("resource_too_large", "使用 feishu_file_read 分段读取或通过文件传输接口下载")
		}
		_, b, err := s.Store.ReadFile(p, id, 0, min(s.config.MaxInlineBytes, 256<<10))
		if err != nil {
			return nil, err
		}
		content := &mcp.ResourceContents{URI: uri, MIMEType: f.MIME}
		if isText(f.MIME) && utf8.Valid(b) {
			content.Text = string(b)
		} else {
			content.Blob = b
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{content}}, nil
	})
	return server
}

func schemaObject(props map[string]any, required []string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	out := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}
func (s *Service) addUtility(server *mcp.Server, name, description string, input map[string]any, readOnly bool, fn func(context.Context, Principal, map[string]any) Result) {
	schema, err := compileSchema(input)
	if err != nil {
		panic(fmt.Sprintf("internal tool schema: %s: %v", name, err))
	}
	server.AddTool(&mcp.Tool{Name: name, Description: description, InputSchema: input, OutputSchema: resultSchema(nil), Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, DestructiveHint: boolean(strings.HasSuffix(name, "delete") || strings.HasSuffix(name, "cancel") || name == "feishu_execute_write"), OpenWorldHint: boolean(strings.HasPrefix(name, "feishu_execute_")), IdempotentHint: name == "feishu_execute_write"}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p, err := principal(ctx)
		if err != nil {
			return mcpResult(failed(err)), nil
		}
		var args map[string]any
		if err = decodeArguments(req.Params.Arguments, &args); err != nil {
			return mcpResult(failed(fault("invalid_arguments", "参数必须是对象"))), nil
		}
		if err = schema.Validate(args); err != nil {
			return mcpResult(failed(fault("invalid_arguments", err.Error()))), nil
		}
		if name != "feishu_context" && !strings.HasPrefix(name, "feishu_tools_") && !strings.HasPrefix(name, "feishu_execute_") {
			if err := s.rate(p); err != nil {
				return mcpResult(failed(err)), nil
			}
		}
		return mcpResult(fn(ctx, p, args)), nil
	})
}
func isText(mime string) bool {
	return strings.HasPrefix(mime, "text/") || strings.HasPrefix(mime, "application/json")
}

func decodeArguments(raw json.RawMessage, out *map[string]any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	return json.Unmarshal(raw, out)
}
