// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadOnlyGuidanceUsesOperationPolicy(t *testing.T) {
	c := testConfig(t)
	c.Clients[1].ToolView = "compact"
	c.Clients[1].Packs = []string{"docs.read"}
	s := startService(t, c, &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	session, _ := connect(t, server.URL, "2025-11-25", "other-key", "")
	for _, topic := range []string{"lark-doc-create", "lark-doc-create-workflow", "lark-doc-update"} {
		r := call(t, session, "feishu_guide", map[string]any{"topic": topic})
		if r.Error == nil || r.Error.Kind != "operation_denied" {
			t.Fatalf("write topic %s visible: %+v", topic, r)
		}
	}
	r := call(t, session, "feishu_guide", map[string]any{"domain": "docs", "limit": 65536})
	if r.Error != nil {
		t.Fatal(r)
	}
	content := object(r.Data)["content"].(string)
	if !strings.Contains(content, "docs.+fetch") || strings.Contains(content, "docs.+create") || strings.Contains(content, "docs.+update") || strings.Contains(content, "lark-doc-create-workflow") {
		t.Fatal("domain guide does not match read policy")
	}
	for _, item := range object(r.Data)["topics"].([]any) {
		id := object(item)["id"]
		if id == "lark-doc-create" || id == "lark-doc-update" || id == "lark-doc-create-workflow" {
			t.Fatal("write topic listed", id)
		}
	}
	topicCount := len(object(r.Data)["topics"].([]any))
	if object(object(r.Data)["domain"])["topics"] != float64(topicCount) {
		t.Fatal("domain topic count differs from filtered list")
	}
	overview := call(t, session, "feishu_guide", nil)
	for _, value := range object(overview.Data)["domains"].([]any) {
		if domain := object(value); domain["domain"] == "docs" && domain["topics"] != float64(topicCount) {
			t.Fatal("overview retained the unrestricted topic count")
		}
	}
	for _, topic := range []string{"lark-doc-fetch", "lark-doc-history"} {
		r := call(t, session, "feishu_guide", map[string]any{"topic": topic, "limit": 65536})
		if r.Error != nil {
			t.Fatal("read topic lost", topic, r)
		}
		text := object(r.Data)["content"].(string)
		if topic == "lark-doc-fetch" && !strings.Contains(text, `docs.+fetch {`) {
			t.Fatal("read examples lost")
		}
		if topic == "lark-doc-fetch" && !strings.Contains(text, "|浏览、总结|`simple`（默认）|") {
			t.Fatal("read detail table lost with denied update row")
		}
		for _, id := range s.Guidance.operationRefs(text, s.Catalog) {
			op, ok := s.Catalog.Operation(id)
			if !ok || !discoverable(c.Clients[1], Principal{}, op) {
				t.Fatalf("denied operation %s remains in %s", id, topic)
			}
		}
	}
}

