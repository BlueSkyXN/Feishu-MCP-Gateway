// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/BlueSkyXN/Feishu-MCP-Gateway/internal/ingress"
)

func requestBearerConfig(t *testing.T) Config {
	t.Helper()
	c := testConfig(t)
	for i := range c.Apps {
		c.Apps[i].CredentialMode = "request_bearer"
		c.Apps[i].SecretEnv = ""
		c.Apps[i].DPoPMode = ""
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	return c
}

func requestHeaders(token string) http.Header {
	h := make(http.Header)
	h.Set("Authorization", "Bearer fixture-key")
	h.Set("X-QoderWake-AtWaker-Sender-Id", "alice")
	h.Set("X-QoderWake-AtWaker-Sender-Channel", "feishu")
	if token != "" {
		h.Set(upstreamTokenHeader, token)
		h.Set(upstreamAppHeader, "cli_fixture_a")
	}
	return h
}

func authenticatedRequestContext(t *testing.T, c Config, h http.Header) (context.Context, requestContext) {
	t.Helper()
	state, err := c.authenticateRequest(&http.Request{Header: h})
	if err != nil {
		t.Fatal(err)
	}
	return withRequestInfo(context.Background(), state), state
}

func requireFaultKind(t *testing.T, err error, kind string) {
	t.Helper()
	var f *Fault
	if !errors.As(err, &f) || f.Kind != kind {
		t.Fatalf("error = %v; want Fault kind %q", err, kind)
	}
}

func TestRequestCredentialConfigurationModes(t *testing.T) {
	c := testConfig(t)
	if c.Apps[0].CredentialMode != "managed" || c.Apps[0].secret == "" || c.Apps[0].DPoPMode != "preferred" {
		t.Fatal("legacy config no longer defaults to managed application credentials")
	}
	if c.Clients[0].CallerAdapter.Kind != "qoderwake" {
		t.Fatal("legacy wake_actor no longer selects the Wake adapter")
	}
	_, state := authenticatedRequestContext(t, c, requestHeaders(""))
	if state.Principal.Actor != "feishu:user_id:alice" || state.Principal.Owner() != actor("alice").Owner() {
		t.Fatal("legacy Wake identity or persisted owner changed")
	}
	c = requestBearerConfig(t)
	t.Setenv("TEST_FEISHU_SECRET", "")
	if err := c.Validate(); err != nil {
		t.Fatalf("request credentials still require a managed secret: %v", err)
	}
	for _, app := range c.Apps {
		if app.secret != "" || app.SecretEnv != "" || app.DPoPMode != "disabled" {
			t.Fatal("request mode retained a managed secret or proof mode")
		}
	}
	for _, tc := range []struct {
		name string
		edit func(*App)
	}{
		{"secret_env", func(a *App) { a.SecretEnv = "REQUEST_SHOULD_NOT_READ_THIS" }},
		{"required proof", func(a *App) { a.DPoPMode = "required" }},
		{"preferred proof", func(a *App) { a.DPoPMode = "preferred" }},
		{"events", func(a *App) { a.EventTypes = []string{"im.message.receive_v1"} }},
		{"event credentials", func(a *App) { a.EventEncryptKeyEnv = "EVENT_KEY" }},
		{"unknown mode", func(a *App) { a.CredentialMode = "request_anything" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := cloneConfig(c)
			tc.edit(&next.Apps[0])
			if err := next.Validate(); err == nil {
				t.Fatal("invalid credential configuration accepted")
			}
		})
	}
}

func TestRequestCredentialHeaders(t *testing.T) {
	c := requestBearerConfig(t)
	for _, tc := range []struct {
		name string
		edit func(http.Header)
		kind string
	}{
		{"valid", func(http.Header) {}, ""},
		{"missing token", func(h http.Header) { h.Del(upstreamTokenHeader) }, "upstream_credential_invalid"},
		{"missing app", func(h http.Header) { h.Del(upstreamAppHeader) }, "upstream_credential_invalid"},
		{"wrong app", func(h http.Header) { h.Set(upstreamAppHeader, "cli_other") }, "upstream_app_mismatch"},
		{"empty token", func(h http.Header) { h.Set(upstreamTokenHeader, "") }, "upstream_credential_invalid"},
		{"empty app", func(h http.Header) { h.Set(upstreamAppHeader, "") }, "upstream_credential_invalid"},
		{"Bearer prefix", func(h http.Header) { h.Set(upstreamTokenHeader, "Bearer fixture-token") }, "upstream_credential_invalid"},
		{"control token", func(h http.Header) { h.Set(upstreamTokenHeader, "fixture\ntoken") }, "upstream_credential_invalid"},
		{"control app", func(h http.Header) { h.Set(upstreamAppHeader, "cli_fixture_a\r") }, "upstream_credential_invalid"},
		{"long token", func(h http.Header) { h.Set(upstreamTokenHeader, strings.Repeat("x", 8193)) }, "upstream_credential_invalid"},
		{"padding only", func(h http.Header) { h.Set(upstreamTokenHeader, "====") }, "upstream_credential_invalid"},
		{"padding in middle", func(h http.Header) { h.Set(upstreamTokenHeader, "ab=cd") }, "upstream_credential_invalid"},
		{"duplicate token", func(h http.Header) { h.Add(upstreamTokenHeader, "fixture-token") }, "invalid_request_header"},
		{"case variant token", func(h http.Header) { h[strings.ToLower(upstreamTokenHeader)] = []string{"fixture-token"} }, "invalid_request_header"},
		{"duplicate app", func(h http.Header) { h.Add(upstreamAppHeader, "cli_fixture_a") }, "invalid_request_header"},
		{"duplicate gateway auth", func(h http.Header) { h.Add("Authorization", "Bearer fixture-key") }, "unauthenticated"},
		{"upstream token as gateway auth", func(h http.Header) { h.Set("Authorization", "Bearer fixture-token") }, "unauthenticated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := requestHeaders("fixture-token")
			tc.edit(h)
			state, err := c.authenticateRequest(&http.Request{Header: h})
			if tc.kind != "" {
				requireFaultKind(t, err, tc.kind)
				return
			}
			if err != nil || state.credential == nil || state.credential.token != "fixture-token" || state.credential.appID != "cli_fixture_a" {
				t.Fatalf("valid request credential was not bound: %v", err)
			}
		})
	}
	_, state := authenticatedRequestContext(t, c, requestHeaders(""))
	if state.credential != nil {
		t.Fatal("missing upstream credential was synthesized")
	}
	managed := testConfig(t)
	_, err := managed.authenticateRequest(&http.Request{Header: requestHeaders("fixture-token")})
	requireFaultKind(t, err, "credential_mode_mismatch")
}

