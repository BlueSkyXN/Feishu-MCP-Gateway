// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Broker struct {
	paceMu                sync.Mutex
	nextRequest, cooldown map[string]time.Time
	tokens                *TokenManager
	client                *http.Client
	limit                 int64
	authorize             func(Invocation) (App, error)
}
type replayReply struct {
	ready  chan struct{}
	digest string
	status int
	body   []byte
	header http.Header
}
type proxyGrant struct {
	broker          *Broker
	inv             Invocation
	key             string
	ctx             context.Context
	mu              sync.Mutex
	replies         map[string]*replayReply
	external        map[string]bool
	inputURLs       map[string]bool
	requests        chan struct{}
	lastError       *Fault
	writeStarted    bool
	confirmedWrites int
	replyBytes      int64
}
type ProxySession struct {
	URL, Key string
	server   *http.Server
	cancel   context.CancelFunc
	grant    *proxyGrant
}

func NewBroker(limit int64, client *http.Client, authorize func(Invocation) (App, error)) *Broker {
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.DialContext = publicDial
		client = &http.Client{Transport: transport, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Broker{nextRequest: map[string]time.Time{}, cooldown: map[string]time.Time{}, tokens: NewTokenManager(client), client: client, limit: limit, authorize: authorize}
}
func publicDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if !ip.IP.IsGlobalUnicast() || ip.IP.IsPrivate() || ip.IP.IsLoopback() || ip.IP.IsLinkLocalUnicast() {
			continue
		}
		conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("target has no reachable public address")
}
func (b *Broker) Open(ctx context.Context, in Invocation) (*ProxySession, error) {
	if _, err := b.authorize(in); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	grantCtx, cancel := context.WithCancel(ctx)
	g := &proxyGrant{broker: b, inv: in, key: newID("grant_"), ctx: grantCtx, replies: map[string]*replayReply{}, external: map[string]bool{}, inputURLs: map[string]bool{}, requests: make(chan struct{}, 4)}
	for _, u := range append(explicitURLs(in.Arguments), in.ExternalURLs...) {
		g.inputURLs[u] = true
	}
	server := &http.Server{Handler: g, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 120 * time.Second, MaxHeaderBytes: 32 << 10}
	go func() { _ = server.Serve(listener) }()
	return &ProxySession{URL: "http://" + listener.Addr().String(), Key: g.key, server: server, cancel: cancel, grant: g}, nil
}
func (s *ProxySession) Outcome() (*Fault, bool) {
	s.grant.mu.Lock()
	defer s.grant.mu.Unlock()
	return s.grant.lastError, s.grant.writeStarted
}
func (g *proxyGrant) reject(w http.ResponseWriter, message string, code int) {
	g.mu.Lock()
	g.lastError = fault("egress_denied", message)
	g.mu.Unlock()
	http.Error(w, message, code)
}
func (s *ProxySession) Close() { s.cancel(); _ = s.server.Close() }

