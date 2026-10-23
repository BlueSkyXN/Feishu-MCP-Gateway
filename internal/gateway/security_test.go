package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func validateProof(t *testing.T, proof, method, htu string) {
	t.Helper()
	parts := strings.Split(proof, ".")
	if len(parts) != 3 {
		t.Fatal("missing compact JWS")
	}
	var h, c map[string]any
	b, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if json.Unmarshal(b, &h) != nil {
		t.Fatal("bad header")
	}
	b, _ = base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(b, &c)
	if c["htm"] != method || c["htu"] != htu || c["jti"] == "" {
		t.Fatalf("invalid claims: %v", c)
	}
	jwk := object(h["jwk"])
	xb, _ := base64.RawURLEncoding.DecodeString(jwk["x"].(string))
	yb, _ := base64.RawURLEncoding.DecodeString(jwk["y"].(string))
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(&ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(xb), Y: new(big.Int).SetBytes(yb)}, hash[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("signature invalid")
	}
}
func TestTokenCacheDPoPIsolationRevocationAndFailure(t *testing.T) {
	var issues atomic.Int32
	var mode atomic.Int32
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "heartbeat") {
			return jsonResponse(r, 200, map[string]any{"code": 0, "data": map[string]any{"now": fmt.Sprint(time.Now().Unix())}}), nil
		}
		issues.Add(1)
		if mode.Load() == 1 {
			resp := jsonResponse(r, 429, map[string]any{})
			resp.Header.Set("Retry-After", "2")
			return resp, nil
		}
		validateProof(t, r.Header.Get("DPoP"), "POST", r.URL.String())
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Error("non-app auth")
		}
		return jsonResponse(r, 200, map[string]any{"access_token": "fixture-token", "expires_in": 100, "token_type": "DPoP"}), nil
	})
	tm := NewTokenManager(&http.Client{Transport: transport})
	a := App{ID: "app", AppID: "cli_1", OrgID: "org", Brand: "feishu", secret: "fixture", DPoPMode: "required"}
	var wg sync.WaitGroup
	tokens := make(chan *appToken, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := tm.Get(context.Background(), a)
			if err != nil {
				t.Error(err)
			}
			tokens <- tok
		}()
	}
	wg.Wait()
	close(tokens)
	var first *appToken
	for tok := range tokens {
		if first == nil {
			first = tok
		}
		if first != tok {
			t.Error("concurrent issuance not merged")
		}
	}
	if issues.Load() != 1 {
		t.Fatal(issues.Load())
	}
	proof, err := signDPoP(first.key, "GET", "https://OPEN.feishu.cn:443/a/../%7euser?query=x", 0)
	if err != nil {
		t.Fatal(err)
	}
	validateProof(t, proof, "GET", "https://open.feishu.cn/~user")
	a2 := a
	a2.ID = "second"
	if _, err = tm.Get(context.Background(), a2); err != nil {
		t.Fatal(err)
	}
	if issues.Load() != 2 {
		t.Fatal("app cache isolation")
	}
	tm.Invalidate(a, first)
	if _, err = tm.Get(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if issues.Load() != 3 {
		t.Fatal("cache invalidation")
	}
	mode.Store(1)
	a.secret = "new-fixture"
	for i := 0; i < 3; i++ {
		_, err = tm.Get(context.Background(), a)
		f, ok := err.(*Fault)
		if !ok || f.Kind != "token_rate_limit" || f.RetryAfter == nil {
			t.Fatalf("%v", err)
		}
	}
	if issues.Load() != 4 {
		t.Fatal("failed issuance not coalesced")
	}
	tm.Retain([]App{a2})
	if tm.Stats()["cached_app_tokens"] != 1 {
		t.Fatal(tm.Stats())
	}
}

