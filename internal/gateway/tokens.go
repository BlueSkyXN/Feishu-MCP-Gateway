package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 应用 Token 和证明密钥只在常驻服务内存中保存，不进入 CLI 子进程。
type appToken struct {
	value, kind string
	key         *ecdsa.PrivateKey
	offset      time.Duration
	refreshAt   time.Time
}
type tokenSlot struct {
	mu      sync.Mutex
	token   *appToken
	refresh chan struct{}
	lastErr error
	retryAt time.Time
	issued  uint64
}
type TokenManager struct {
	mu     sync.Mutex
	slots  map[string]*tokenSlot
	client *http.Client
}

func NewTokenManager(client *http.Client) *TokenManager {
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		client = &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &TokenManager{slots: map[string]*tokenSlot{}, client: client}
}
func tokenKey(a App) string {
	return digest(encode([]string{a.ID, a.OrgID, a.Brand, a.AppID, a.secret, a.DPoPMode}))
}
func (t *TokenManager) Get(ctx context.Context, a App) (*appToken, error) {
	k := tokenKey(a)
	t.mu.Lock()
	slot := t.slots[k]
	if slot == nil {
		slot = &tokenSlot{}
		t.slots[k] = slot
	}
	t.mu.Unlock()
	for {
		slot.mu.Lock()
		if slot.token != nil && time.Now().Before(slot.token.refreshAt) {
			tok := slot.token
			slot.mu.Unlock()
			return tok, nil
		}
		if slot.lastErr != nil && time.Now().Before(slot.retryAt) {
			err := slot.lastErr
			slot.mu.Unlock()
			return nil, err
		}
		if slot.refresh != nil {
			wait := slot.refresh
			slot.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-wait:
				continue
			}
		}
		wait := make(chan struct{})
		slot.refresh = wait
		slot.mu.Unlock()
		tok, err := t.issue(ctx, a)
		slot.mu.Lock()
		slot.token = tok
		slot.lastErr = err
		if err != nil {
			delay := time.Second
			if f, ok := err.(*Fault); ok && f.RetryAfter != nil {
				delay = max(delay, time.Duration(*f.RetryAfter*float64(time.Second)))
			}
			slot.retryAt = time.Now().Add(delay)
		} else {
			slot.issued++
		}
		slot.refresh = nil
		close(wait)
		slot.mu.Unlock()
		return tok, err
	}
}
func (t *TokenManager) Invalidate(a App, tok *appToken) {
	t.mu.Lock()
	slot := t.slots[tokenKey(a)]
	t.mu.Unlock()
	if slot != nil {
		slot.mu.Lock()
		if slot.token == tok {
			slot.token = nil
		}
		slot.mu.Unlock()
	}
}
func (t *TokenManager) Stats() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	var cached, issued int
	for _, s := range t.slots {
		s.mu.Lock()
		if s.token != nil && time.Now().Before(s.token.refreshAt) {
			cached++
		}
		issued += int(s.issued)
		s.mu.Unlock()
	}
	return map[string]any{"cached_app_tokens": cached, "successful_issuances": issued}
}

func accountsOrigin(brand string) string {
	if brand == "lark" {
		return "https://accounts.larksuite.com"
	}
	return "https://accounts.feishu.cn"
}
func platformHost(brand string) string {
	if brand == "lark" {
		return "open.larksuite.com"
	}
	return "open.feishu.cn"
}
func mcpHost(brand string) string {
	if brand == "lark" {
		return "mcp.larksuite.com"
	}
	return "mcp.feishu.cn"
}

