// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeExecutor struct {
	calls atomic.Int64
	fn    func(context.Context, Invocation, App) ExecutionResult
}

func (f *fakeExecutor) Execute(ctx context.Context, in Invocation, a App) ExecutionResult {
	f.calls.Add(1)
	if f.fn != nil {
		return f.fn(ctx, in, a)
	}
	if in.Operation.ID == "drive.files.create_folder" {
		return ExecutionResult{Data: map[string]any{"token": "created-folder"}}
	}
	return ExecutionResult{Data: map[string]any{"space": map[string]any{"space_id": object(in.Arguments["params"])["space_id"], "name": "fixture"}}}
}

func testConfig(t *testing.T) Config {
	t.Helper()
	t.Setenv("TEST_FEISHU_SECRET", "fixture-app-secret")
	c := Config{CatalogRevision: CatalogRevision(), DataDir: t.TempDir(), Listen: "127.0.0.1:0", Workers: 4, Apps: []App{{ID: "app-a", OrgID: "org-a", AppID: "cli_fixture_a", Brand: "feishu", SecretEnv: "TEST_FEISHU_SECRET"}, {ID: "app-b", OrgID: "org-b", AppID: "cli_fixture_b", Brand: "lark", SecretEnv: "TEST_FEISHU_SECRET"}}, Clients: []Client{
		{ID: "wake", AppID: "app-a", KeySHA256: digest([]byte("fixture-key")), IdentityMode: "wake_actor", Packs: []string{"wiki.read", "drive.write"}, ResourceMode: "resource_scoped", MaxConcurrent: 4, Resources: map[string][]string{"wiki_space": {"space-a", "space-b"}, "folder": {"folder-a", "folder-b"}}, Actors: map[string]ActorPolicy{"feishu:user_id:alice": {Resources: map[string][]string{"wiki_space": {"space-a"}, "folder": {"folder-a"}}}, "feishu:user_id:bob": {Resources: map[string][]string{"wiki_space": {"space-b"}, "folder": {"folder-b"}}}}},
		{ID: "desktop", AppID: "app-b", KeySHA256: digest([]byte("other-key")), IdentityMode: "client", Packs: []string{"wiki.read"}, ResourceMode: "application_scope", MaxConcurrent: 2},
	}}
	for i := range c.Apps {
		c.Apps[i].OutboundRequestsPerSecond = 1000
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	return c
}
func actor(name string) Principal {
	return Principal{ClientID: "wake", AppID: "app-a", OrgID: "org-a", Actor: "feishu:user_id:" + name}
}
func startService(t *testing.T, c Config, f *fakeExecutor) *Service {
	t.Helper()
	s, err := NewService(c, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}
func argsSpace(space string) map[string]any {
	return map[string]any{"params": map[string]any{"space_id": space}}
}

type authTransport struct {
	key    string
	sender atomic.Value
}

func (a *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header = req.Header.Clone()
	r.Header.Set("Authorization", "Bearer "+a.key)
	if name, _ := a.sender.Load().(string); name != "" {
		r.Header.Set("X-QoderWake-AtWaker-Sender-Id", name)
		r.Header.Set("X-QoderWake-AtWaker-Sender-Channel", "feishu")
	}
	return http.DefaultTransport.RoundTrip(r)
}
func connect(t *testing.T, url, version, key, name string) (*mcp.ClientSession, *authTransport) {
	t.Helper()
	transport := &authTransport{key: key}
	transport.sender.Store(name)
	client := mcp.NewClient(&mcp.Implementation{Name: "gateway-test", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: url + "/mcp", HTTPClient: &http.Client{Transport: transport}}, &mcp.ClientSessionOptions{ProtocolVersion: version})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session, transport
}
func call(t *testing.T, session *mcp.ClientSession, name string, args any) Result {
	t.Helper()
	r, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	var out Result
	if err = json.Unmarshal(encode(r.StructuredContent), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMCPVersionsAndPerRequestActor(t *testing.T) {
	for _, version := range []string{"2026-07-28", "2025-11-25"} {
		t.Run(version, func(t *testing.T) {
			f := &fakeExecutor{}
			s := startService(t, testConfig(t), f)
			httpServer := httptest.NewServer(s.Handler())
			defer httpServer.Close()
			session, transport := connect(t, httpServer.URL, version, "fixture-key", "")
			list, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(list.Tools) < 4 {
				t.Fatal("tools missing")
			}
			if version == "2026-07-28" && list.CacheScope != "private" {
				t.Fatalf("cache scope = %q", list.CacheScope)
			}
			for _, tool := range list.Tools {
				if tool.Name == "feishu_wiki_spaces_list" {
					t.Fatal("unscopable tool exposed")
				}
			}
			r := call(t, session, "feishu_wiki_spaces_get", argsSpace("space-a"))
			if r.Error == nil || r.Error.Kind != "actor_required" {
				t.Fatalf("missing actor: %+v", r)
			}
			for _, name := range []string{"alice", "bob", "alice"} {
				transport.sender.Store(name)
				space := "space-a"
				if name == "bob" {
					space = "space-b"
				}
				r = call(t, session, "feishu_wiki_spaces_get", argsSpace(space))
				if r.Status != "succeeded" {
					t.Fatalf("%s: %+v", name, r)
				}
			}
			transport.sender.Store("bob")
			r = call(t, session, "feishu_wiki_spaces_get", argsSpace("space-a"))
			if r.Error == nil || r.Error.Kind != "resource_denied" {
				t.Fatalf("cross actor resource: %+v", r)
			}
			if f.calls.Load() != 3 {
				t.Fatalf("unauthorized calls reached backend: %d", f.calls.Load())
			}
		})
	}
}

func TestSchemaAndIdentityCannotBeOverridden(t *testing.T) {
	f := &fakeExecutor{}
	s := startService(t, testConfig(t), f)
	for _, args := range []map[string]any{{"params": map[string]any{"space_id": "space-a", "as": "user"}}, {"params": map[string]any{"space_id": "space-a"}, "app_id": "other"}, {"params": map[string]any{"space_id": 9.0}}} {
		r := s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", args)
		if r.Error == nil || r.Error.Kind != "invalid_arguments" {
			t.Fatalf("accepted invalid input: %+v", r)
		}
	}
	if f.calls.Load() != 0 {
		t.Fatal("backend reached")
	}
}

func TestWriteIdempotencyConcurrentAndRestart(t *testing.T) {
	c := testConfig(t)
	f := &fakeExecutor{}
	s, err := NewService(c, f)
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"data": map[string]any{"folder_token": "folder-a", "name": "fixture"}, "idempotency_key": "same-key"}
	results := make(chan Result, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", args)
		}()
	}
	wg.Wait()
	close(results)
	id := ""
	for r := range results {
		if r.Error != nil {
			t.Fatalf("invoke: %+v", r)
		}
		if id != "" && id != r.InvocationID {
			t.Fatal("duplicate invocation")
		}
		id = r.InvocationID
	}
	if f.calls.Load() != 1 {
		t.Fatalf("write count=%d", f.calls.Load())
	}
	s.Close()
	f2 := &fakeExecutor{}
	s2 := startService(t, c, f2)
	r := s2.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", args)
	if r.InvocationID != id || r.Status != "succeeded" || f2.calls.Load() != 0 {
		t.Fatalf("restart replay: %+v", r)
	}
	args["data"].(map[string]any)["name"] = "different"
	r = s2.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", args)
	if r.Error == nil || r.Error.Kind != "idempotency_conflict" {
		t.Fatalf("key reuse: %+v", r)
	}
}

func TestUnknownWriteIsNeverAutomaticallyRetried(t *testing.T) {
	f := &fakeExecutor{fn: func(context.Context, Invocation, App) ExecutionResult {
		return ExecutionResult{Uncertain: true, Error: fault("network", "response lost")}
	}}
	s := startService(t, testConfig(t), f)
	args := map[string]any{"data": map[string]any{"folder_token": "folder-a", "name": "fixture"}, "idempotency_key": "unknown"}
	for i := 0; i < 2; i++ {
		r := s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", args)
		if r.Status != "unknown" {
			t.Fatalf("%+v", r)
		}
	}
	if f.calls.Load() != 1 {
		t.Fatal("ambiguous write replayed")
	}
}

func TestPaginationBoundToActorAndQuery(t *testing.T) {
	f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, a App) ExecutionResult {
		params := object(in.Arguments["params"])
		if token, ok := params["page_token"]; ok && token != "upstream-token" {
			t.Errorf("not unwrapped: %v", token)
		}
		return ExecutionResult{Data: map[string]any{"items": []any{}, "page_token": "upstream-token", "has_more": true}}
	}}
	s := startService(t, testConfig(t), f)
	args := argsSpace("space-a")
	r := s.Invoke(context.Background(), actor("alice"), "wiki.nodes.list", args)
	if r.Error != nil {
		t.Fatalf("%+v", r)
	}
	cursor := r.Data.(map[string]any)["page_token"].(string)
	if !strings.HasPrefix(cursor, "cur_") {
		t.Fatal(cursor)
	}
	args["params"].(map[string]any)["page_token"] = cursor
	r = s.Invoke(context.Background(), actor("alice"), "wiki.nodes.list", args)
	if r.Error != nil {
		t.Fatalf("own cursor: %+v", r)
	}
	other := argsSpace("space-b")
	other["params"].(map[string]any)["page_token"] = cursor
	r = s.Invoke(context.Background(), actor("bob"), "wiki.nodes.list", other)
	if r.Error == nil || r.Error.Kind != "invalid_cursor" {
		t.Fatalf("cross actor cursor: %+v", r)
	}
	args["params"].(map[string]any)["page_size"] = 10.0
	r = s.Invoke(context.Background(), actor("alice"), "wiki.nodes.list", args)
	if r.Error == nil || r.Error.Kind != "invalid_cursor" {
		t.Fatalf("changed query: %+v", r)
	}
}