func proxyRequest(t *testing.T, session *ProxySession, method, target, path, body, nonce string) *http.Request {
	t.Helper()
	stamp := fmt.Sprint(time.Now().Unix())
	sum := digest([]byte(body))
	canonical := strings.Join([]string{"gateway-v1", method, target, path, sum, stamp, "bot", "Authorization", nonce, "application/json", ""}, "\n")
	mac := hmac.New(sha256.New, []byte(session.Key))
	mac.Write([]byte(canonical))
	r, _ := http.NewRequest(method, session.URL+path, strings.NewReader(body))
	for k, v := range map[string]string{"X-Lark-Proxy-Version": "gateway-v1", "X-Lark-Proxy-Identity": "bot", "X-Lark-Proxy-Auth-Header": "Authorization", "X-Feishu-Gateway-Request-Id": nonce, "X-Lark-Proxy-Target": "https://" + target, "X-Lark-Proxy-Timestamp": stamp, "X-Lark-Body-SHA256": sum, "X-Lark-Proxy-Signature": hex.EncodeToString(mac.Sum(nil)), "Content-Type": "application/json"} {
		r.Header.Set(k, v)
	}
	return r
}
func TestBrokerSignedGrantReplayPathAndRevocation(t *testing.T) {
	c := testConfig(t)
	a, _ := c.app("app-a")
	a.DPoPMode = "disabled"
	cat, _ := LoadCatalog()
	op, _ := cat.Operation("wiki.spaces.get")
	var calls atomic.Int32
	var revoked atomic.Bool
	broker := NewBroker(c.MaxFileBytes, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "oauth") {
			return jsonResponse(r, 200, map[string]any{"access_token": "fixture", "token_type": "Bearer", "expires_in": 100}), nil
		}
		calls.Add(1)
		return jsonResponse(r, 200, map[string]any{"code": 0, "data": map[string]any{}}), nil
	})}, func(in Invocation) (App, error) {
		if revoked.Load() {
			return App{}, fault("revoked", "revoked")
		}
		return a, nil
	})
	session, err := broker.Open(context.Background(), Invocation{Principal: actor("alice"), Operation: op, Arguments: argsSpace("space-a")})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	send := func(r *http.Request) int {
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	for i := 0; i < 2; i++ {
		if code := send(proxyRequest(t, session, "GET", "open.feishu.cn", op.PathReplace("space_id", "space-a"), "", "nonce-a")); code != 200 {
			t.Fatal(code)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("replay repeated upstream request")
	}
	r := proxyRequest(t, session, "GET", "open.feishu.cn", op.PathReplace("space_id", "space-a"), "", "nonce-b")
	r.Header.Set("X-Lark-Proxy-Identity", "user")
	if send(r) != 403 {
		t.Fatal("user identity accepted")
	}
	if send(proxyRequest(t, session, "GET", "open.feishu.cn", op.PathReplace("space_id", "space-b"), "", "nonce-c")) != 403 {
		t.Fatal("other resource allowed")
	}
	r = proxyRequest(t, session, "GET", "open.feishu.cn", op.PathReplace("space_id", "space-a"), "", "nonce-d")
	r.Header.Set("Content-Type", "text/plain")
	if send(r) != 401 {
		t.Fatal("unsigned body metadata accepted")
	}
	revoked.Store(true)
	if send(proxyRequest(t, session, "GET", "open.feishu.cn", op.PathReplace("space_id", "space-a"), "", "nonce-a")) != 403 {
		t.Fatal("replay bypassed revocation")
	}
}
func (op Operation) PathReplace(k, v string) string { return strings.ReplaceAll(op.Path, "{"+k+"}", v) }

func TestReloadCompactViewsHighRiskAndLimits(t *testing.T) {
	c := testConfig(t)
	c.Clients[0].ToolView = "compact"
	s := startService(t, c, &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	session, _ := connect(t, server.URL, "2026-07-28", "fixture-key", "alice")
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == "feishu_wiki_spaces_get" {
			t.Fatal("compact view included named tools")
		}
	}
	r := call(t, session, "feishu_tools_search", map[string]any{"query": "wiki.spaces.get"})
	if r.Error != nil || len(object(r.Data)["operations"].([]any)) != 1 {
		t.Fatal(r)
	}
	r = call(t, session, "feishu_execute_read", map[string]any{"operation_id": "wiki.spaces.get", "arguments": argsSpace("space-a")})
	if r.Status != "succeeded" {
		t.Fatal(r)
	}
	r = call(t, session, "feishu_execute_read", map[string]any{"operation_id": "drive.files.create_folder", "arguments": map[string]any{}})
	if r.Error.Kind != "wrong_executor" {
		t.Fatal(r)
	}
	next := cloneConfig(c)
	next.Clients[0].Actors[actor("alice").Actor] = ActorPolicy{Operations: []string{"wiki.nodes.list"}}
	if err = s.Reload(next); err != nil {
		t.Fatal(err)
	}
	r = call(t, session, "feishu_execute_read", map[string]any{"operation_id": "wiki.spaces.get", "arguments": argsSpace("space-a")})
	if r.Error == nil {
		t.Fatal("actor policy was cached")
	}
	next = cloneConfig(c)
	next.Apps[0].AppID = "other"
	if s.Reload(next) == nil {
		t.Fatal("app rebound in place")
	}
	next = cloneConfig(c)
	next.Clients[0].Packs = nil
	next.Clients[0].Operations = []string{"drive.permission.members.create"}
	next.Clients[0].ResourceMode = "application_scope"
	next.Clients[0].Actors = map[string]ActorPolicy{actor("alice").Actor: {}}
	if s.Reload(next) != nil {
		t.Fatal("reload failed")
	}
	op, _ := s.Catalog.Operation("drive.permission.members.create")
	if visible(next.Clients[0], op) {
		t.Fatal("high risk operation allowed by pack")
	}
	next.Clients[0].HighRiskOperations = []string{op.ID}
	if !visible(next.Clients[0], op) {
		t.Fatal("exact high risk preauthorization ignored")
	}
	next = cloneConfig(c)
	next.Clients[0].MaxRequestsPerMinute = 1
	next.Clients[0].Actors[actor("alice").Actor] = ActorPolicy{MaxConcurrent: 1}
	if err = s.Reload(next); err != nil {
		t.Fatal(err)
	}
	if err = s.acquirePrincipal(actor("alice")); err != nil {
		t.Fatal(err)
	}
	if s.acquirePrincipal(actor("alice")) == nil {
		t.Fatal("actor concurrency not enforced")
	}
	if err = s.acquirePrincipal(actor("bob")); err != nil {
		t.Fatal(err)
	}
	s.releasePrincipal(actor("bob"))
	s.releasePrincipal(actor("alice"))
	if s.rate(actor("alice")) != nil {
		t.Fatal("initial rate call")
	}
	if s.rate(actor("bob")) == nil {
		t.Fatal("shared client rate limit ignored")
	}
}
func cloneConfig(c Config) Config { var out Config; _ = json.Unmarshal(encode(c), &out); return out }

func TestInputPinsPreventExpiryAndDeletion(t *testing.T) {
	c := testConfig(t)
	store, err := OpenStore(c)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cat, _ := LoadCatalog()
	op, _ := cat.Operation("docs.+create")
	f, err := store.PutFile(actor("alice"), "input.txt", "text/plain", []byte("fixture"))
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"content": map[string]any{"file_id": f.ID}, "idempotency_key": "pin"}
	record, _, err := store.Reserve(actor("alice"), op, args, "pin", true)
	if err != nil {
		t.Fatal(err)
	}
	if store.DeleteFile(actor("alice"), f.ID) == nil {
		t.Fatal("pinned file deleted")
	}
	store.db.Exec(`UPDATE files SET expires=0,meta=json_set(meta,'$.expires_at',0) WHERE id=?`, f.ID)
	if err = store.Collect(); err != nil {
		t.Fatal(err)
	}
	if _, err = store.readPinned(actor("alice"), f.ID, record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.File(actor("alice"), f.ID); err == nil {
		t.Fatal("expired unpinned read")
	}
	if _, err = store.readPinned(actor("bob"), f.ID, record.ID); err == nil {
		t.Fatal("pin granted ownership")
	}
	store.Cancel(actor("alice"), record.ID)
	store.Collect()
	if _, err = store.readPinned(actor("alice"), f.ID, record.ID); err == nil {
		t.Fatal("cancelled task retained access")
	}
}

func TestFlatShortcutCursorAndStrictResourceEgress(t *testing.T) {
	c := testConfig(t)
	store, err := OpenStore(c)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	args := map[string]any{"doc": "docFixture", "page_size": float64(2)}
	handle, err := store.WrapCursor(actor("alice"), "docs.+history-list", args, "upstream-page")
	if err != nil {
		t.Fatal(err)
	}
	next := cloneArguments(args)
	next["page_token"] = handle
	if err = store.UnwrapCursor(actor("alice"), "docs.+history-list", next); err != nil || next["page_token"] != "upstream-page" {
		t.Fatal("flat cursor failed", err, next)
	}
	next = cloneArguments(args)
	next["page_token"] = handle
	next["doc"] = "otherDoc"
	if store.UnwrapCursor(actor("alice"), "docs.+history-list", next) == nil {
		t.Fatal("changed resource accepted old cursor")
	}
	cat, _ := LoadCatalog()
	op, _ := cat.Operation("sheets.+workbook-info")
	g := &proxyGrant{inv: Invocation{Operation: op, Arguments: map[string]any{"spreadsheet_token": "shtA"}}}
	for _, tc := range []struct {
		target  string
		allowed bool
	}{{"/open-apis/sheet_ai/v2/spreadsheets/shtA/tools/invoke_read", true}, {"/open-apis/sheet_ai/v2/spreadsheets/shtB/tools/invoke_read", false}, {"/open-apis/sheet_ai/v2/spreadsheets/shtA/tools/invoke_write", false}} {
		r, _ := http.NewRequest("POST", "https://open.feishu.cn"+tc.target, nil)
		err = g.allowRequest(r, []byte(`{}`), true)
		if (err == nil) != tc.allowed {
			t.Errorf("%s: %v", tc.target, err)
		}
	}
}

func TestGenericEgressQueryAndFileIntegrity(t *testing.T) {
	cat, _ := LoadCatalog()
	op, _ := cat.Operation("wiki.nodes.list")
	g := &proxyGrant{inv: Invocation{Operation: op, Arguments: map[string]any{"params": map[string]any{"space_id": "space-a", "page_size": float64(2)}}}}
	r, _ := http.NewRequest("GET", "https://open.feishu.cn/open-apis/wiki/v2/spaces/space-a/nodes?page_size=3", nil)
	if g.allowRequest(r, nil, false) == nil {
		t.Fatal("query tampering accepted")
	}
	r.URL.RawQuery = "page_size=2"
	if err := g.allowRequest(r, nil, false); err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	if g.allowRequest(r, []byte(`{"unexpected":true}`), false) == nil {
		t.Fatal("unrequested request body accepted")
	}
}

func TestOutboundPacingHonorsCooldownAndCancellation(t *testing.T) {
	b := NewBroker(1000, &http.Client{}, nil)
	a := App{ID: "app", OutboundRequestsPerSecond: 1000}
	r := &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"10"}}}
	b.rememberRateLimit(a, r)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	if b.pace(ctx, a) == nil {
		t.Fatal("cooldown ignored")
	}
	b.paceMu.Lock()
	b.cooldown = map[string]time.Time{}
	b.nextRequest = map[string]time.Time{}
	b.paceMu.Unlock()
	if err := b.pace(context.Background(), a); err != nil {
		t.Fatal(err)
	}
}

