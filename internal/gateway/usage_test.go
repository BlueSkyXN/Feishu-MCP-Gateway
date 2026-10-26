// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// broadConfig 给 wake 接入开放全部普通能力包，用于检查发现与搜索。
func broadConfig(t *testing.T, view string) Config {
	t.Helper()
	c := testConfig(t)
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	packs := map[string]bool{}
	for _, op := range cat.Operations {
		packs[op.Pack] = true
	}
	c.Clients[0].Packs = nil
	for p := range packs {
		c.Clients[0].Packs = append(c.Clients[0].Packs, p)
	}
	slices.Sort(c.Clients[0].Packs)
	c.Clients[0].ResourceMode = "application_scope"
	c.Clients[0].ToolView = view
	c.Clients[0].Actors = map[string]ActorPolicy{"feishu:user_id:alice": {}, "feishu:user_id:bob": {Operations: []string{"wiki.+node-get", "wiki.+node-list"}}}
	return c
}

func TestGuidanceIsConsistentWithCatalog(t *testing.T) {
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	g, err := LoadGuidance(cat)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Operations) < 200 || len(g.Skills) != 15 || g.revision == "" || GuidanceRevision() != g.revision {
		t.Fatalf("guidance coverage: operations=%d skills=%d", len(g.Operations), len(g.Skills))
	}
	// 用户身份、CLI 登录和本机命令不应出现在操作级指南里。
	forbidden := regexp.MustCompile(`--as\b|auth login|\bUAT\b|user_access_token|lark-cli `)
	for id, og := range g.Operations {
		for _, list := range [][]string{og.UseWhen, og.AvoidWhen, og.Prerequisites, og.Tips} {
			for _, item := range list {
				if forbidden.MatchString(item) {
					t.Errorf("%s: CLI-only guidance kept: %s", id, item)
				}
			}
		}
		for _, ex := range og.Examples {
			op, _ := cat.Operation(ex.OperationID)
			args := map[string]any{}
			for k, v := range ex.Arguments {
				args[k] = v
			}
			if op.Risk != "read" {
				args["idempotency_key"] = "example"
			}
			if err := op.Validate(args); err != nil {
				t.Errorf("%s example does not match schema: %v", id, err)
			}
		}
	}
	guide, ok := g.guide("lark-doc")
	if !ok || !strings.Contains(guide, "`docs.+fetch`") || strings.Contains(guide, "--as user") {
		t.Fatal("docs guide not translated")
	}
	if _, _, content, ok := g.topic("lark-sheets-read-data"); !ok || !strings.Contains(content, "`sheets.+csv-get`") {
		t.Fatal("sheets topic not translated")
	}
}

func TestSkillFileMatchesRenderAndCatalog(t *testing.T) {
	b, err := os.ReadFile("../../skills/feishu-gateway/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != RenderSkill() {
		t.Fatal("skills/feishu-gateway/SKILL.md is stale; run feishu-mcp-gateway --render-skill")
	}
	cat, _ := LoadCatalog()
	refs := regexp.MustCompile("`([a-z]+\\.(?:\\+[a-z0-9-]+|[a-z_]+(?:\\.[a-z_]+)+))`").FindAllStringSubmatch(string(b), -1)
	if len(refs) < 40 {
		t.Fatalf("too few operation references: %d", len(refs))
	}
	for _, m := range refs {
		if _, ok := cat.Operation(m[1]); !ok {
			t.Errorf("Skill references unknown operation %s", m[1])
		}
	}
}

func TestSearchFindsTaskPhrasesInChineseAndEnglish(t *testing.T) {
	s := startService(t, broadConfig(t, "compact"), &fakeExecutor{})
	c := s.currentConfig().Clients[0]
	top := func(query string, n int) []string {
		hits, _ := s.search.search(query, func(op Operation) bool { return discoverable(c, Principal{ClientID: c.ID}, op) })
		out := []string{}
		for _, h := range hits[:min(n, len(hits))] {
			out = append(out, h.entry.op.ID)
		}
		return out
	}
	for _, tc := range []struct {
		query string
		n     int
		want  []string
	}{
		{"读取文档", 3, []string{"docs.+fetch"}},
		{"read document", 3, []string{"docs.+fetch"}},
		{"send file", 3, []string{"im.+messages-send"}},
		{"发送文件", 3, []string{"im.+messages-send"}},
		{"上传文件", 3, []string{"drive.+upload"}},
		{"spreadsheet read", 3, []string{"sheets.+csv-get", "sheets.+cells-get"}},
		{"读取表格", 3, []string{"sheets.+csv-get"}},
		{"bitable record", 5, []string{"base.+record-list"}},
		{"多维表格 记录", 5, []string{"base.+record-list"}},
		{"知识库节点", 5, []string{"wiki.+node-get"}},
		{"创建日程", 3, []string{"calendar.+create"}},
	} {
		got := top(tc.query, tc.n)
		for _, want := range tc.want {
			if !slices.Contains(got, want) {
				t.Errorf("%q top %d = %v, want %s", tc.query, tc.n, got, want)
			}
		}
	}
	if got := top("wiki.spaces.get", 10); len(got) != 1 || got[0] != "wiki.spaces.get" {
		t.Fatalf("exact id: %v", got)
	}
}