func (g *proxyGrant) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case g.requests <- struct{}{}:
		defer func() { <-g.requests }()
	case <-r.Context().Done():
		return
	case <-g.ctx.Done():
		return
	}
	if g.ctx.Err() != nil {
		g.reject(w, "invocation ended", http.StatusForbidden)
		return
	}
	app, err := g.broker.authorize(g.inv)
	if err != nil {
		g.reject(w, "invocation authorization revoked", http.StatusForbidden)
		return
	}
	header := r.Header
	version := header.Get("X-Lark-Proxy-Version")
	identity := header.Get("X-Lark-Proxy-Identity")
	authHeader := header.Get("X-Lark-Proxy-Auth-Header")
	nonce := header.Get("X-Feishu-Gateway-Request-Id")
	if version != "gateway-v1" || nonce == "" || len(nonce) > 128 || (identity != "bot" && identity != "external") || (identity == "bot" && authHeader != "Authorization" && authHeader != "X-Lark-MCP-TAT") || (identity == "external" && authHeader != "") {
		g.reject(w, "unsupported proxy identity or contract", http.StatusForbidden)
		return
	}
	target, err := url.Parse(header.Get("X-Lark-Proxy-Target"))
	if err != nil || target.Scheme != "https" || target.Host == "" || target.User != nil || target.Path != "" || target.RawQuery != "" || target.Fragment != "" || target.Port() != "" {
		g.reject(w, "invalid upstream target", http.StatusForbidden)
		return
	}
	timestamp := header.Get("X-Lark-Proxy-Timestamp")
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || seconds < time.Now().Unix()-60 || seconds > time.Now().Unix()+60 {
		g.reject(w, "expired proxy signature", http.StatusUnauthorized)
		return
	}
	body, err := readBounded(r.Body, g.broker.limit+(2<<20))
	if err != nil {
		g.reject(w, "request exceeds limit", http.StatusRequestEntityTooLarge)
		return
	}
	bodySHA := digest(body)
	canonical := strings.Join([]string{version, r.Method, target.Host, r.URL.RequestURI(), bodySHA, timestamp, identity, authHeader, nonce, r.Header.Get("Content-Type"), r.Header.Get("Range")}, "\n")
	mac := hmac.New(sha256.New, []byte(g.key))
	mac.Write([]byte(canonical))
	signature, err := hex.DecodeString(header.Get("X-Lark-Proxy-Signature"))
	if err != nil || header.Get("X-Lark-Body-SHA256") != bodySHA || !hmac.Equal(signature, mac.Sum(nil)) {
		g.reject(w, "invalid proxy signature", http.StatusUnauthorized)
		return
	}
	upstream := target.String() + r.URL.RequestURI()
	if identity == "bot" {
		if target.Host != platformHost(app.Brand) && target.Host != mcpHost(app.Brand) {
			g.reject(w, "application target denied", http.StatusForbidden)
			return
		}
		if err = g.allowRequest(r, body, app.resourceScoped); err != nil {
			g.reject(w, err.Error(), http.StatusForbidden)
			return
		}
	} else {
		g.mu.Lock()
		allowed := g.external[upstream] || ((r.Method == "GET" || r.Method == "HEAD") && g.inputURLs[upstream])
		g.mu.Unlock()
		if !allowed {
			g.reject(w, "external URL was not issued by an authorized response", http.StatusForbidden)
			return
		}
	}
	requestHash := digest([]byte(canonical))
	g.mu.Lock()
	reply, exists := g.replies[nonce]
	if !exists {
		if len(g.replies) >= 2048 || g.replyBytes >= min(64<<20, g.broker.limit*4) {
			g.mu.Unlock()
			g.reject(w, "invocation request limit reached", http.StatusTooManyRequests)
			return
		}
		reply = &replayReply{ready: make(chan struct{}), digest: requestHash, header: http.Header{}}
		g.replies[nonce] = reply
	}
	g.mu.Unlock()
	if exists {
		if reply.digest != requestHash {
			g.reject(w, "nonce reused with different request", http.StatusConflict)
			return
		}
		select {
		case <-reply.ready:
			copyReply(w, reply)
		case <-r.Context().Done():
		}
		return
	}
	defer close(reply.ready)
	response, err := g.forward(r.Context(), app, identity, authHeader, upstream, r.Method, r.Header, body)
	if err != nil {
		g.mu.Lock()
		var f *Fault
		if errors.As(err, &f) {
			g.lastError = f
		} else {
			g.lastError = fault("upstream_transport", "出站请求中断，请核对调用回执")
		}
		g.mu.Unlock()
		reply.status = http.StatusBadGateway
		reply.body = []byte("upstream request failed; reconcile writes before retry")
		copyReply(w, reply)
		return
	}
	defer response.Body.Close()
	raw, err := readBounded(response.Body, g.broker.limit)
	if err != nil {
		g.mu.Lock()
		var f *Fault
		if errors.As(err, &f) {
			g.lastError = f
		} else {
			g.lastError = fault("upstream_transport", "出站请求中断，请核对调用回执")
		}
		g.mu.Unlock()
		reply.status = http.StatusBadGateway
		reply.body = []byte("upstream response exceeds limit or was interrupted")
		copyReply(w, reply)
		return
	}
	g.mu.Lock()
	g.lastError = nil
	if g.inv.Operation.Risk != "read" && r.Method != "GET" && r.Method != "HEAD" && response.StatusCode >= 200 && response.StatusCode < 300 {
		var envelope struct {
			Code *int `json:"code"`
		}
		if json.Unmarshal(raw, &envelope) == nil && envelope.Code != nil && *envelope.Code == 0 {
			g.confirmedWrites++
		}
	}
	g.mu.Unlock()
	reply.status = response.StatusCode
	reply.body = raw
	g.mu.Lock()
	g.replyBytes += int64(len(raw))
	g.mu.Unlock()
	for _, name := range []string{"Content-Type", "Retry-After", "Content-Disposition", "X-Tt-Logid", "X-Request-Id"} {
		if v := response.Header.Get(name); v != "" {
			reply.header.Set(name, v)
		}
	}
	if identity == "bot" && strings.Contains(response.Header.Get("Content-Type"), "json") {
		var value any
		if json.Unmarshal(raw, &value) == nil {
			g.rememberURLs(value)
		}
	}
	if location := response.Header.Get("Location"); location != "" {
		u, e := url.Parse(location)
		if e == nil && u.Scheme == "https" && u.User == nil && u.Host != "" {
			g.mu.Lock()
			g.external[location] = true
			g.mu.Unlock()
			reply.header.Set("Location", location)
		}
	}
	copyReply(w, reply)
}
func copyReply(w http.ResponseWriter, r *replayReply) {
	for name, values := range r.header {
		if name == "X-Request-Digest" {
			continue
		}
		for _, v := range values {
			w.Header().Add(name, v)
		}
	}
	w.WriteHeader(r.status)
	_, _ = w.Write(r.body)
}