func TestAdminCredentialAndMetadataSpill(t *testing.T) {
	c := testConfig(t)
	c.AdminKeySHA256 = digest([]byte("fixture-admin"))
	c.MaxInlineBytes = 1024
	s := startService(t, c, &fakeExecutor{fn: func(context.Context, Invocation, App) ExecutionResult {
		return ExecutionResult{Data: map[string]any{"space": map[string]any{}}, Meta: map[string]any{"details": strings.Repeat("x", 5000)}}
	}})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	for _, tc := range []struct {
		key    string
		status int
	}{{"fixture-key", 401}, {"fixture-admin", 200}} {
		r, _ := http.NewRequest("GET", server.URL+"/admin/status", nil)
		r.Header.Set("Authorization", "Bearer "+tc.key)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("status %d", resp.StatusCode)
		}
		if strings.Contains(string(b), "fixture-app-secret") {
			t.Fatal("diagnostics leaked secret")
		}
	}
	r := s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", argsSpace("space-a"))
	if r.Data != nil || len(r.Files) != 1 || len(encode(r)) > 1024 {
		t.Fatalf("metadata not bounded: %s", encode(r))
	}
	_, b, err := s.Store.ReadFile(actor("alice"), r.Files[0].ID, 0, 65536)
	if err != nil || !strings.Contains(string(b), "details") {
		t.Fatal("metadata spill lost content", err)
	}
}

func TestShutdownWaitsForSynchronousReceipt(t *testing.T) {
	c := testConfig(t)
	started := make(chan struct{})
	f := &fakeExecutor{fn: func(ctx context.Context, _ Invocation, _ App) ExecutionResult {
		close(started)
		<-ctx.Done()
		return ExecutionResult{Error: fault("execution_interrupted", "fixture cancellation")}
	}}
	s, err := NewService(c, f)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan Result, 1)
	go func() {
		result <- s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", argsSpace("space-a"))
	}()
	<-started
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if r := <-result; r.Status != "cancelled" {
		t.Fatalf("shutdown receipt %s", encode(r))
	}
	c2 := cloneConfig(c)
	s2, err := NewService(c2, &fakeExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	var count int
	if err = s2.Store.db.QueryRow(`SELECT COUNT(*) FROM invocations WHERE state='cancelled'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("receipt was lost before closing storage", count, err)
	}
	if r := s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", argsSpace("space-a")); r.Error == nil || r.Error.Kind != "service_stopping" {
		t.Fatal("closed service accepted invocation")
	}
}
