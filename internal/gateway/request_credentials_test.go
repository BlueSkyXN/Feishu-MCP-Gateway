// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type requestHeaderTransport struct{ headers atomic.Value }

func (x *requestHeaderTransport) setHeaders(h http.Header) { x.headers.Store(h.Clone()) }

func (x *requestHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header = req.Header.Clone()
	for name, values := range x.headers.Load().(http.Header) {
		r.Header[name] = append([]string(nil), values...)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func connectRequestHeaders(t *testing.T, endpoint string, h http.Header) (*mcp.ClientSession, *requestHeaderTransport) {
	t.Helper()
	x := &requestHeaderTransport{}
	x.setHeaders(h)
	client := mcp.NewClient(&mcp.Implementation{Name: "request-credential-test", Version: "1"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint + "/mcp", HTTPClient: &http.Client{Transport: x}}, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session, x
}

func assertNoRecordedCredentials(t *testing.T, s *Service, tokens ...string) {
	t.Helper()
	rows, err := s.Store.db.Query(`SELECT principal,args,result FROM invocations`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var principal, args, result string
		if err := rows.Scan(&principal, &args, &result); err != nil {
			t.Fatal(err)
		}
		for _, token := range tokens {
			if strings.Contains(principal, token) || strings.Contains(args, token) || strings.Contains(result, token) {
				t.Fatal("request credential was stored in an invocation")
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestTokenManagerDoesNotIssueOrCache(t *testing.T) {
	c := requestBearerConfig(t)
	app := c.Apps[0]
	var requests atomic.Int64
	m := NewTokenManager(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return jsonResponse(r, 500, map[string]any{"error": "unexpected managed token request"}), nil
	})})
	_, err := m.Get(context.Background(), app)
	requireFaultKind(t, err, "upstream_credential_required")
	for _, value := range []string{"request-token-first", "request-token-second"} {
		ctx, state := authenticatedRequestContext(t, c, requestHeaders(value))
		token, err := m.Get(ctx, app)
		if err != nil || token.value != value || token.kind != "Bearer" || token.key != nil {
			t.Fatalf("request token was not used directly: %v", err)
		}
		m.Invalidate(app, token)
		if _, err = m.Get(ctx, app); err != nil {
			t.Fatalf("request invalidation attempted managed refresh: %v", err)
		}
		for _, change := range []func(*requestCredential){
			func(v *requestCredential) { v.appConnectionID = "other-connection" },
			func(v *requestCredential) { v.appID = "cli_other" },
			func(v *requestCredential) { v.brand = "lark" },
		} {
			bad := state
			cred := *state.credential
			change(&cred)
			bad.credential = &cred
			_, err = m.Get(withRequestInfo(context.Background(), bad), app)
			requireFaultKind(t, err, "upstream_app_mismatch")
		}
	}
	m.Retain(c.Apps)
	if requests.Load() != 0 || len(m.slots) != 0 || m.Stats()["cached_app_tokens"] != 0 || m.Stats()["successful_issuances"] != 0 {
		t.Fatal("request mode issued or cached an application token")
	}
}

func TestRequestTokenCannotFallBackToManagedCredentials(t *testing.T) {
	c := testConfig(t)
	app := c.Apps[0]
	app.DPoPMode = "disabled"
	var issues atomic.Int64
	m := NewTokenManager(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		issues.Add(1)
		if r.URL.Path != "/oauth/v3/token" {
			t.Error("unexpected managed token endpoint")
		}
		return jsonResponse(r, 200, map[string]any{"access_token": "managed-fixture-token", "expires_in": 7200, "token_type": "Bearer"}), nil
	})})
	ctx := withRequestInfo(context.Background(), requestContext{credential: &requestCredential{appConnectionID: app.ID, appID: app.AppID, brand: app.Brand, token: "request-fixture-token"}})
	_, err := m.Get(ctx, app)
	requireFaultKind(t, err, "credential_mode_mismatch")
	if issues.Load() != 0 {
		t.Fatal("supplied request token silently fell back to managed credentials")
	}
	for range 2 {
		token, err := m.Get(context.Background(), app)
		if err != nil || token.value != "managed-fixture-token" {
			t.Fatalf("managed token behavior changed: %v", err)
		}
	}
	if issues.Load() != 1 {
		t.Fatal("managed application token cache stopped working")
	}
}

func TestRequestTokenMissingStillAllowsLocalMCPFunctions(t *testing.T) {
	c := requestBearerConfig(t)
	f := &fakeExecutor{}
	s := startService(t, c, f)
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	session, _ := connectRequestHeaders(t, server.URL, requestHeaders(""))
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil || len(listed.Tools) < 4 {
		t.Fatalf("missing request token prevented tool discovery: %v", err)
	}
	r := call(t, session, "feishu_context", map[string]any{})
	if r.Status != "succeeded" || object(r.Data)["credential_mode"] != "request_bearer" || object(r.Data)["upstream_credential_present"] != false {
		t.Fatalf("local context unavailable: %s", encode(r))
	}
	r = call(t, session, "feishu_file_create", map[string]any{"name": "local.txt", "text": "local content"})
	if r.Status != "succeeded" || len(r.Files) != 1 {
		t.Fatalf("local file creation required upstream token: %s", encode(r))
	}
	fileID := r.Files[0].ID
	r = call(t, session, "feishu_file_stat", map[string]any{"file_id": fileID})
	if r.Status != "succeeded" {
		t.Fatal("local file stat required upstream token")
	}
	download, err := http.NewRequest(http.MethodGet, server.URL+"/files/"+fileID, nil)
	if err != nil {
		t.Fatal(err)
	}
	download.Header = requestHeaders("")
	response, err := http.DefaultClient.Do(download)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || string(content) != "local content" {
		t.Fatal("local HTTP file download required upstream token")
	}
	_, state := authenticatedRequestContext(t, c, requestHeaders(""))
	op, _ := s.Catalog.Operation("wiki.spaces.get")
	record, _, err := s.Store.Reserve(state.Principal, op, argsSpace("space-a"), "", false)
	if err != nil {
		t.Fatal(err)
	}
	receipt := record.Result
	receipt.Status = "succeeded"
	if err := s.Store.Finish(record.ID, receipt); err != nil {
		t.Fatal(err)
	}
	r = call(t, session, "feishu_job_get", map[string]any{"job_id": record.ID})
	if r.Status != "succeeded" || r.InvocationID != record.ID {
		t.Fatalf("existing local receipt required upstream token: %s", encode(r))
	}
	r = call(t, session, op.Tool, argsSpace("space-a"))
	requireFaultKind(t, r.Error, "upstream_credential_required")
	var count int
	if err := s.Store.db.QueryRow(`SELECT COUNT(*) FROM invocations`).Scan(&count); err != nil || count != 1 || f.calls.Load() != 0 {
		t.Fatal("missing credentials reserved or executed an upstream invocation", err)
	}
}

func TestRequestTokenReachesExecutorThroughHTTPMCP(t *testing.T) {
	c := requestBearerConfig(t)
	var expected atomic.Value
	expected.Store("http-request-token-one")
	f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, app App) ExecutionResult {
		info := requestInfo(ctx)
		if info.credential == nil || info.credential.token != expected.Load().(string) || info.credential.appConnectionID != app.ID {
			t.Error("HTTP MCP invocation lost request-only application credential")
		}
		if info.Caller.ID != "alice" || in.Principal.Actor != "feishu:user_id:alice" {
			t.Error("HTTP MCP invocation lost normalized caller context")
		}
		return ExecutionResult{Data: map[string]any{"space": map[string]any{"space_id": "space-a", "name": "fixture"}}}
	}}
	s := startService(t, c, f)
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	session, transport := connectRequestHeaders(t, server.URL, requestHeaders(expected.Load().(string)))
	op, _ := s.Catalog.Operation("wiki.spaces.get")
	for _, token := range []string{"http-request-token-one", "http-request-token-two"} {
		expected.Store(token)
		transport.setHeaders(requestHeaders(token))
		r := call(t, session, op.Tool, argsSpace("space-a"))
		if r.Status != "succeeded" {
			t.Fatalf("HTTP MCP request failed: %s", encode(r))
		}
	}
	if f.calls.Load() != 2 {
		t.Fatal("HTTP MCP requests were not independently executed")
	}
	assertNoRecordedCredentials(t, s, "http-request-token-one", "http-request-token-two")
}