func (g *proxyGrant) forward(ctx context.Context, a App, identity, authHeader, target, method string, headers http.Header, body []byte) (*http.Response, error) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(g.ctx, cancel)
	cleanup := func() { stop(); cancel() }
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()

	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.GetBody = nil
	for _, name := range []string{"Content-Type", "Accept", "Range", "User-Agent"} {
		if value := headers.Get(name); value != "" {
			request.Header.Set(name, value)
		}
	}
	var token *appToken
	if identity == "bot" {
		token, err = g.broker.tokens.Get(ctx, a)
		if err != nil {
			return nil, err
		}
		if authHeader == "X-Lark-MCP-TAT" {
			if token.kind == "DPoP" {
				return nil, fault("proof_header_unsupported", "该上游协议不接受 DPoP 自定义认证头")
			}
			request.Header.Set(authHeader, token.value)
		} else {
			request.Header.Set("Authorization", token.kind+" "+token.value)
			if token.kind == "DPoP" {
				proof, err := signDPoP(token.key, method, target, token.offset)
				if err != nil {
					return nil, err
				}
				request.Header.Set("DPoP", proof)
			}
		}
	}

	if err = g.broker.pace(ctx, a); err != nil {
		return nil, err
	}
	current, err := g.broker.authorize(g.inv)
	if err != nil {
		return nil, err
	}
	if tokenKey(current) != tokenKey(a) {
		return nil, fault("access_revoked", "应用凭据或身份绑定已改变，请重新发起调用")
	}
	if method != "GET" && method != "HEAD" && g.inv.Operation.Risk != "read" {
		g.mu.Lock()
		g.writeStarted = true
		g.mu.Unlock()
	}
	response, err := g.broker.client.Do(request)
	if err == nil {
		g.broker.rememberRateLimit(a, response)
	}
	if err == nil && response.StatusCode == 401 && token != nil {
		g.broker.tokens.Invalidate(a, token)
	}
	if err == nil {
		response.Body = &cleanupBody{ReadCloser: response.Body, cleanup: cleanup}
		success = true
	}
	return response, err
}

type cleanupBody struct {
	io.ReadCloser
	cleanup func()
}

func (b *cleanupBody) Close() error { defer b.cleanup(); return b.ReadCloser.Close() }

