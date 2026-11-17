// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/BlueSkyXN/Feishu-MCP-Gateway/internal/ingress"
)

const (
	upstreamTokenHeader = "X-Feishu-Access-Token"
	upstreamAppHeader   = "X-Feishu-App-Id"
	routeHeader         = "X-Feishu-Gateway-Route"
)

// requestCredential never belongs to Config, Principal, Invocation.Arguments or
// SQLite. It is held only by the originating request and its synchronous work.
type requestCredential struct {
	appConnectionID string
	appID           string
	brand           string
	token           string
}

type requestContext struct {
	Principal      Principal
	Caller         ingress.Caller
	RouteID        string
	CredentialMode string
	credential     *requestCredential
}
type requestContextKey struct{}

func requestInfo(ctx context.Context) requestContext {
	v, _ := ctx.Value(requestContextKey{}).(requestContext)
	return v
}

func withRequestInfo(ctx context.Context, v requestContext) context.Context {
	return context.WithValue(ctx, requestContextKey{}, v)
}

func singleRequestHeader(h http.Header, name string) (string, bool, error) {
	var values []string
	for key, vv := range h {
		if strings.EqualFold(key, name) {
			values = append(values, vv...)
		}
	}
	if len(values) == 0 {
		return "", false, nil
	}
	if len(values) != 1 {
		return "", true, fault("invalid_request_header", "请求头不能重复: "+name)
	}
	return values[0], true, nil
}

func validHeaderAtom(value string, max int) bool {
	if value == "" || len(value) > max {
		return false
	}
	for _, ch := range value {
		if ch < 0x21 || ch > 0x7e {
			return false
		}
	}
	return true
}

func validBearerValue(value string) bool {
	if value == "" || len(value) > 8192 {
		return false
	}
	// RFC 6750 b64token characters; accept padding only at the end.
	padding := false
	characters := 0
	for _, ch := range value {
		if ch == '=' {
			padding = true
			continue
		}
		if padding || !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || strings.ContainsRune("-._~+/", ch)) {
			return false
		}
		characters++
	}
	return characters > 0
}

func routeFault(err error) error {
	switch {
	case errors.Is(err, ingress.ErrRouteAmbiguous):
		return fault("route_ambiguous", "多条同优先级路由匹配；请调整规则或显式选择路由")
	case errors.Is(err, ingress.ErrRouteNotFound):
		return fault("route_not_found", "指定路由未在此接入中配置")
	case errors.Is(err, ingress.ErrRouteDenied):
		return fault("route_denied", "指定路由不匹配本次调用者上下文")
	default:
		return fault("route_invalid", "路由配置或选择无效")
	}
}

