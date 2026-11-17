// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/BlueSkyXN/Feishu-MCP-Gateway/internal/ingress"
)

// feishu_context 的 app_id/brand/supports_async 必须来自本次请求经授权检查选中的应用（A05）。
// 无上游凭据也能读取；request_bearer 恒为同步；managed 具备异步能力。
func TestContextMetadataReflectsSelectedApp(t *testing.T) {
	for _, tc := range []struct {
		name           string
		requestBearer  bool
		wantAppID      string
		wantBrand      string
		wantCredential string
		wantAsync      bool
	}{
		{"managed", false, "cli_fixture_a", "feishu", "managed", true},
		{"request_bearer", true, "cli_fixture_a", "feishu", "request_bearer", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testConfig(t)
			if tc.requestBearer {
				c = requestBearerConfig(t)
			}
			s := startService(t, c, &fakeExecutor{})
			server := httptest.NewServer(s.Handler())
			defer server.Close()
			session, _ := connectRequestHeaders(t, server.URL, requestHeaders(""))
			r := call(t, session, "feishu_context", map[string]any{})
			if r.Status != "succeeded" {
				t.Fatalf("context failed: %s", encode(r))
			}
			data := object(r.Data)
			if data["app_id"] != tc.wantAppID || data["brand"] != tc.wantBrand {
				t.Fatalf("app metadata = %v/%v; want %s/%s", data["app_id"], data["brand"], tc.wantAppID, tc.wantBrand)
			}
			if data["credential_mode"] != tc.wantCredential {
				t.Fatalf("credential_mode = %v", data["credential_mode"])
			}
			if data["supports_async"] != tc.wantAsync {
				t.Fatalf("supports_async = %v; want %v", data["supports_async"], tc.wantAsync)
			}
			// 无上游凭据时 upstream_credential_present 必须是 false，context 仍可用。
			if data["upstream_credential_present"] != false {
				t.Fatalf("upstream_credential_present = %v; want false", data["upstream_credential_present"])
			}
		})
	}
}

// 同一客户端配置了路由到 managed 与 request_bearer 两个应用时，
// 带不同 route 的并发 context 调用不得串应用（A05）。
func TestConcurrentRoutedContextDoesNotCrossApps(t *testing.T) {
	c := testConfig(t)
	// app-a 保持 managed；app-b 改为 request_bearer，wake 通过路由可选两者。
	c.Apps[1].CredentialMode = "request_bearer"
	c.Apps[1].SecretEnv = ""
	c.Apps[1].DPoPMode = ""
	c.Clients[0].Routes = []ingress.Route{{ID: "to-managed", AppConnectionID: "app-a"}, {ID: "to-request", AppConnectionID: "app-b"}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	s := startService(t, c, &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	expect := map[string]struct {
		appID, brand, mode, token, headerApp string
	}{
		"to-managed": {"cli_fixture_a", "feishu", "managed", "", ""},
		"to-request": {"cli_fixture_b", "lark", "request_bearer", "route-token-for-app-b", "cli_fixture_b"},
	}
	var wg sync.WaitGroup
	type reading struct {
		route string
		app   string
		brand string
		mode  string
		async bool
	}
	results := make(chan reading, len(expect)*8)
	for route, want := range expect {
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(route string, want struct{ appID, brand, mode, token, headerApp string }) {
				defer wg.Done()
				session, _ := connectRequestHeaders(t, server.URL, routeHeaders(route, want.token, want.headerApp))
				r := call(t, session, "feishu_context", map[string]any{})
				if r.Status != "succeeded" {
					t.Errorf("route %s context failed: %s", route, encode(r))
					return
				}
				data := object(r.Data)
				results <- reading{route, data["app_id"].(string), data["brand"].(string), data["credential_mode"].(string), data["supports_async"].(bool)}
			}(route, want)
		}
	}
	wg.Wait()
	close(results)
	for got := range results {
		want := expect[got.route]
		wantAsync := want.mode != "request_bearer"
		if got.app != want.appID || got.brand != want.brand || got.mode != want.mode || got.async != wantAsync {
			t.Errorf("route %s context crossed apps: app=%s brand=%s mode=%s async=%v", got.route, got.app, got.brand, got.mode, got.async)
		}
	}
}

func routeHeaders(route, token, appID string) http.Header {
	h := requestHeaders(token)
	if appID != "" {
		h.Set(upstreamAppHeader, appID)
	}
	h.Set("X-Feishu-Gateway-Route", route)
	return h
}