func (g *proxyGrant) rememberURLs(v any) {
	switch value := v.(type) {
	case string:
		if strings.HasPrefix(value, "https://") {
			u, err := url.Parse(value)
			if err == nil && u.Host != "" && u.User == nil && len(value) <= 16384 {
				g.mu.Lock()
				if len(g.external) < 2048 {
					g.external[value] = true
				}
				g.mu.Unlock()
			}
		}
	case map[string]any:
		for _, child := range value {
			g.rememberURLs(child)
		}
	case []any:
		for _, child := range value {
			g.rememberURLs(child)
		}
	}
}

func (g *proxyGrant) allowRequest(r *http.Request, body []byte, scoped bool) error {
	op := g.inv.Operation
	if path.Clean(r.URL.Path) != r.URL.Path || strings.Contains(r.URL.Path, "\\") {
		return fmt.Errorf("non-canonical upstream path")
	}
	if op.Kind != "shortcut" {
		if err := g.checkGenericInput(r, body); err != nil {
			return err
		}
		expected := op.Path
		for name, value := range object(g.inv.Arguments["params"]) {
			placeholder := "{" + name + "}"
			if strings.Contains(expected, placeholder) {
				text := fmt.Sprint(value)
				if strings.ContainsAny(text, "/\\") {
					return fmt.Errorf("invalid path parameter")
				}
				expected = strings.ReplaceAll(expected, placeholder, text)
			}
		}
		if r.Method != op.Method || r.URL.Path != expected {
			return fmt.Errorf("request does not match registered operation")
		}
		if value, ok := g.inv.Arguments["data"]; ok && len(op.FileFields) == 0 {
			if !strings.Contains(r.Header.Get("Content-Type"), "json") {
				return fault("egress_arguments", "上游请求内容类型不符合已授权参数")
			}
			var actual any
			if json.Unmarshal(body, &actual) != nil || !bytes.Equal(encode(value), encode(actual)) {
				return fmt.Errorf("request body differs from authorized arguments")
			}
		}
		if _, ok := g.inv.Arguments["data"]; !ok && len(op.FileFields) == 0 && len(bytes.TrimSpace(body)) > 0 {
			return fault("egress_arguments", "此次操作未授权请求体")
		}
		return nil
	}
	allowed := false
	for _, prefix := range op.EgressPrefixes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("shortcut endpoint outside declared domains")
	}
	if op.Risk == "read" && r.Method != "GET" && r.Method != "HEAD" {
		name := path.Base(r.URL.Path)
		allowedRead := r.Method == "POST" && (strings.Contains(name, "search") || strings.Contains(name, "query") || strings.Contains(name, "fetch") || strings.Contains(name, "list") || strings.HasPrefix(name, "batch_get") || name == "export_tasks" || name == "render" || name == "preview" || name == "download" || name == "export" || name == "filter" || name == "batch_query" || name == "batch_get" || name == "query_statistic" || name == "invoke_read" || name == "get_attachments" || name == "basic_batch" || name == "read_status" || name == "preview_result" || name == "mget_instance_relation_info" || r.URL.Path == "/open-apis/calendar/v4/calendars/primary" || r.URL.Path == "/open-apis/base/v3/bases/tables/forms/detail")
		if !allowedRead {
			return fmt.Errorf("read operation cannot perform this upstream mutation")
		}
	}
	if scoped {
		for field, kind := range op.ResourceBindings {
			value, err := boundResource(g.inv.Arguments, field, kind)
			if err != nil {
				return err
			}
			value = resourceToken(value)
			segments := map[string][]string{"document": {"documents"}, "base": {"bases", "apps"}, "spreadsheet": {"spreadsheets"}, "chat": {"chats"}, "wiki_space": {"spaces"}}[kind]
			matched := false
			for _, segment := range segments {
				anchor := "/" + segment + "/" + value
				if strings.Contains(r.URL.Path, anchor+"/") || strings.HasSuffix(r.URL.Path, anchor) {
					matched = true
				}
			}
			if !matched {
				return fault("resource_egress_denied", "此快捷操作的子请求无法验证资源范围，请使用具备该范围的操作")
			}
		}
	}
	return nil
}

func (s *ProxySession) HasConfirmedWrite() bool {
	s.grant.mu.Lock()
	defer s.grant.mu.Unlock()
	return s.grant.confirmedWrites > 0
}