func TestRequestCallerMetadataDoesNotChangeClientOwner(t *testing.T) {
	c := testConfig(t)
	c.Clients[0].IdentityMode = "client"
	c.Clients[0].CallerAdapter = ingress.Adapter{Kind: "qoderwake"}
	c.Clients[0].Actors = nil
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	h := requestHeaders("")
	h.Set("X-QoderWake-AtWaker-Sender-Name", "Fixture%20Name+A%2BB")
	h.Set("X-QoderWake-AtWaker-Sender-Open-Id", "ou_fixture")
	h.Set("X-QoderWake-AtWaker-Sender-Union-Id", "on_fixture")
	_, state := authenticatedRequestContext(t, c, h)
	want := ingress.Caller{Source: "qoderwake", Channel: "feishu", ID: "alice", IDType: "user_id", Name: "Fixture Name+A+B", OpenID: "ou_fixture", UnionID: "on_fixture"}
	if state.Caller != want || state.Principal.Actor != "" {
		t.Fatalf("caller metadata unexpectedly became actor identity: %+v", state.Caller)
	}
	base := Principal{ClientID: "wake", AppID: "app-a", OrgID: "org-a"}
	if state.Principal.Owner() != base.Owner() || state.Principal.Routing == nil || state.Principal.Routing.Caller.Name != "" {
		t.Fatal("display metadata changed durable owner or persisted display name")
	}
	h.Set("X-QoderWake-AtWaker-Sender-Id", "bob")
	h.Set("X-QoderWake-AtWaker-Sender-Name", "Other%20Name")
	_, other := authenticatedRequestContext(t, c, h)
	if other.Principal.Owner() != state.Principal.Owner() {
		t.Fatal("optional caller metadata changed client-mode ownership")
	}
}

