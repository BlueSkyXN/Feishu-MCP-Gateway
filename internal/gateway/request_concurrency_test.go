// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestConcurrentHTTPMCPRequestCredentialsRemainIsolated(t *testing.T) {
	c := requestBearerConfig(t)
	// Both requests deliberately have the same persisted identity. Distinct
	// business resources, rather than caller or app IDs, identify their tokens.
	alice := c.Clients[0].Actors["feishu:user_id:alice"]
	alice.Resources["wiki_space"] = []string{"space-a", "space-b"}
	c.Clients[0].Actors["feishu:user_id:alice"] = alice
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	expectedBySpace := map[string]string{
		"space-a": "overlapping-request-token-a",
		"space-b": "overlapping-request-token-b",
	}
	var managedRequests atomic.Int64
	tokens := NewTokenManager(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		managedRequests.Add(1)
		return jsonResponse(r, 500, map[string]any{"error": "unexpected managed token request"}), nil
	})})

	entered := make(chan string, 2)
	readTogether, finishTogether := make(chan struct{}), make(chan struct{})
	var releaseReads, releaseFinish sync.Once
	unblockReads := func() { releaseReads.Do(func() { close(readTogether) }) }
	unblockFinish := func() { releaseFinish.Do(func() { close(finishTogether) }) }
	defer unblockReads()
	defer unblockFinish()
	type observation struct {
		space                string
		contextOK, managerOK bool
		identityOK           bool
		tokenErr             error
	}
	observed := make(chan observation, 2)
	f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, app App) ExecutionResult {
		space, _ := object(in.Arguments["params"])["space_id"].(string)
		select {
		case entered <- space:
		case <-ctx.Done():
			return ExecutionResult{Error: fault("test_timeout", "concurrent entry was cancelled")}
		}
		// Neither invocation may inspect credentials until both HTTP MCP calls
		// have reached the executor. A shared last-request credential would now
		// be observable as a mismatch for one of the two business markers.
		select {
		case <-readTogether:
		case <-ctx.Done():
			return ExecutionResult{Error: fault("test_timeout", "concurrent read barrier was cancelled")}
		}
		expected, known := expectedBySpace[space]
		info := requestInfo(ctx)
		token, err := tokens.Get(ctx, app)
		check := observation{
			space: space, tokenErr: err,
			contextOK: known && info.credential != nil && info.credential.token == expected &&
				info.credential.appConnectionID == app.ID,
			managerOK: err == nil && token != nil && token.value == expected && token.kind == "Bearer" && token.key == nil,
			identityOK: in.Principal.ClientID == "wake" && in.Principal.AppID == "app-a" &&
				in.Principal.OrgID == "org-a" && in.Principal.Actor == "feishu:user_id:alice" &&
				in.Principal.Owner() == actor("alice").Owner(),
		}
		select {
		case observed <- check:
		case <-ctx.Done():
			return ExecutionResult{Error: fault("test_timeout", "concurrent observation was cancelled")}
		}
		// Keep both request contexts alive until both reads have completed.
		select {
		case <-finishTogether:
		case <-ctx.Done():
			return ExecutionResult{Error: fault("test_timeout", "concurrent finish barrier was cancelled")}
		}
		if !check.contextOK || !check.managerOK || !check.identityOK {
			return ExecutionResult{Error: fault("test_credential_mismatch", "request credentials crossed invocation boundaries")}
		}
		return ExecutionResult{Data: map[string]any{"space": map[string]any{"space_id": space, "name": "fixture"}}}
	}}
	s := startService(t, c, f)
	server := httptest.NewServer(s.Handler())
	t.Cleanup(server.Close)
	first, _ := connectRequestHeaders(t, server.URL, requestHeaders(expectedBySpace["space-a"]))
	second, _ := connectRequestHeaders(t, server.URL, requestHeaders(expectedBySpace["space-b"]))
	op, ok := s.Catalog.Operation("wiki.spaces.get")
	if !ok {
		t.Fatal("fixture operation missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	type outcome struct {
		space   string
		result  Result
		isError bool
		err     error
	}
	completed := make(chan outcome, 2)
	for _, request := range []struct {
		space   string
		session *mcp.ClientSession
	}{{"space-a", first}, {"space-b", second}} {
		go func(space string, session *mcp.ClientSession) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: op.Tool, Arguments: argsSpace(space)})
			out := outcome{space: space, err: err}
			if err == nil {
				out.isError = result.IsError
				out.err = json.Unmarshal(encode(result.StructuredContent), &out.result)
			}
			completed <- out
		}(request.space, request.session)
	}

	seen := make(map[string]bool, 2)
	for range 2 {
		select {
		case space := <-entered:
			if seen[space] || expectedBySpace[space] == "" {
				t.Fatal("executor entry did not preserve independent business markers")
			}
			seen[space] = true
		case out := <-completed:
			t.Fatalf("request ended before both invocations overlapped: space=%s error=%v status=%s", out.space, out.err, out.result.Status)
		case <-ctx.Done():
			t.Fatal("both HTTP MCP requests did not reach the executor before timeout")
		}
	}
	unblockReads()
	for range 2 {
		select {
		case check := <-observed:
			if !check.contextOK || !check.managerOK || !check.identityOK {
				t.Errorf("overlapping request lost its own credential or identity: space=%s context=%t manager=%t identity=%t error=%v", check.space, check.contextOK, check.managerOK, check.identityOK, check.tokenErr)
			}
		case <-ctx.Done():
			t.Fatal("concurrent credential reads did not complete before timeout")
		}
	}
	unblockFinish()
	invocations := make(map[string]bool, 2)
	for range 2 {
		select {
		case out := <-completed:
			if out.err != nil || out.isError || out.result.Status != "succeeded" || out.result.Error != nil {
				t.Fatalf("concurrent HTTP MCP call failed: space=%s error=%v status=%s", out.space, out.err, out.result.Status)
			}
			if object(object(out.result.Data)["space"])["space_id"] != out.space {
				t.Fatal("concurrent response returned another request's business marker")
			}
			invocations[out.result.InvocationID] = true
		case <-ctx.Done():
			t.Fatal("concurrent HTTP MCP calls did not finish before timeout")
		}
	}
	if f.calls.Load() != 2 || len(invocations) != 2 || managedRequests.Load() != 0 {
		t.Fatal("requests were not independently executed or attempted managed token issuance")
	}
	tokens.mu.Lock()
	slots := len(tokens.slots)
	tokens.mu.Unlock()
	if slots != 0 || tokens.Stats()["cached_app_tokens"] != 0 || tokens.Stats()["successful_issuances"] != 0 {
		t.Fatal("concurrent request tokens entered the managed token cache")
	}
	var rows, owners int
	if err := s.Store.db.QueryRow(`SELECT COUNT(*),COUNT(DISTINCT owner) FROM invocations`).Scan(&rows, &owners); err != nil || rows != 2 || owners != 1 {
		t.Fatal("concurrent request records did not retain the same stable owner", err)
	}
	assertNoRecordedCredentials(t, s, expectedBySpace["space-a"], expectedBySpace["space-b"])
}
