// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 超大分页参数曾让 offset+limit 溢出，或在 amd64 上转换成负数，切片越界后 panic 结束整个进程。
func TestHugePagingArgumentsDoNotCrash(t *testing.T) {
	s := startService(t, broadConfig(t, "compact"), &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	session, _ := connect(t, server.URL, "2026-07-28", "fixture-key", "")
	r := call(t, session, "feishu_tools_search", map[string]any{"query": "wiki", "after": "9223372036854775800"})
	data := object(r.Data)
	if r.Error != nil || len(data["operations"].([]any)) != 0 || data["has_more"] != false {
		t.Fatalf("huge cursor: %+v", r)
	}
	for _, offset := range []float64{1e19, 9223372036854775807, 1 << 62} {
		r = call(t, session, "feishu_guide", map[string]any{"domain": "docs", "offset": offset})
		if r.Error != nil || object(r.Data)["eof"] != true || object(r.Data)["content"] != "" {
			t.Fatalf("guide offset %v: %+v", offset, r)
		}
	}
	if r = call(t, session, "feishu_context", nil); r.Error != nil {
		t.Fatalf("context after huge paging: %+v", r)
	}
}

func TestToolPanicBecomesInternalError(t *testing.T) {
	h := guardTool("feishu_fixture", func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		panic("fixture panic")
	})
	res, err := h(context.Background(), &mcp.CallToolRequest{})
	if err != nil || res == nil || !res.IsError {
		t.Fatalf("guarded panic: %+v %v", res, err)
	}
	if r, ok := res.StructuredContent.(Result); !ok || r.Error == nil || r.Error.Kind != "internal" {
		t.Fatalf("guarded panic result: %+v", res.StructuredContent)
	}
	resource := guardResource(func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		panic("fixture panic")
	})
	if out, err := resource(context.Background(), &mcp.ReadResourceRequest{}); out != nil || asFault(err).Kind != "internal" {
		t.Fatalf("guarded resource panic: %+v %v", out, err)
	}
}

// 执行中的 panic 写入回执：写入为 unknown，读取为 failed；同键重试返回原回执，服务继续处理。
func TestExecutionPanicIsRecordedAndServiceContinues(t *testing.T) {
	f := &fakeExecutor{fn: func(context.Context, Invocation, App) ExecutionResult { panic("fixture panic") }}
	s := startService(t, testConfig(t), f)
	args := func() map[string]any {
		return map[string]any{"data": map[string]any{"folder_token": "folder-a", "name": "fixture"}, "idempotency_key": "panic-key"}
	}
	write := s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", args())
	if write.Status != "unknown" || write.Error == nil || write.Error.Kind != "internal" {
		t.Fatalf("write panic: %+v", write)
	}
	again := s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", args())
	if again.Status != "unknown" || again.InvocationID != write.InvocationID || f.calls.Load() != 1 {
		t.Fatalf("retry after panic: %+v calls=%d", again, f.calls.Load())
	}
	read := s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", argsSpace("space-a"))
	if read.Status != "failed" || read.Error == nil || read.Error.Kind != "internal" {
		t.Fatalf("read panic: %+v", read)
	}
}
