package gateway

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type App struct {
	ID        string `json:"id"`
	OrgID     string `json:"organization_id"`
	AppID     string `json:"app_id"`
	Brand     string `json:"brand"`
	SecretEnv string `json:"secret_env"`
	Disabled  bool   `json:"disabled,omitempty"`
	secret    string
}

type ActorPolicy struct {
	Operations []string            `json:"operations,omitempty"`
	Resources  map[string][]string `json:"resources,omitempty"`
}

type Client struct {
	ID            string                 `json:"id"`
	AppID         string                 `json:"app_connection_id"`
	KeySHA256     string                 `json:"key_sha256"`
	IdentityMode  string                 `json:"identity_mode"`
	Packs         []string               `json:"packs"`
	Operations    []string               `json:"operations,omitempty"`
	ResourceMode  string                 `json:"resource_mode"`
	Resources     map[string][]string    `json:"resources,omitempty"`
	Actors        map[string]ActorPolicy `json:"actors,omitempty"`
	Disabled      bool                   `json:"disabled,omitempty"`
	MaxConcurrent int                    `json:"max_concurrent,omitempty"`
}

type Config struct {
	MaxPendingJobs  int      `json:"max_pending_jobs_per_client,omitempty"`
	Listen          string   `json:"listen"`
	DataDir         string   `json:"data_dir"`
	CLIBinary       string   `json:"cli_binary"`
	CLISHA256       string   `json:"cli_sha256"`
	TrustedProxy    bool     `json:"trusted_https_proxy,omitempty"`
	Origins         []string `json:"allowed_origins,omitempty"`
	Apps            []App    `json:"apps"`
	Clients         []Client `json:"clients"`
	MaxFileBytes    int64    `json:"max_file_bytes,omitempty"`
	MaxStorageBytes int64    `json:"max_storage_bytes_per_client,omitempty"`
	FileTTL         int64    `json:"file_ttl_seconds,omitempty"`
	MaxInlineBytes  int      `json:"max_inline_bytes,omitempty"`
	TimeoutSeconds  int      `json:"operation_timeout_seconds,omitempty"`
	Workers         int      `json:"workers,omitempty"`
}

func LoadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, err
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return c, errors.New("配置必须只包含一个 JSON 对象")
	}
	if !filepath.IsAbs(c.DataDir) {
		c.DataDir = filepath.Join(filepath.Dir(path), c.DataDir)
	}
	return c, c.Validate()
}

func (c *Config) Validate() error {
	if c.Listen == "" {
		c.Listen = "127.0.0.1:18790"
	}
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if (ip == nil || !ip.IsLoopback()) && !c.TrustedProxy {
		return errors.New("非回环监听需要显式 trusted_https_proxy，并由 HTTPS 反向代理保护")
	}
	if c.DataDir == "" {
		return errors.New("data_dir 不能为空")
	}
	if c.MaxFileBytes == 0 {
		c.MaxFileBytes = 16 << 20
	}
	if c.MaxStorageBytes == 0 {
		c.MaxStorageBytes = 256 << 20
	}
	if c.FileTTL == 0 {
		c.FileTTL = 86400
	}
	if c.MaxInlineBytes == 0 {
		c.MaxInlineBytes = 64 << 10
	}
	if c.TimeoutSeconds == 0 {
		c.TimeoutSeconds = 120
	}
	if c.Workers == 0 {
		c.Workers = 4
	}
	if c.MaxPendingJobs == 0 {
		c.MaxPendingJobs = 100
	}
	if c.MaxPendingJobs < 1 || c.MaxPendingJobs > 10000 {
		return errors.New("后台队列限额必须在 1 与 10000 之间")
	}
	if c.MaxFileBytes < 1 || c.MaxFileBytes > 256<<20 || c.MaxStorageBytes < c.MaxFileBytes || c.MaxInlineBytes < 1024 || c.MaxInlineBytes > 1<<20 || c.FileTTL < 60 || c.TimeoutSeconds < 1 || c.TimeoutSeconds > 3600 || c.Workers < 1 || c.Workers > 32 {
		return errors.New("容量、时限或 worker 配置超出范围")
	}
	apps := map[string]bool{}
	for i := range c.Apps {
		a := &c.Apps[i]
		if a.ID == "" || a.OrgID == "" || a.AppID == "" || apps[a.ID] {
			return errors.New("应用 ID、企业绑定不能为空且连接 ID 不能重复")
		}
		apps[a.ID] = true
		if a.Brand != "feishu" && a.Brand != "lark" {
			return fmt.Errorf("应用 %s 的 brand 必须为 feishu 或 lark", a.ID)
		}
		if !a.Disabled {
			a.secret = os.Getenv(a.SecretEnv)
			if a.secret == "" {
				return fmt.Errorf("应用 %s 缺少配置的 secret_env", a.ID)
			}
		}
	}
	ids, keys := map[string]bool{}, map[string]bool{}
	for i := range c.Clients {
		v := &c.Clients[i]
		key, err := hex.DecodeString(v.KeySHA256)
		if err != nil || len(key) != 32 || v.ID == "" || ids[v.ID] || keys[strings.ToLower(v.KeySHA256)] || !apps[v.AppID] {
			return errors.New("接入 ID、key_sha256 或应用绑定无效或重复")
		}
		ids[v.ID] = true
		keys[strings.ToLower(v.KeySHA256)] = true
		if v.IdentityMode != "client" && v.IdentityMode != "wake_actor" {
			return errors.New("identity_mode 必须为 client 或 wake_actor")
		}
		if v.ResourceMode != "application_scope" && v.ResourceMode != "resource_scoped" {
			return errors.New("resource_mode 必须明确选择 application_scope 或 resource_scoped")
		}
		if v.MaxConcurrent == 0 {
			v.MaxConcurrent = 2
		}
		if v.MaxConcurrent < 1 || v.MaxConcurrent > c.Workers {
			return errors.New("max_concurrent 必须在 1 与 workers 之间")
		}
		if v.IdentityMode == "wake_actor" && len(v.Actors) == 0 {
			return errors.New("wake_actor 接入必须明确配置 actors")
		}
	}
	return nil
}