func TestFilesResourcesAndTransportIsolation(t *testing.T) {
	s := startService(t, testConfig(t), &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	session, transport := connect(t, server.URL, "2026-07-28", "fixture-key", "alice")
	r := call(t, session, "feishu_file_create", map[string]any{"name": "note.txt", "text": "你好"})
	if len(r.Files) != 1 {
		t.Fatalf("%+v", r)
	}
	file := r.Files[0]
	resource, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: file.URI})
	if err != nil || resource.Contents[0].Text != "你好" {
		t.Fatalf("resource: %+v %v", resource, err)
	}
	transport.sender.Store("bob")
	if _, err = session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: file.URI}); err == nil {
		t.Fatal("cross actor resource accepted")
	}
	r = call(t, session, "feishu_file_read", map[string]any{"file_id": file.ID})
	if r.Error == nil {
		t.Fatal("cross actor fallback read accepted")
	}
	for _, who := range []string{"", "bob", "alice"} {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/files/"+file.ID, nil)
		req.Header.Set("Authorization", "Bearer fixture-key")
		if who != "" {
			req.Header.Set("X-QoderWake-AtWaker-Sender-Id", who)
			req.Header.Set("X-QoderWake-AtWaker-Sender-Channel", "feishu")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if who == "alice" {
			if resp.StatusCode != 200 || string(body) != "你好" {
				t.Fatalf("download: %d %s", resp.StatusCode, body)
			}
		} else if resp.StatusCode == 200 {
			t.Fatalf("download allowed: %s", who)
		}
		if !strings.Contains(resp.Header.Get("Cache-Control"), "private") {
			t.Fatal("private caching absent")
		}
	}
	if _, err := s.Store.PutFile(actor("alice"), "../escape", "text/plain", []byte("x")); err == nil {
		t.Fatal("path filename accepted")
	}
	if _, err := s.Store.PutFile(actor("alice"), "bad\x00name", "text/plain", []byte("x")); err == nil {
		t.Fatal("control character filename accepted")
	}
	s.Store.db.Exec(`UPDATE files SET expires=0 WHERE id=?`, file.ID)
	if err := s.Store.Collect(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.File(actor("alice"), file.ID); err == nil {
		t.Fatal("expired file read")
	}
}

func TestJobAuthorizationCancelAndResume(t *testing.T) {
	started := make(chan struct{}, 1)
	f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, a App) ExecutionResult {
		started <- struct{}{}
		<-ctx.Done()
		return ExecutionResult{Error: fault("cancelled", "stopped")}
	}}
	c := testConfig(t)
	s := startService(t, c, f)
	args := argsSpace("space-a")
	args["async"] = true
	r := s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", args)
	if r.Status != "queued" || r.JobID == "" {
		t.Fatalf("%+v", r)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("job not started")
	}
	if cross := s.Job(actor("bob"), r.JobID, false); cross.Error == nil {
		t.Fatal("cross actor job accepted")
	}
	s.Job(actor("alice"), r.JobID, true)
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := s.Job(actor("alice"), r.JobID, false)
		if got.Status == "cancelled" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cancel failed: %+v", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRestartMarksRunningUnknownAndResumesQueued(t *testing.T) {
	c := testConfig(t)
	store, err := OpenStore(c)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	op, _ := cat.Operation("wiki.spaces.get")
	running, _, _ := store.Reserve(actor("alice"), op, argsSpace("space-a"), "", false)
	queued, _, _ := store.Reserve(actor("alice"), op, argsSpace("space-a"), "", true)
	store.Close()
	f := &fakeExecutor{}
	s := startService(t, c, f)
	r, err := s.Store.Record(actor("alice"), running.ID)
	if err != nil || r.State != "unknown" {
		t.Fatalf("restart: %+v %v", r, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, err = s.Store.Record(actor("alice"), queued.ID)
		if err != nil {
			t.Fatal(err)
		}
		if r.State == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("queued: %+v", r)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if f.calls.Load() != 1 {
		t.Fatal("running job was replayed")
	}
}

func TestHTTPAuthenticationOriginAndUpload(t *testing.T) {
	s := startService(t, testConfig(t), &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	for _, test := range []struct {
		key, origin, actor string
		want               int
	}{{"", "", "", 401}, {"wrong", "", "", 401}, {"fixture-key", "https://untrusted.invalid", "alice", 403}, {"fixture-key", "", "", 403}, {"fixture-key", "", "alice", 201}} {
		req, _ := http.NewRequest("POST", server.URL+"/files?name=data.bin", bytes.NewBufferString("fixture"))
		if test.key != "" {
			req.Header.Set("Authorization", "Bearer "+test.key)
		}
		req.Header.Set("Origin", test.origin)
		if test.actor != "" {
			req.Header.Set("X-QoderWake-AtWaker-Sender-Id", test.actor)
			req.Header.Set("X-QoderWake-AtWaker-Sender-Channel", "feishu")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != test.want {
			t.Errorf("%+v: %d", test, resp.StatusCode)
		}
	}
}

func TestConfigRejectsIdentityAndCredentialAmbiguity(t *testing.T) {
	c := testConfig(t)
	c.Clients[0].IdentityMode = "auto"
	if c.Validate() == nil {
		t.Fatal("auto identity accepted")
	}
	c = testConfig(t)
	c.Clients[1].KeySHA256 = c.Clients[0].KeySHA256
	if c.Validate() == nil {
		t.Fatal("duplicate credential accepted")
	}
	c = testConfig(t)
	c.Listen = "0.0.0.0:18790"
	if c.Validate() == nil {
		t.Fatal("remote plaintext default accepted")
	}
}

func TestOutputSpillAndFileReauthorization(t *testing.T) {
	c := testConfig(t)
	c.MaxInlineBytes = 1024
	f := &fakeExecutor{fn: func(context.Context, Invocation, App) ExecutionResult {
		return ExecutionResult{Data: map[string]any{"space": map[string]any{"name": strings.Repeat("a", 2048)}}}
	}}
	s, err := NewService(c, f)
	if err != nil {
		t.Fatal(err)
	}
	r := s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", argsSpace("space-a"))
	if len(r.Files) != 1 || r.Data != nil {
		t.Fatalf("spill: %+v", r)
	}
	fileID := r.Files[0].ID
	s.Close()
	c.Clients[0].Actors["feishu:user_id:alice"] = ActorPolicy{Resources: map[string][]string{"wiki_space": {"space-b"}}}
	s2 := startService(t, c, &fakeExecutor{})
	if _, err = s2.AuthorizeFile(actor("alice"), fileID); err == nil {
		t.Fatal("revoked source resource still readable")
	}
}

func TestCatalogAppIdentityAndHighRiskGates(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Operations) != 599 {
		t.Fatalf("catalog size %d", len(c.Operations))
	}
	for _, id := range []string{"wiki.spaces.create"} {
		if _, ok := c.Operation(id); ok {
			t.Fatalf("unsupported operation exposed: %s", id)
		}
	}
}

// 枚举值与参数类型一致，数组参数的枚举约束元素；否则该参数的任何取值都会被校验拒绝。
func TestCatalogEnumsMatchDeclaredTypes(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var walk func(id, path string, node any)
	walk = func(id, path string, node any) {
		switch v := node.(type) {
		case map[string]any:
			if values, ok := v["enum"].([]any); ok {
				kind, _ := v["type"].(string)
				for _, value := range values {
					var match bool
					switch kind {
					case "integer":
						f, ok := value.(float64)
						match = ok && f == float64(int64(f))
					case "number":
						_, match = value.(float64)
					case "boolean":
						_, match = value.(bool)
					case "string", "":
						_, match = value.(string)
					}
					if !match {
						t.Errorf("%s %s: 枚举值 %v 与类型 %q 不一致", id, path, value, kind)
						break
					}
				}
			}
			for key, child := range v {
				walk(id, path+"/"+key, child)
			}
		case []any:
			for _, child := range v {
				walk(id, path, child)
			}
		}
	}
	for _, op := range c.Operations {
		walk(op.ID, "", op.InputSchema)
	}
}

// sheets 的 JSON 参数按 lark-cli 的解析方式校验：properties 只传一部分，其余字段来自 rule_type、range 等平铺参数；
// sheets、styles 接受 {"sheets": [...]} 外层对象或数组本身；枚举仍然生效。
func TestSheetsJSONParametersAcceptCLIShapes(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	url := "https://example.feishu.cn/sheets/shtXXX"
	sheet := []any{map[string]any{"name": "明细", "columns": []any{"日期", "金额"}, "data": []any{[]any{"2024-01-15", 1234.5}}}}
	cases := []struct {
		id   string
		args map[string]any
		ok   bool
	}{
		{"sheets.+cond-format-create", map[string]any{"url": url, "sheet_id": "sh1", "rule_type": "duplicateValues", "ranges": `["A1:A100"]`, "properties": map[string]any{"style": map[string]any{"back_color": "#FFD7D7"}}}, true},
		{"sheets.+filter-create", map[string]any{"url": url, "sheet_id": "sh1", "range": "A1:F100", "properties": map[string]any{"rules": []any{}}}, true},
		{"sheets.+table-put", map[string]any{"url": url, "sheets": map[string]any{"sheets": sheet}}, true},
		{"sheets.+table-put", map[string]any{"url": url, "sheets": sheet}, true},
		{"sheets.+styles-put", map[string]any{"url": url, "styles": map[string]any{"styles": []any{map[string]any{"name": "明细", "cell_merges": []any{map[string]any{"range": "A1:B1"}}}}}}, true},
		{"sheets.+cells-get", map[string]any{"url": url, "range": "A1:B2", "include": []any{"formula", "style"}}, true},
		{"sheets.+cells-get", map[string]any{"url": url, "range": "A1:B2", "include": []any{"bogus"}}, false},
		{"sheets.+chart-config-update", map[string]any{"url": url, "sheet_id": "sh1", "chart_id": "chrXXX", "x_axis_label_angle": -45.0}, true},
		{"sheets.+chart-config-update", map[string]any{"url": url, "sheet_id": "sh1", "chart_id": "chrXXX", "x_axis_label_angle": 30.0}, false},
	}
	for _, tc := range cases {
		op, ok := c.Operation(tc.id)
		if !ok {
			t.Fatalf("missing %s", tc.id)
		}
		if op.Risk != "read" {
			tc.args["idempotency_key"] = "fixture"
		}
		if err := op.Validate(tc.args); (err == nil) != tc.ok {
			t.Errorf("%s %v: want ok=%v, got %v", tc.id, tc.args, tc.ok, err)
		}
	}
}

func TestCLISubprocessEnvironmentAndMapping(t *testing.T) {
	c := testConfig(t)
	dir := t.TempDir()
	script := fixtureHeader() + `if [ "$LARKSUITE_CLI_DEFAULT_AS" != bot ] || [ "$LARKSUITE_CLI_STRICT_MODE" != bot ] || [ -n "$LARKSUITE_CLI_USER_ACCESS_TOKEN" ] || [ -z "$LARKSUITE_CLI_AUTH_PROXY" ] || [ -n "$LARKSUITE_CLI_APP_SECRET" ] || [ -n "$LARKSUITE_CLI_TENANT_ACCESS_TOKEN" ] || [ -n "$AWS_SECRET_ACCESS_KEY" ]; then exit 9; fi
if [ "$1" != wiki ] || [ "$2" != spaces ] || [ "$3" != get ] || [ "$4" != --as ] || [ "$5" != bot ]; then exit 8; fi
if [ "$HOME" != "$PWD/.gateway" ] || [ "$LARKSUITE_CLI_CONFIG_DIR" != "$PWD/.gateway/config" ]; then exit 7; fi
printf '%s\n' '{"ok":true,"identity":"bot","data":{"space":{"name":"fixture"}}}'
`
	path := filepath.Join(dir, "fixture-cli")
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	c.CLIBinary = path
	c.CLISHA256 = digest([]byte(script))
	t.Setenv("LARKSUITE_CLI_USER_ACCESS_TOKEN", "host-user-token")
	t.Setenv("LARKSUITE_CLI_AUTH_PROXY", "http://malicious.invalid")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "host-secret")
	e, err := NewCLIExecutor(c)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := NewService(c, e)
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	op, _ := cat.Operation("wiki.spaces.get")
	app, _ := c.app("app-a")
	result := e.Execute(context.Background(), Invocation{Principal: actor("alice"), Operation: op, Arguments: argsSpace("space-a")}, app)
	if result.Error != nil {
		t.Fatalf("%+v", result)
	}
	entries, _ := os.ReadDir(filepath.Join(c.DataDir, "work"))
	if len(entries) != 0 {
		t.Fatal("temporary execution files retained")
	}
	os.WriteFile(path, []byte(script+"\n"), 0700)
	if r := e.Execute(context.Background(), Invocation{Operation: op}, app); r.Error == nil || r.Error.Kind != "backend_version_mismatch" {
		t.Fatal("modified binary accepted")
	}
}

func TestExclusiveStoreAndNewerSchema(t *testing.T) {
	c := testConfig(t)
	s, err := OpenStore(c)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := OpenStore(c); err == nil {
		other.Close()
		t.Fatal("duplicate process accepted")
	}
	if _, err = s.db.Exec(`PRAGMA user_version=99`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if other, err := OpenStore(c); err == nil {
		other.Close()
		t.Fatal("newer database accepted")
	}
}

func TestFileQuotaAndJobQueueLimit(t *testing.T) {
	c := testConfig(t)
	c.MaxFileBytes = 1024
	c.MaxStorageBytes = 1024
	c.MaxPendingJobs = 1
	store, err := OpenStore(c)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.PutFile(actor("alice"), "one", "text/plain", make([]byte, 700)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutFile(actor("bob"), "two", "text/plain", make([]byte, 700)); err == nil {
		t.Fatal("client quota bypassed across actors")
	}
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	op, _ := cat.Operation("wiki.spaces.get")
	if _, _, err = store.Reserve(actor("alice"), op, argsSpace("space-a"), "", true); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.Reserve(actor("bob"), op, argsSpace("space-b"), "", true); err == nil {
		t.Fatal("client job queue bypassed across actors")
	}
}

func TestApplicationBindingAndConcurrentActors(t *testing.T) {
	f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, a App) ExecutionResult {
		if in.Principal.AppID != a.ID {
			t.Errorf("wrong app: %s vs %s", in.Principal.AppID, a.ID)
		}
		return ExecutionResult{Data: map[string]any{"space": map[string]any{"name": a.ID}}}
	}}
	s := startService(t, testConfig(t), f)
	var wg sync.WaitGroup
	for _, p := range []Principal{actor("alice"), actor("bob"), {ClientID: "desktop", AppID: "app-b", OrgID: "org-b"}} {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			space := "space-a"
			if p.Actor == "feishu:user_id:bob" {
				space = "space-b"
			}
			r := s.Invoke(context.Background(), p, "wiki.spaces.get", argsSpace(space))
			if r.Error != nil {
				t.Errorf("%+v", r)
			}
		}()
	}
	wg.Wait()
	if f.calls.Load() != 3 {
		t.Fatal("calls missing")
	}
	forged := actor("alice")
	forged.AppID = "app-b"
	r := s.Invoke(context.Background(), forged, "wiki.spaces.get", argsSpace("space-a"))
	if r.Error == nil {
		t.Fatal("forged app binding accepted")
	}
}

func TestCLITypedErrorsPartialAndWrongIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, stdout, stderr string
		want                 string
		uncertain            bool
	}{
		{"identity", `{"ok":true,"identity":"user","data":{}}`, "", "identity_violation", false},
		{"partial", `{"ok":false,"identity":"bot","data":{"items":[{"status":"succeeded"},{"status":"failed"}]},"meta":{"count":2}}`, "", "partial_failure", false},
		{"auth", "", `{"ok":false,"identity":"bot","error":{"type":"authorization","subtype":"missing_scope","code":99991679,"message":"run auth login","missing_scopes":["scope"],"log_id":"fixture-log"}}`, "upstream_authorization_missing_scope", false},
		{"network", "", `{"ok":false,"identity":"bot","error":{"type":"network","subtype":"timeout","message":"timeout"}}`, "upstream_network_timeout", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testConfig(t)
			script := fixtureHeader()
			if tc.stdout != "" {
				script += "printf '%s\\n' '" + tc.stdout + "'\n"
			}
			if tc.stderr != "" {
				script += "printf '%s\\n' '" + tc.stderr + "' >&2\n"
			}
			script += "exit 1\n"
			path := filepath.Join(t.TempDir(), "fixture-cli")
			if err := os.WriteFile(path, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			c.CLIBinary = path
			c.CLISHA256 = digest([]byte(script))
			e, err := NewCLIExecutor(c)
			if err != nil {
				t.Fatal(err)
			}
			bound, err := NewService(c, e)
			if err != nil {
				t.Fatal(err)
			}
			defer bound.Close()
			cat, err := LoadCatalog()
			if err != nil {
				t.Fatal(err)
			}
			op, _ := cat.Operation("drive.files.create_folder")
			a, _ := c.app("app-a")
			r := e.Execute(context.Background(), Invocation{Principal: actor("alice"), Operation: op, Arguments: map[string]any{"data": map[string]any{"folder_token": "folder-a", "name": "fixture"}, "idempotency_key": "fixture"}}, a)
			if r.Error == nil || r.Error.Kind != tc.want || r.Uncertain != tc.uncertain {
				t.Fatalf("%+v", r)
			}
			if tc.name == "auth" {
				if r.Error.UpstreamCode == nil || *r.Error.UpstreamCode != 99991679 || r.Error.LogID != "fixture-log" || strings.Contains(r.Error.Message, "auth login") {
					t.Fatalf("auth mapping: %+v", r.Error)
				}
			}
			if tc.name == "partial" && (r.Data == nil || r.Meta == nil) {
				t.Fatal("partial result lost")
			}
		})
	}
}

func fixtureHeader() string {
	cat, _ := LoadCatalog()
	return "#!/bin/sh\nif [ \"$1\" = --gateway-capabilities ]; then printf '%s\\n' '{\"contract\":\"feishu-gateway-backend-v1\",\"source_commit\":\"" + cat.SourceCommit + "\",\"app_only\":true,\"rooted_fileio\":true,\"sidecar_required\":true}'; exit 0; fi\n"
}