func TestRequestTokenRotationPreservesFilesAndIdempotency(t *testing.T) {
	c := requestBearerConfig(t)
	f := &fakeExecutor{}
	s := startService(t, c, f)
	firstCtx, first := authenticatedRequestContext(t, c, requestHeaders("rotation-token-one"))
	secondCtx, second := authenticatedRequestContext(t, c, requestHeaders("rotation-token-two"))
	if first.Principal.Owner() != second.Principal.Owner() || first.Principal.Owner() != actor("alice").Owner() {
		t.Fatal("token rotation changed persisted ownership")
	}
	file, err := s.Store.PutFile(first.Principal, "existing.txt", "text/plain", []byte("stable data"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthorizeFile(second.Principal, file.ID); err != nil {
		t.Fatal("rotated token lost access to existing file", err)
	}
	args := map[string]any{"data": map[string]any{"folder_token": "folder-a", "name": "fixture"}, "idempotency_key": "rotation-write"}
	firstResult := s.Invoke(firstCtx, first.Principal, "drive.files.create_folder", args)
	secondResult := s.Invoke(secondCtx, second.Principal, "drive.files.create_folder", args)
	if firstResult.Status != "succeeded" || secondResult.Status != "succeeded" || firstResult.InvocationID != secondResult.InvocationID || f.calls.Load() != 1 {
		t.Fatal("token rotation replayed an already completed write")
	}
	assertNoRecordedCredentials(t, s, "rotation-token-one", "rotation-token-two")
}

func TestRequestAsyncRejectedBeforeReservation(t *testing.T) {
	c := requestBearerConfig(t)
	f := &fakeExecutor{}
	s := startService(t, c, f)
	ctx, state := authenticatedRequestContext(t, c, requestHeaders("async-request-token"))
	args := argsSpace("space-a")
	args["async"] = true
	for _, callCtx := range []context.Context{ctx, context.Background()} {
		r := s.Invoke(callCtx, state.Principal, "wiki.spaces.get", args)
		requireFaultKind(t, r.Error, "request_credential_async_unsupported")
	}
	var count int
	if err := s.Store.db.QueryRow(`SELECT COUNT(*) FROM invocations`).Scan(&count); err != nil || count != 0 || f.calls.Load() != 0 {
		t.Fatal("unsupported request-credential job was persisted or executed", err)
	}
}

func TestRequestCredentialModeReloadRejectsCapturedContext(t *testing.T) {
	for _, supplied := range []string{"", "mode-reload-token"} {
		name := "without-token"
		if supplied != "" {
			name = "with-token"
		}
		t.Run(name, func(t *testing.T) {
			c := requestBearerConfig(t)
			f := &fakeExecutor{}
			s := startService(t, c, f)
			ctx, state := authenticatedRequestContext(t, c, requestHeaders(supplied))
			next := cloneConfig(c)
			next.Apps[0].CredentialMode = "managed"
			next.Apps[0].SecretEnv = "TEST_FEISHU_SECRET"
			if err := s.Reload(next); err != nil {
				t.Fatal(err)
			}
			r := s.Invoke(ctx, state.Principal, "wiki.spaces.get", argsSpace("space-a"))
			requireFaultKind(t, r.Error, "access_revoked")
			var count int
			if err := s.Store.db.QueryRow(`SELECT COUNT(*) FROM invocations`).Scan(&count); err != nil || count != 0 || f.calls.Load() != 0 {
				t.Fatal("captured request changed credential modes while being executed", err)
			}
			app, _ := s.currentConfig().app(state.Principal.AppID)
			var requests atomic.Int64
			tokens := NewTokenManager(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)
				return jsonResponse(r, 500, map[string]any{"error": "unexpected request"}), nil
			})})
			_, err := tokens.Get(ctx, app)
			requireFaultKind(t, err, "access_revoked")
			if requests.Load() != 0 {
				t.Fatal("token manager refreshed after credential-mode change")
			}
		})
	}
}