func TestCompactDiscoveryHonorsActorPolicyAndPaging(t *testing.T) {
	s := startService(t, broadConfig(t, "compact"), &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	session, transport := connect(t, server.URL, "2026-07-28", "fixture-key", "")
	r := call(t, session, "feishu_tools_search", map[string]any{"query": "wiki", "limit": 5})
	data := object(r.Data)
	if r.Error != nil || len(data["operations"].([]any)) != 5 || data["next_after"] != "5" {
		t.Fatalf("discovery without actor: %+v", r)
	}
	r = call(t, session, "feishu_tools_search", map[string]any{"query": "wiki", "limit": 5, "after": "5"})
	if first := object(object(r.Data)["operations"].([]any)[0])["operation_id"]; first == object(data["operations"].([]any)[0])["operation_id"] {
		t.Fatal("second page repeated the first page")
	}
	transport.sender.Store("bob")
	r = call(t, session, "feishu_tools_search", map[string]any{"query": "wiki"})
	ops := object(r.Data)["operations"].([]any)
	if len(ops) != 2 {
		t.Fatalf("actor-restricted search: %+v", r)
	}
	r = call(t, session, "feishu_tools_describe", map[string]any{"operation_id": "wiki.spaces.get"})
	if r.Error == nil || r.Error.Kind != "operation_denied" {
		t.Fatalf("actor-restricted describe: %+v", r)
	}
	r = call(t, session, "feishu_tools_describe", map[string]any{"operation_id": "wiki.+node-get"})
	if r.Error != nil || object(r.Data)["domain"] != "wiki" {
		t.Fatalf("describe: %+v", r)
	}
	r = call(t, session, "feishu_guide", nil)
	domains := object(r.Data)["domains"].([]any)
	if len(domains) != 1 || object(domains[0])["operations"] != 2.0 {
		t.Fatalf("actor guide overview: %+v", r)
	}
	r = call(t, session, "feishu_tools_search", map[string]any{"query": "发送消息"})
	if len(object(r.Data)["operations"].([]any)) != 0 || object(r.Data)["hint"] == nil {
		t.Fatalf("no-result hint: %+v", r)
	}
	transport.sender.Store("alice")
	r = call(t, session, "feishu_tools_describe", map[string]any{"operation_id": "docs.+update"})
	g := object(object(r.Data)["guidance"])
	if r.Error != nil || len(g["tips"].([]any)) == 0 || len(g["topics"].([]any)) == 0 {
		t.Fatalf("describe guidance: %+v", r)
	}
	r = call(t, session, "feishu_tools_search", map[string]any{"query": "", "domain": "nope"})
	if r.Error == nil || r.Error.Kind != "unknown_domain" {
		t.Fatalf("unknown domain: %+v", r)
	}
}

func TestGuideToolScopesDomainsTopicsAndPages(t *testing.T) {
	c := testConfig(t)
	c.Clients[1].Packs = []string{"docs.read"}
	s := startService(t, c, &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	session, _ := connect(t, server.URL, "2025-11-25", "other-key", "")
	if inst := session.InitializeResult().Instructions; !strings.Contains(inst, "云文档 docs") || !strings.Contains(inst, "feishu_guide") || !strings.Contains(inst, "feishu_docs_fetch") {
		t.Fatalf("instructions: %s", inst)
	}
	r := call(t, session, "feishu_guide", nil)
	views := object(r.Data)["domains"].([]any)
	if len(views) != 1 || object(views[0])["domain"] != "docs" || !object(views[0])["guide"].(bool) {
		t.Fatalf("overview: %+v", r)
	}
	r = call(t, session, "feishu_guide", map[string]any{"domain": "docs", "limit": 200})
	page := object(r.Data)
	if r.Error != nil || page["next_offset"] != 200.0 || page["eof"] != false || !strings.Contains(page["content"].(string), "lark-doc") || len(page["topics"].([]any)) == 0 {
		t.Fatalf("domain guide: %+v", r)
	}
	r = call(t, session, "feishu_guide", map[string]any{"domain": "docs", "offset": 1 << 30})
	if object(r.Data)["eof"] != true || object(r.Data)["content"] != "" {
		t.Fatalf("offset beyond end: %+v", r)
	}
	r = call(t, session, "feishu_guide", map[string]any{"topic": "lark-doc-fetch"})
	if r.Error != nil || !strings.Contains(object(r.Data)["content"].(string), "docs.+fetch") {
		t.Fatalf("topic: %+v", r)
	}
	for _, args := range []map[string]any{{"domain": "sheets"}, {"topic": "lark-sheets-read-data"}, {"domain": "docs", "topic": "lark-sheets-read-data"}} {
		r = call(t, session, "feishu_guide", args)
		if r.Error == nil || r.Error.Kind != "domain_unavailable" {
			t.Fatalf("closed domain %v: %+v", args, r)
		}
	}
	r = call(t, session, "feishu_context", nil)
	ctx := object(r.Data)
	if ctx["tool_view"] != "named" || ctx["guidance_revision"] != GuidanceRevision() || ctx["catalog_revision"] != CatalogRevision() || len(ctx["domains"].([]any)) != 1 {
		t.Fatalf("context: %+v", r)
	}
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range list.Tools {
		if tool.Name == "feishu_docs_fetch" && (!strings.Contains(tool.Description, "适用：") || strings.Contains(tool.Description, ".。")) {
			t.Fatalf("named description: %s", tool.Description)
		}
	}
}

func TestDiagnosticsReportGuidanceAndClientViews(t *testing.T) {
	s := startService(t, broadConfig(t, "compact"), &fakeExecutor{})
	d := s.Diagnostics()
	var decoded map[string]any
	if err := json.Unmarshal(encode(d), &decoded); err != nil {
		t.Fatal(err)
	}
	views := decoded["client_views"].([]any)
	if decoded["guidance_revision"] != GuidanceRevision() || decoded["version"] != Version || len(views) != 2 || object(views[0])["tool_view"] != "compact" || object(views[0])["operations"].(float64) < 400 {
		t.Fatalf("diagnostics: %+v", decoded)
	}
}
