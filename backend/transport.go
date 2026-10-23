// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/extension/transport"
)

// 覆盖官方 sidecar interceptor，把无 Token 的预签名传输也交给 Gateway 检查。
type gatewayTransport struct{}

func (gatewayTransport) Name() string { return "feishu-gateway" }
func (gatewayTransport) ResolveInterceptor(context.Context) transport.Interceptor {
	return &gatewayInterceptor{address: os.Getenv("LARKSUITE_CLI_AUTH_PROXY"), key: os.Getenv("LARKSUITE_CLI_PROXY_KEY")}
}

type gatewayInterceptor struct{ address, key string }

func (*gatewayInterceptor) PreRoundTrip(*http.Request) func(*http.Response, error) { return nil }
func (g *gatewayInterceptor) PreRoundTripE(req *http.Request) (func(*http.Response, error), error) {
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("backend permits only HTTPS upstream targets")
	}
	authHeader, identity := "", "external"
	if value := req.Header.Get("Authorization"); value != "" {
		if value != "Bearer sidecar-managed-tat" {
			return nil, fmt.Errorf("backend rejects non-application credentials")
		}
		authHeader = "Authorization"
		identity = "bot"
	}
	if req.Header.Get("X-Lark-MCP-UAT") != "" {
		return nil, fmt.Errorf("user identity is unavailable")
	}
	if value := req.Header.Get("X-Lark-MCP-TAT"); value != "" {
		if value != "sidecar-managed-tat" {
			return nil, fmt.Errorf("backend rejects raw tokens")
		}
		authHeader = "X-Lark-MCP-TAT"
		identity = "bot"
	}
	var body []byte
	var err error
	if req.Body != nil {
		limit := int64(258 << 20)
		if n, e := strconv.ParseInt(os.Getenv("FEISHU_GATEWAY_FILE_LIMIT"), 10, 64); e == nil && n > 0 {
			limit = n + (2 << 20)
		}
		body, err = io.ReadAll(io.LimitReader(req.Body, limit+1))
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		if int64(len(body)) > limit {
			return nil, fmt.Errorf("outbound body exceeds invocation limit")
		}
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = nil
	host, query := req.URL.Host, req.URL.RequestURI()
	sum := sha256.Sum256(body)
	bodySHA := hex.EncodeToString(sum[:])
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := rand.Text()
	contentType := req.Header.Get("Content-Type")
	canonical := strings.Join([]string{"gateway-v1", req.Method, host, query, bodySHA, timestamp, identity, authHeader, nonce, contentType, req.Header.Get("Range")}, "\n")
	mac := hmac.New(sha256.New, []byte(g.key))
	mac.Write([]byte(canonical))
	req.Header.Del("Authorization")
	req.Header.Del("DPoP")
	req.Header.Del("X-Lark-MCP-TAT")
	req.Header.Del("Cookie")
	req.Header.Set("X-Lark-Proxy-Version", "gateway-v1")
	req.Header.Set("X-Lark-Proxy-Target", "https://"+host)
	req.Header.Set("X-Lark-Proxy-Identity", identity)
	req.Header.Set("X-Lark-Proxy-Auth-Header", authHeader)
	req.Header.Set("X-Lark-Proxy-Timestamp", timestamp)
	req.Header.Set("X-Lark-Body-SHA256", bodySHA)
	req.Header.Set("X-Lark-Proxy-Signature", hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-Feishu-Gateway-Request-Id", nonce)
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(g.address, "http://")
	req.Host = ""
	return nil, nil
}