func TestRequestTokenRealWrapperBrokerForwarding(t *testing.T) {
	const supplied = "real-wrapper-request-token"
	var requests atomic.Int64
	s, executor := realBackend(t, func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.URL.Path != "/open-apis/wiki/v2/spaces/space-a" {
			t.Errorf("request mode unexpectedly called %s", r.URL.Path)
			return jsonResponse(r, 500, map[string]any{"code": 1}), nil
		}
		if r.Header.Get("Authorization") != "Bearer "+supplied || r.Header.Get(upstreamTokenHeader) != "" {
			t.Error("broker did not translate the request credential into upstream authorization")
		}
		return jsonResponse(r, 200, map[string]any{"code": 0, "data": map[string]any{"space": map[string]any{"space_id": "space-a", "name": "fixture"}}}), nil
	})
	c := cloneConfig(s.currentConfig())
	c.Apps[0].CredentialMode = "request_bearer"
	c.Apps[0].SecretEnv = ""
	c.Apps[0].DPoPMode = "disabled"
	if err := s.Reload(c); err != nil {
		t.Fatal(err)
	}
	ctx, state := authenticatedRequestContext(t, s.currentConfig(), requestHeaders(supplied))
	r := s.Invoke(ctx, state.Principal, "wiki.spaces.get", argsSpace("space-a"))
	if r.Status != "succeeded" || requests.Load() != 1 {
		t.Fatalf("real wrapper request failed: %s; outbound=%d", encode(r), requests.Load())
	}
	if strings.Contains(string(encode(r)), supplied) || len(executor.broker.tokens.slots) != 0 {
		t.Fatal("real wrapper exposed or cached request token")
	}
	assertNoRecordedCredentials(t, s, supplied)
}