func TestGuideFilteringKeepsIndependentTableRows(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	view := guidanceView{guidance: &Guidance{}, catalog: catalog, operations: map[string]bool{"docs.+fetch": true}}
	header := "|场景|参数|\n|:---|---:|"
	readRow := "|浏览、总结|`simple`，docs.+fetch|"
	writeRow := "|编辑|docs.+update|"
	for _, tc := range []struct{ name, input, want string }{
		{"mixed table", header + "\n" + readRow + "\n" + writeRow, header + "\n" + readRow},
		{"escaped pipe", header + "\n|读取 A\\|B|docs.+fetch|\n" + writeRow, header + "\n|读取 A\\|B|docs.+fetch|"},
		{"all rows allowed", header + "\n" + readRow, header + "\n" + readRow},
		{"no surviving row", header + "\n" + writeRow, ""},
		{"forbidden header", "|docs.+update|参数|\n|---|---|\n" + readRow, ""},
		{"invalid divider", "|场景|参数|\n|--bad--|---|\n" + readRow + "\n" + writeRow, ""},
		{"invalid header", "普通段落\n|---|---|\n" + readRow + "\n" + writeRow, ""},
		{"trailing paragraph", header + "\n" + readRow + "\n" + writeRow + "\n后续说明", ""},
		{"code fence", "```markdown\n" + header + "\n" + readRow + "\n" + writeRow + "\n```", ""},
		{"ordered workflow", "1. docs.+fetch\n2. docs.+update", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := view.filterBlock(tc.input); got != tc.want {
				t.Fatalf("filtered block:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestGuidanceViewTracksActorAndReload(t *testing.T) {
	c := broadConfig(t, "compact")
	c.Clients[0].Actors["feishu:user_id:bob"] = ActorPolicy{Operations: []string{"docs.+fetch"}}
	s := startService(t, c, &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	session, transport := connect(t, server.URL, "2026-07-28", "fixture-key", "alice")
	args := map[string]any{"topic": "lark-doc-create"}
	if got := call(t, session, "feishu_guide", args); got.Error != nil || !strings.Contains(object(got.Data)["content"].(string), `docs.+create {`) {
		t.Fatal("allowed guide changed", got)
	}
	transport.sender.Store("bob")
	if got := call(t, session, "feishu_guide", args); got.Error == nil || got.Error.Kind != "operation_denied" {
		t.Fatal("actor policy not applied", got)
	}
	if got := call(t, session, "feishu_guide", map[string]any{"topic": "lark-doc-fetch"}); got.Error != nil || !strings.Contains(object(got.Data)["content"].(string), `docs.+fetch {`) {
		t.Fatal("allowed actor read topic unavailable", got)
	}
	transport.sender.Store("alice")
	if got := call(t, session, "feishu_guide", args); got.Error != nil {
		t.Fatal("shared guide mutated", got)
	}
	c.Clients[0].Actors["feishu:user_id:alice"] = ActorPolicy{Operations: []string{"docs.+fetch"}}
	if err := s.Reload(c); err != nil {
		t.Fatal(err)
	}
	if got := call(t, session, "feishu_guide", args); got.Error == nil || got.Error.Kind != "operation_denied" {
		t.Fatal("reload did not revoke guide", got)
	}
	view := s.guidanceView(s.currentConfig().Clients[0], actor("alice"))
	for id, visible := range view.topics {
		if visible && !view.allowsText(view.content(id)) {
			t.Fatal("filtered topic still contains an unavailable reference", id)
		}
	}
}

func TestGuideFilteringKeepsFencesAndWholeSteps(t *testing.T) {
	s := startService(t, broadConfig(t, "compact"), &fakeExecutor{})
	c := cloneConfigValue(s.currentConfig()).Clients[0]
	c.Actors["feishu:user_id:alice"] = ActorPolicy{Operations: []string{"docs.+fetch"}}
	view := s.guidanceView(c, actor("alice"))
	const content = "# Guide\n\n## Read\n\n````markdown\n```text\n# inside code\ndocs.+fetch {}\n```\n````\n\n## Write docs.+create\n\nremove child text\n\n### Nested\n\nremove nested text\n\n## Mixed steps\n\n1. docs.+fetch {}\n2. docs.+update {}\n\n## Another read\n\n~~~text\ndocs.+fetch {}\n~~~\n"
	s.Guidance.documents["fixture"] = parseGuideDocument(content)
	filtered := view.content("fixture")
	if strings.Contains(filtered, "remove") || strings.Contains(filtered, "docs.+update") || strings.Contains(filtered, "1. docs.+fetch") {
		t.Fatal("partial write workflow retained", filtered)
	}
	if !strings.Contains(filtered, "````markdown\n```text\n# inside code\ndocs.+fetch {}\n```\n````") || !strings.Contains(filtered, "~~~text\ndocs.+fetch {}\n~~~") {
		t.Fatal("fenced read examples damaged", filtered)
	}
}
