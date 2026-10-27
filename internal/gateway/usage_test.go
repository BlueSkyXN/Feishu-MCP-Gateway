// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"encoding/json"
	"io/fs"
	"maps"
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

// 指南正文不保留身份切换参数、CLI 全局 flag 的表格行和按 shell 区分的写法表。
// 无法等价转换的 lark-cli 命令保留原文（管道或 heredoc 输入、stdin、命令替换、无法映射的参数、命令模板与 CLI 输出示例），
// 数量超过上限时检查 scripts/lark_guidance.py 的转换规则。
func TestGuidanceTextHasNoCLIOnlyLeftovers(t *testing.T) {
	const maxCLIResidue = 48
	identity := regexp.MustCompile(`--as\b`)
	cli := regexp.MustCompile(`lark-cli [a-z]`)
	rule := regexp.MustCompile(`^\s*\|(?:\s*:?-{3,}:?\s*\|)+\s*$`)
	shell := regexp.MustCompile(`\b(?:bash|zsh|PowerShell|cmd\.exe)\b`)
	residue := 0
	err := fs.WalkDir(guidanceFS, "guidance", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		b, err := guidanceFS.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(b), "\n")
		fence := false
		for i := 0; i < len(lines); i++ {
			line := lines[i]
			if identity.MatchString(line) {
				t.Errorf("%s:%d: identity flag kept: %s", path, i+1, line)
			}
			if !strings.HasPrefix(line, "> 由 ") && cli.MatchString(line) {
				residue++
			}
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				fence = !fence
				continue
			}
			if fence || !strings.HasPrefix(strings.TrimSpace(line), "|") || i+1 >= len(lines) || !rule.MatchString(lines[i+1]) {
				continue
			}
			if shell.MatchString(line) {
				t.Errorf("%s:%d: shell-specific table kept: %s", path, i+1, line)
			}
			for i += 2; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
				if cells := strings.Split(lines[i], "|"); len(cells) > 2 && strings.TrimSpace(cells[1]) == "" {
					t.Errorf("%s:%d: table row lost its first cell: %s", path, i+1, lines[i])
				}
			}
			i--
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if residue > maxCLIResidue {
		t.Errorf("lark-cli 原文残留 %d 行，超过 %d；检查 scripts/lark_guidance.py 的转换规则", residue, maxCLIResidue)
	}
}

// placeholderValue 匹配指南示例里的占位值，如 <doc_id>、<目标ID>、<sheet|bitable>。
var placeholderValue = regexp.MustCompile(`^<[^<>"'=/\s]+>$`)

// relaxPlaceholders 返回放宽后的 schema：取值为占位值的参数只要求存在，不检查类型、枚举与格式。
func relaxPlaceholders(schema map[string]any, value any) map[string]any {
	out := maps.Clone(schema)
	switch v := value.(type) {
	case map[string]any:
		props, _ := schema["properties"].(map[string]any)
		if props == nil {
			return out
		}
		relaxed := maps.Clone(props)
		for key, child := range v {
			sub, ok := props[key].(map[string]any)
			if !ok {
				continue
			}
			if text, ok := child.(string); ok && placeholderValue.MatchString(text) {
				relaxed[key] = map[string]any{}
			} else {
				relaxed[key] = relaxPlaceholders(sub, child)
			}
		}
		out["properties"] = relaxed
	case []any:
		items, _ := schema["items"].(map[string]any)
		for _, child := range v {
			if items == nil {
				break
			}
			if text, ok := child.(string); ok && placeholderValue.MatchString(text) {
				items = map[string]any{}
			} else {
				items = relaxPlaceholders(items, child)
			}
		}
		if items != nil {
			out["items"] = items
		}
	}
	return out
}

func walkStrings(value any, visit func(string)) {
	switch v := value.(type) {
	case string:
		visit(v)
	case map[string]any:
		for _, child := range v {
			walkStrings(child, visit)
		}
	case []any:
		for _, child := range v {
			walkStrings(child, visit)
		}
	}
}