func (t *TokenManager) issue(ctx context.Context, a App) (*appToken, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	mode := a.DPoPMode
	if mode == "" {
		mode = "preferred"
	}
	var key *ecdsa.PrivateKey
	var offset time.Duration
	var err error
	if mode != "disabled" {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fault("proof_key_unavailable", "无法生成应用 Token 证明密钥")
		}
		offset = t.clockOffset(ctx, a)
	}
	for attempt := 0; attempt < 4; attempt++ {
		endpoint := accountsOrigin(a.Brand) + "/oauth/v3/token"
		form := url.Values{"grant_type": {"client_credentials"}, "client_id": {a.AppID}, "client_secret": {a.secret}}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if key != nil {
			proof, err := signDPoP(key, http.MethodPost, endpoint, offset)
			if err != nil {
				return nil, err
			}
			request.Header.Set("DPoP", proof)
			request.GetBody = nil
		}
		response, err := t.client.Do(request)
		received := time.Now()
		if err != nil {
			return nil, fault("token_transport", "应用取票请求失败")
		}
		raw, err := readBounded(response.Body, 1<<20)
		response.Body.Close()
		if err != nil {
			return nil, fault("token_response", "应用取票响应不可读取")
		}
		var result struct {
			Code        int    `json:"code"`
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
			Error       string `json:"error"`
			TokenType   string `json:"token_type"`
		}
		if err = json.Unmarshal(raw, &result); err != nil {
			return nil, fault("token_response", "应用取票响应格式无效")
		}
		if response.StatusCode == 429 {
			f := fault("token_rate_limit", "应用取票被限流，请等待后重试")
			if n, e := strconv.ParseFloat(response.Header.Get("Retry-After"), 64); e == nil && n >= 0 && n <= 86400 {
				f.RetryAfter = &n
			}
			return nil, f
		}
		if key != nil && result.Error == "invalid_dpop_proof" && result.Code == 20153 && attempt == 0 {
			if serverTime, e := http.ParseTime(response.Header.Get("Date")); e == nil {
				offset = serverTime.Sub(received)
				continue
			}
		}
		if key != nil && result.Error == "invalid_dpop_proof" && attempt < 2 {
			continue
		}
		if key != nil && mode == "preferred" && result.Error == "invalid_dpop_proof" && attempt == 2 {
			key = nil
			continue
		}
		if response.StatusCode != 200 || result.Error != "" || result.Code != 0 || result.AccessToken == "" {
			f := fault("application_token_failed", "应用取票失败，请核对应用凭据和应用配置")
			if result.Code != 0 {
				f.UpstreamCode = &result.Code
			}
			return nil, f
		}
		if result.ExpiresIn <= 0 || result.ExpiresIn > 30*86400 {
			return nil, fault("token_response", "应用 Token 有效期无效")
		}
		kind := result.TokenType
		if kind == "" {
			kind = "Bearer"
		}
		if !strings.EqualFold(kind, "Bearer") && !strings.EqualFold(kind, "DPoP") {
			return nil, fault("token_type_unsupported", "应用取票返回了不支持的 Token 类型")
		}
		if strings.EqualFold(kind, "DPoP") && key == nil {
			return nil, fault("proof_binding_missing", "Token 要求证明密钥，但此次取票没有建立绑定")
		}
		if mode == "required" && !strings.EqualFold(kind, "DPoP") {
			return nil, fault("proof_required", "应用配置要求 DPoP，但上游未返回绑定 Token")
		}
		if strings.EqualFold(kind, "Bearer") {
			kind = "Bearer"
			key = nil
		} else {
			kind = "DPoP"
		}
		ttl := time.Duration(result.ExpiresIn) * time.Second
		return &appToken{value: result.AccessToken, kind: kind, key: key, offset: offset, refreshAt: time.Now().Add(ttl - min(ttl/10, 60*time.Second))}, nil
	}
	return nil, fault("application_token_failed", "应用证明校验未通过")
}
func (t *TokenManager) clockOffset(ctx context.Context, a App) time.Duration {
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, accountsOrigin(a.Brand)+"/accounts/status/heartbeat", nil)
	response, err := t.client.Do(request)
	if err != nil {
		return 0
	}
	received := time.Now()
	defer response.Body.Close()
	raw, err := readBounded(response.Body, 1<<20)
	if err != nil {
		return 0
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Now string `json:"now"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &body) != nil || body.Code != 0 {
		return 0
	}
	n, err := strconv.ParseInt(body.Data.Now, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return time.Unix(n, 0).Sub(received)
}

func signDPoP(key *ecdsa.PrivateKey, method, target string, offset time.Duration) (string, error) {
	htu, err := NormalizeHTU(target)
	if err != nil {
		return "", err
	}
	b64 := base64.RawURLEncoding.EncodeToString
	header := map[string]any{"typ": "dpop+jwt", "alg": "ES256", "jwk": map[string]string{"kty": "EC", "crv": "P-256", "x": b64(key.X.FillBytes(make([]byte, 32))), "y": b64(key.Y.FillBytes(make([]byte, 32)))}}
	claims := map[string]any{"jti": newID(""), "htm": strings.ToUpper(method), "htu": htu, "iat": time.Now().Add(offset).Unix()}
	payload := b64(encode(header)) + "." + b64(encode(claims))
	hash := sha256.Sum256([]byte(payload))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return "", err
	}
	signature := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	return payload + "." + b64(signature), nil
}

func (t *TokenManager) Retain(apps []App) {
	keep := map[string]bool{}
	for _, a := range apps {
		if !a.Disabled {
			keep[tokenKey(a)] = true
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for k := range t.slots {
		if !keep[k] {
			delete(t.slots, k)
		}
	}
}
