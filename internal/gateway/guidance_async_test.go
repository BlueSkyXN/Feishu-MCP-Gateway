// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// initialize 指南、具名工具描述、compact 执行工具描述与 Skill 的异步说明必须一致：
// managed 支持 async；request_bearer 仅同步；不确定时调用 feishu_context（A06）。
func TestAsyncGuidanceConsistentAcrossSurfaces(t *testing.T) {
	for _, tc := range []struct {
		name    string
		bearer  bool
		compact bool
	}{
		{"managed named", false, false},
		{"managed compact", false, true},
		{"request_bearer named", true, false},
		{"request_bearer compact", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testConfig(t)
			if tc.bearer {
				c = requestBearerConfig(t)
			}
			c.Clients[0].ToolView = "named"
			if tc.compact {
				c.Clients[0].ToolView = "compact"
			}
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
			s := startService(t, c, &fakeExecutor{})
			server := httptest.NewServer(s.Handler())
			defer server.Close()
			session, _ := connectRequestHeaders(t, server.URL, requestHeaders(""))
			// initialize.instructions：条件说明，同一客户端路由到不同模式时仍正确。
			text := s.instructions(c.Clients[0])
			if !strings.Contains(text, "managed") || !strings.Contains(text, "request_bearer") {
				t.Fatalf("initialize instructions missing conditional async note: %q", text)
			}
			if !strings.Contains(text, "feishu_context") {
				t.Fatal("initialize instructions do not point to feishu_context")
			}
			listed, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			checked := 0
			for _, tool := range listed.Tools {
				desc := tool.Description
				switch {
				case strings.HasPrefix(tool.Name, "feishu_execute_"):
					if !strings.Contains(desc, "async") || !strings.Contains(desc, "managed") {
						t.Errorf("%s description missing conditional async note: %q", tool.Name, desc)
					}
					checked++
				case !tc.compact && tool.Name != "feishu_context" && tool.Name != "feishu_guide" &&
					!strings.HasPrefix(tool.Name, "feishu_job_") && !strings.HasPrefix(tool.Name, "feishu_file") &&
					!strings.HasPrefix(tool.Name, "feishu_tools_") && tool.Name != "feishu_file_transfer":
					// 具名工具：目录描述 + 执行身份尾注。
					if !strings.Contains(desc, "async") {
						t.Errorf("named tool %s missing async note: %q", tool.Name, desc)
					}
					checked++
				}
			}
			if checked == 0 {
				t.Fatal("no tool descriptions were checked")
			}
		})
	}
}

// 工具注册不随请求变化：同一客户端两次连接看到的工具集合一致（A06）。
func TestToolRegistrationStaticPerClient(t *testing.T) {
	c := requestBearerConfig(t)
	s := startService(t, c, &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	first, _ := connectRequestHeaders(t, server.URL, requestHeaders(""))
	second, _ := connectRequestHeaders(t, server.URL, requestHeaders(""))
	names := func(session interface{ ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error) }) map[string]bool {
		listed, err := session.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, tool := range listed.Tools {
			out[tool.Name] = true
		}
		return out
	}
	a, b := names(first), names(second)
	if len(a) == 0 {
		t.Fatal("no tools listed")
	}
	for name := range a {
		if !b[name] {
			t.Errorf("tool %s disappeared between sessions of the same client", name)
		}
	}
	for name := range b {
		if !a[name] {
			t.Errorf("tool %s appeared between sessions of the same client", name)
		}
	}
}