func TestRequestGenericActorAndOptionalWhitelist(t *testing.T) {
	c := testConfig(t)
	c.Clients[0].IdentityMode = "actor"
	c.Clients[0].CallerAdapter = ingress.Adapter{Kind: "headers", Headers: ingress.HeaderMapping{Channel: "X-Caller-Channel", ID: "X-Caller-Id", IDType: "member_id", Name: "X-Caller-Name"}}
	c.Clients[0].Actors = nil
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	h := http.Header{"Authorization": {"Bearer fixture-key"}, "X-Caller-Channel": {"custom"}, "X-Caller-Id": {"member-a"}, "X-Caller-Name": {"A%20B"}}
	_, state := authenticatedRequestContext(t, c, h)
	if state.Principal.Actor != "custom:member_id:member-a" || state.Caller.Name != "A B" {
		t.Fatal("generic caller adapter not applied")
	}
	if _, _, err := c.checkPrincipal(state.Principal, true); err != nil {
		t.Fatalf("actor mode incorrectly requires a whitelist: %v", err)
	}
	_, absent := authenticatedRequestContext(t, c, http.Header{"Authorization": {"Bearer fixture-key"}})
	if _, _, err := c.checkPrincipal(absent.Principal, false); err != nil {
		t.Fatal("actor metadata blocked discovery", err)
	}
	_, _, err := c.checkPrincipal(absent.Principal, true)
	requireFaultKind(t, err, "actor_required")
	c.Clients[0].Actors = map[string]ActorPolicy{"custom:member_id:member-b": {}}
	_, err = c.authenticateRequest(&http.Request{Header: h})
	requireFaultKind(t, err, "actor_denied")
	c.Clients[0].Actors[state.Principal.Actor] = ActorPolicy{}
	if _, err = c.authenticateRequest(&http.Request{Header: h}); err != nil {
		t.Fatal("listed actor rejected", err)
	}
}

func requestRoutingConfig(t *testing.T) Config {
	t.Helper()
	c := requestBearerConfig(t)
	c.Clients[0].IdentityMode = "client"
	c.Clients[0].ResourceMode = "application_scope"
	c.Clients[0].Actors = nil
	c.Clients[0].Routes = []ingress.Route{
		{ID: "general", Priority: 0, AppConnectionID: "app-a"},
		{ID: "feishu-route", Priority: 5, Match: ingress.Match{Channels: []string{"feishu"}}, AppConnectionID: "app-b"},
		{ID: "bob-route", Priority: 10, Match: ingress.Match{IDs: []string{"bob"}}, AppConnectionID: "app-a"},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRequestRoutingAndExplicitSelection(t *testing.T) {
	c := requestRoutingConfig(t)
	_, state := authenticatedRequestContext(t, c, requestHeaders(""))
	if state.RouteID != "feishu-route" || state.Principal.AppID != "app-b" || state.Principal.OrgID != "org-b" {
		t.Fatal("highest matching route did not choose its application")
	}
	h := requestHeaders("fixture-token")
	h.Set(upstreamAppHeader, "cli_fixture_b")
	_, state = authenticatedRequestContext(t, c, h)
	if state.credential.appConnectionID != "app-b" || state.credential.brand != "lark" {
		t.Fatal("request token was not bound to the selected application")
	}
	h = requestHeaders("")
	h.Set(routeHeader, "general")
	_, explicit := authenticatedRequestContext(t, c, h)
	if explicit.RouteID != "general" || explicit.Principal.AppID != "app-a" {
		t.Fatal("explicit route did not override automatic priority")
	}
	for _, tc := range []struct{ id, kind string }{{"unknown", "route_not_found"}, {"bob-route", "route_denied"}} {
		h.Set(routeHeader, tc.id)
		_, err := c.authenticateRequest(&http.Request{Header: h})
		requireFaultKind(t, err, tc.kind)
	}
	h.Set(routeHeader, "general")
	h.Add(routeHeader, "feishu-route")
	_, err := c.authenticateRequest(&http.Request{Header: h})
	requireFaultKind(t, err, "invalid_request_header")
	c.Clients[0].Routes = append(c.Clients[0].Routes, ingress.Route{ID: "tie", Priority: 5, AppConnectionID: "app-a"})
	_, err = c.authenticateRequest(&http.Request{Header: requestHeaders("")})
	requireFaultKind(t, err, "route_ambiguous")
}

func TestRequestRouteReloadRevokesOldPrincipal(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		name := "automatic"
		if explicit {
			name = "explicit"
		}
		t.Run(name, func(t *testing.T) {
			c := requestRoutingConfig(t)
			s := startService(t, c, &fakeExecutor{})
			h := requestHeaders("")
			if explicit {
				h.Set(routeHeader, "feishu-route")
			}
			_, state := authenticatedRequestContext(t, c, h)
			if _, _, err := s.currentConfig().checkPrincipal(state.Principal, false); err != nil {
				t.Fatal(err)
			}
			next := cloneConfig(c)
			next.Clients[0].Routes = next.Clients[0].Routes[:1]
			if err := s.Reload(next); err != nil {
				t.Fatal(err)
			}
			_, _, err := s.currentConfig().checkPrincipal(state.Principal, false)
			requireFaultKind(t, err, "access_revoked")
		})
	}
}