func (c Config) client(id string) (Client, bool) {
	for _, v := range c.Clients {
		if v.ID == id && !v.Disabled {
			return v, true
		}
	}
	return Client{}, false
}
func (c Config) app(id string) (App, bool) {
	for _, v := range c.Apps {
		if v.ID == id && !v.Disabled {
			return v, true
		}
	}
	return App{}, false
}

func (c Config) authenticate(r *http.Request) (Principal, error) {
	if len(r.Header.Values("Authorization")) != 1 {
		return Principal{}, fault("unauthenticated", "需要唯一的 Bearer 接入凭据")
	}
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return Principal{}, fault("unauthenticated", "需要 Bearer 接入凭据")
	}
	hash := sha256.Sum256([]byte(strings.TrimPrefix(h, "Bearer ")))
	for _, v := range c.Clients {
		expected, _ := hex.DecodeString(v.KeySHA256)
		if subtle.ConstantTimeCompare(hash[:], expected) != 1 || v.Disabled {
			continue
		}
		a, ok := c.app(v.AppID)
		if !ok {
			return Principal{}, fault("application_disabled", "应用连接已停用")
		}
		p := Principal{ClientID: v.ID, AppID: a.ID, OrgID: a.OrgID}
		if v.IdentityMode == "wake_actor" {
			id := r.Header.Get("X-QoderWake-AtWaker-Sender-Id")
			channel := r.Header.Get("X-QoderWake-AtWaker-Sender-Channel")
			if len(r.Header.Values("X-QoderWake-AtWaker-Sender-Id")) > 1 || len(r.Header.Values("X-QoderWake-AtWaker-Sender-Channel")) > 1 {
				return Principal{}, fault("invalid_actor", "提问者 Header 重复")
			}
			if id != "" || channel != "" {
				if id == "" || (channel != "feishu" && channel != "lark") || strings.ContainsAny(id, ":\r\n") || len(id) > 128 {
					return Principal{}, fault("invalid_actor", "提问者标识或渠道无效")
				}
				p.Actor = channel + ":user_id:" + id
				if _, ok := v.Actors[p.Actor]; !ok {
					return Principal{}, fault("actor_denied", "提问者未被授予此接入权限")
				}
			}
		}
		return p, nil
	}
	return Principal{}, fault("unauthenticated", "接入凭据无效或已停用")
}

func (c Config) checkPrincipal(p Principal, requireActor bool) (Client, App, error) {
	v, ok := c.client(p.ClientID)
	if !ok {
		return Client{}, App{}, fault("access_revoked", "接入已停用")
	}
	a, ok := c.app(v.AppID)
	if !ok || a.ID != p.AppID || a.OrgID != p.OrgID {
		return v, a, fault("access_revoked", "应用绑定已改变或停用")
	}
	if v.IdentityMode == "wake_actor" {
		if requireActor && p.Actor == "" {
			return v, a, fault("actor_required", "此操作需要 Wake 提问者身份")
		}
		if p.Actor != "" {
			if _, ok := v.Actors[p.Actor]; !ok {
				return v, a, fault("actor_denied", "提问者权限已撤销")
			}
		}
	} else if p.Actor != "" {
		return v, a, fault("access_revoked", "身份模式已改变")
	}
	return v, a, nil
}

func visible(c Client, op Operation) bool {
	return (slices.Contains(c.Packs, op.Pack) || slices.Contains(c.Operations, op.ID)) && (c.ResourceMode != "resource_scoped" || len(op.ResourceBindings) > 0)
}
func authorize(c Config, p Principal, op Operation, args map[string]any) (App, error) {
	v, a, err := c.checkPrincipal(p, true)
	if err != nil {
		return a, err
	}
	if !visible(v, op) {
		return a, fault("operation_denied", "该接入未开放此操作")
	}
	rules := []map[string][]string{}
	if v.ResourceMode == "resource_scoped" {
		rules = append(rules, v.Resources)
	}
	if p.Actor != "" {
		ap := v.Actors[p.Actor]
		if len(ap.Operations) > 0 && !slices.Contains(ap.Operations, op.ID) {
			return a, fault("operation_denied", "提问者未获准执行此操作")
		}
		if len(ap.Resources) > 0 {
			if len(op.ResourceBindings) == 0 {
				return a, fault("resource_scope_unsupported", "此操作不能验证提问者资源范围")
			}
			rules = append(rules, ap.Resources)
		}
	}
	for _, rule := range rules {
		for field, kind := range op.ResourceBindings {
			parts := strings.Split(field, ".")
			value, _ := object(args[parts[0]])[parts[1]].(string)
			if value == "" || !slices.Contains(rule[kind], value) {
				return a, fault("resource_denied", "目标资源未在授权范围内")
			}
		}
	}
	return a, nil
}