// 指南代码块里的 `operation_id {…}` 行是 Agent 会照着调用的示例：参数要通过对应操作的 schema，
// 字符串里不能残留 shell 写法（$'...'、$VAR）；`<…>` 占位值只放宽类型、枚举与格式检查。
// 原示例是 dry-run 预览时，转换结果不能是一条真实写入。
func TestGuidanceCodeExamplesMatchSchema(t *testing.T) {
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	shellSyntax := regexp.MustCompile(`^\$['"A-Za-z_{(]`)
	preview := regexp.MustCompile(`(?i)预览|preview|不实际执行|不真正执行|without (?:executing|creating)`)
	confirmed := regexp.MustCompile(`确认后|真实|换成`)
	examples := 0
	err = fs.WalkDir(guidanceFS, "guidance", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		b, err := guidanceFS.ReadFile(path)
		if err != nil {
			return err
		}
		fence, pendingPreview := false, ""
		for i, raw := range strings.Split(string(b), "\n") {
			// 引用块中的代码块带 > 前缀。
			line := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(raw), ">"))
			if strings.HasPrefix(line, "```") {
				fence, pendingPreview = !fence, ""
				continue
			}
			if !fence || line == "" {
				continue
			}
			if strings.HasPrefix(line, "#") {
				pendingPreview = ""
				if preview.MatchString(line) && !confirmed.MatchString(line) && !strings.Contains(line, "网关不提供 dry-run") {
					pendingPreview = line
				}
				continue
			}
			id, rest, _ := strings.Cut(line, " ")
			op, ok := cat.Operation(id)
			previewComment := pendingPreview
			pendingPreview = ""
			if !ok {
				continue
			}
			examples++
			if previewComment != "" && op.Risk != "read" {
				t.Errorf("%s:%d: 预览示例被转换成真实写入（%s）: %s", path, i+1, previewComment, line)
			}
			args := map[string]any{}
			if rest = strings.TrimSpace(rest); strings.HasPrefix(rest, "{") {
				if err := json.NewDecoder(strings.NewReader(rest)).Decode(&args); err != nil {
					t.Errorf("%s:%d: 参数不是 JSON: %v", path, i+1, err)
					continue
				}
			}
			walkStrings(args, func(text string) {
				if shellSyntax.MatchString(text) {
					t.Errorf("%s:%d: 参数残留 shell 写法 %q", path, i+1, text)
				}
			})
			if _, ok := args["idempotency_key"]; !ok && op.Risk != "read" {
				args["idempotency_key"] = "example"
			}
			resolved, err := compileSchema(relaxPlaceholders(op.InputSchema, args))
			if err != nil {
				t.Fatal(err)
			}
			if err := resolved.Validate(args); err != nil {
				t.Errorf("%s:%d: %s 的示例参数不符合 schema: %v", path, i+1, id, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if examples == 0 {
		t.Fatal("没有找到代码块调用示例")
	}
	// 各条转换规则的代表结果。
	for path, want := range map[string]string{
		"guidance/lark-im/lark-im-messages-send.md":       `"text": "Line 1\nLine 2\n  indented line"`, // $'...' 按 bash 规则解码
		"guidance/lark-doc/lark-doc-update.md":            `"content": "<p>新段落</p>"`,                   // 含 / 的文本不是文件
		"guidance/lark-drive/lark-drive-delete-reply.md":  "# 网关不提供 dry-run 预览，此示例省略",
		"guidance/lark-calendar/lark-calendar-create.md":  "`[文本](url)`", // 代码片段里的链接语法保留
		"guidance/lark-doc/lark-doc-md.md":                "## 参数传递",     // shell 传参小节换成网关写法
		"guidance/lark-sheets/lark-sheets-write-cells.md": `> sheets.+csv-put {"url": "...", "sheet_name": "Sheet1", "start_cell": "A1", "csv": "name,score\nalice,=SUM(B2:B10)"}`,
	} {
		b, err := guidanceFS.ReadFile(path)
		if err != nil || !strings.Contains(string(b), want) {
			t.Errorf("%s: 缺少转换结果 %s", path, want)
		}
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
		// 以下说法在 0.3.0-dev 首版中没有找到目标操作。
		{"新建电子表格", 3, []string{"sheets.+workbook-create"}},
		{"create spreadsheet", 3, []string{"sheets.+workbook-create"}},
		{"往表格追加一行", 3, []string{"sheets.+table-put"}},
		{"append rows to sheet", 3, []string{"sheets.+table-put"}},
		{"在知识库里找文档", 3, []string{"drive.+search"}},
		{"查看我明天的日程", 3, []string{"calendar.+agenda"}},
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