func (c Config) authenticateRequest(r *http.Request) (requestContext, error) {
	var out requestContext
	auth, present, err := singleRequestHeader(r.Header, "Authorization")
	if err != nil || !present || !strings.HasPrefix(auth, "Bearer ") || len(auth) == len("Bearer ") {
		return out, fault("unauthenticated", "需要唯一的 Bearer 接入凭据")
	}
	hash := sha256.Sum256([]byte(strings.TrimPrefix(auth, "Bearer ")))
	var client Client
	found := false
	for _, candidate := range c.Clients {
		expected, _ := hex.DecodeString(candidate.KeySHA256)
		if !candidate.Disabled && subtle.ConstantTimeCompare(hash[:], expected) == 1 {
			client, found = candidate, true
			break
		}
	}
	if !found {
		return out, fault("unauthenticated", "接入凭据无效或已停用")
	}
	caller, err := client.CallerAdapter.Parse(r.Header)
	if err != nil {
		return out, fault("invalid_caller", "调用者字段缺失、重复或格式无效")
	}
	requested, hasRequested, err := singleRequestHeader(r.Header, routeHeader)
	if err != nil {
		return out, err
	}
	if hasRequested && !validHeaderAtom(requested, 128) {
		return out, fault("route_invalid", "路由 ID 不能为空或包含空白字符")
	}
	selected, err := ingress.Select(client.Routes, caller, requested, client.AppID)
	if err != nil {
		return out, routeFault(err)
	}
	app, ok := c.app(selected.AppConnectionID)
	if !ok {
		return out, fault("application_disabled", "选定的应用连接已停用")
	}
	p := Principal{ClientID: client.ID, AppID: app.ID, OrgID: app.OrgID}
	if caller.ID != "" || len(client.Routes) > 0 || requested != "" {
		stableCaller := caller
		stableCaller.Name = ""
		p.Routing = &RoutingContext{Caller: stableCaller, RequestedRoute: requested}
	}
	if client.IdentityMode == "actor" || client.IdentityMode == "wake_actor" {
		p.Actor = caller.ActorKey()
		if p.Actor != "" && (client.IdentityMode == "wake_actor" || len(client.Actors) > 0) {
			if _, ok := client.Actors[p.Actor]; !ok {
				return out, fault("actor_denied", "调用者未被授予此接入")
			}
		}
	}
	out.Principal, out.Caller, out.RouteID, out.CredentialMode = p, caller, selected.RouteID, app.CredentialMode
	token, tokenPresent, err := singleRequestHeader(r.Header, upstreamTokenHeader)
	if err != nil {
		return requestContext{}, err
	}
	appID, appPresent, err := singleRequestHeader(r.Header, upstreamAppHeader)
	if err != nil {
		return requestContext{}, err
	}
	if tokenPresent || appPresent {
		if app.CredentialMode != "request_bearer" {
			return requestContext{}, fault("credential_mode_mismatch", "当前应用为托管模式，不接受请求级飞书 Token")
		}
		if !tokenPresent || !appPresent || !validBearerValue(token) || !validHeaderAtom(appID, 256) {
			return requestContext{}, fault("upstream_credential_invalid", "需要同时提供原始飞书应用 Bearer Token 与 App ID")
		}
		if appID != app.AppID {
			return requestContext{}, fault("upstream_app_mismatch", "请求声明的 App ID 与选定路由不一致")
		}
		out.credential = &requestCredential{appConnectionID: app.ID, appID: app.AppID, brand: app.Brand, token: token}
	}
	return out, nil
}

func requestAppToken(ctx context.Context, app App) (*appToken, error) {
	cred := requestInfo(ctx).credential
	if cred == nil {
		return nil, fault("upstream_credential_required", "本次飞书操作需要客户端提供应用访问 Token")
	}
	if cred.appConnectionID != app.ID || cred.appID != app.AppID || cred.brand != app.Brand {
		return nil, fault("upstream_app_mismatch", "请求凭据与当前应用绑定不一致")
	}
	return &appToken{value: cred.token, kind: "Bearer"}, nil
}

func checkRequestCredentialMode(ctx context.Context, app App) error {
	info := requestInfo(ctx)
	if info.CredentialMode != "" && info.CredentialMode != app.CredentialMode {
		return fault("access_revoked", "本次请求的应用凭据模式已改变，请重新发起调用")
	}
	if info.credential != nil && app.CredentialMode != "request_bearer" {
		return fault("credential_mode_mismatch", "请求级飞书凭据不能回退到托管取票")
	}
	return nil
}

// ExplainRequest is local diagnostics only: never returns any access token or
// secret, and never connects to Feishu or starts the execution backend.
func (c Config) ExplainRequest(headers http.Header) (map[string]any, error) {
	r := &http.Request{Header: headers}
	state, err := c.authenticateRequest(r)
	if err != nil {
		return nil, err
	}
	_, app, err := c.checkPrincipal(state.Principal, false)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"client_id":                   state.Principal.ClientID,
		"route_id":                    state.RouteID,
		"app_connection_id":           app.ID,
		"app_id":                      app.AppID,
		"brand":                       app.Brand,
		"actor":                       state.Principal.Actor,
		"caller":                      state.Caller,
		"credential_mode":             app.CredentialMode,
		"upstream_credential_present": state.credential != nil,
	}, nil
}