func TestRequestTokenAbsentFromSubprocessEnvironment(t *testing.T) {
	c := requestBearerConfig(t)
	const supplied = "fixture-request-environment-token"
	script := fixtureHeader() + `case "$(env)" in *fixture-request-environment-token*) exit 9;; esac
if [ -n "$LARKSUITE_CLI_APP_SECRET" ] || [ -n "$LARKSUITE_CLI_TENANT_ACCESS_TOKEN" ] || [ -n "$LARKSUITE_CLI_USER_ACCESS_TOKEN" ]; then exit 8; fi
if [ -z "$LARKSUITE_CLI_AUTH_PROXY" ] || [ "$LARKSUITE_CLI_APP_ID" != cli_fixture_a ]; then exit 7; fi
printf '%s\n' '{"ok":true,"identity":"bot","data":{"space":{"name":"fixture"}}}'
`
	path := filepath.Join(t.TempDir(), "request-fixture-cli")
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	c.CLIBinary, c.CLISHA256 = path, digest([]byte(script))
	executor, err := NewCLIExecutor(c)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewService(c, executor)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, state := authenticatedRequestContext(t, c, requestHeaders(supplied))
	r := s.Invoke(ctx, state.Principal, "wiki.spaces.get", argsSpace("space-a"))
	if r.Status != "succeeded" {
		t.Fatalf("request token or managed secret reached subprocess environment: %s", encode(r))
	}
	assertNoRecordedCredentials(t, s, supplied)
}
