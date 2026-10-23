// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type policyState struct {
	config   Config
	views    map[string]*mcp.Server
	revision string
}
type rateWindow struct {
	start time.Time
	used  int
}
type policyRef struct{ atomic.Pointer[policyState] }

func (s *Service) currentConfig() Config {
	if p := s.policy.Load(); p != nil {
		return p.config
	}
	return s.config
}
func (s *Service) makePolicy(c Config) *policyState {
	p := &policyState{config: c, views: map[string]*mcp.Server{}, revision: digest(encode(c))}
	for _, client := range c.Clients {
		if !client.Disabled {
			p.views[client.ID] = s.mcpView(client)
		}
	}
	return p
}
func validateCatalogConfig(c Config, catalog *Catalog) error {
	packs := map[string]bool{"files": true, "events.read": true}
	for _, op := range catalog.Operations {
		packs[op.Pack] = true
	}
	for _, client := range c.Clients {
		for _, pack := range client.Packs {
			if !packs[pack] {
				return errors.New("未知能力包: " + pack)
			}
		}
		lists := [][]string{client.Operations, client.HighRiskOperations}
		for _, a := range client.Actors {
			lists = append(lists, a.Operations)
		}
		for _, list := range lists {
			for _, id := range list {
				if _, ok := catalog.Operation(id); !ok {
					return errors.New("未适配的 operation_id: " + id)
				}
			}
		}
	}
	return nil
}

// Reload 只替换接入策略与凭据；监听、后端和存储变更需要重启。
func (s *Service) Reload(c Config) error {
	c = cloneConfigValue(c)
	if err := c.Validate(); err != nil {
		return err
	}
	if err := validateCatalogConfig(c, s.Catalog); err != nil {
		return err
	}
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	old := s.currentConfig()
	base, next := old, c
	base.Apps = nil
	base.Clients = nil
	next.Apps = nil
	next.Clients = nil
	if !reflect.DeepEqual(base, next) {
		return errors.New("运行参数改变需要重启；热加载只支持 apps 与 clients")
	}
	for _, a := range c.Apps {
		if old, ok := s.bindings[a.ID]; ok && (old.AppID != a.AppID || old.OrgID != a.OrgID || old.Brand != a.Brand) {
			return errors.New("现有应用连接不能改绑；请使用新的连接 id")
		}
	}
	for _, a := range c.Apps {
		s.bindings[a.ID] = a
	}
	s.policy.Store(s.makePolicy(c))
	if cli, ok := s.executor.(*CLIExecutor); ok {
		cli.broker.tokens.Retain(c.Apps)
	}
	return nil
}
func (s *Service) rate(p Principal) error {
	c := s.currentConfig()
	client, app, err := c.checkPrincipal(p, true)
	if err != nil {
		return err
	}
	limits := map[string]int{"client:" + p.ClientID: client.MaxRequestsPerMinute, "app:" + app.ID: app.MaxRequestsPerMinute}
	if p.Actor != "" {
		limits["actor:"+p.Owner()] = client.Actors[p.Actor].MaxRequestsPerMinute
	}
	s.limitsMu.Lock()
	defer s.limitsMu.Unlock()
	now := time.Now()
	for key, limit := range limits {
		if limit <= 0 {
			continue
		}
		w := s.rates[key]
		if now.Sub(w.start) >= time.Minute {
			w = rateWindow{start: now}
			s.rates[key] = w
		}
		if w.used >= limit {
			n := max(1, time.Until(w.start.Add(time.Minute)).Seconds())
			return &Fault{Kind: "rate_limit", Message: "调用频率已达配置上限，请等待后重试", RetryAfter: &n}
		}
	}
	for key, limit := range limits {
		if limit > 0 {
			w := s.rates[key]
			w.used++
			s.rates[key] = w
		}
	}
	return nil
}
func (s *Service) acquirePrincipal(p Principal) error {
	c := s.currentConfig()
	client, app, err := c.checkPrincipal(p, true)
	if err != nil {
		return err
	}
	limits := map[string]int{"global": c.Workers, "client:" + p.ClientID: client.MaxConcurrent, "app:" + app.ID: app.MaxConcurrent}
	if p.Actor != "" {
		limits["actor:"+p.Owner()] = client.Actors[p.Actor].MaxConcurrent
	}
	s.limitsMu.Lock()
	defer s.limitsMu.Unlock()
	for key, limit := range limits {
		if limit > 0 && s.counts[key] >= limit {
			return fault("concurrency_limit", "并发已达配置上限，请稍后重试")
		}
	}
	for key := range limits {
		s.counts[key]++
	}
	return nil
}
func (s *Service) releasePrincipal(p Principal) {
	s.limitsMu.Lock()
	defer s.limitsMu.Unlock()
	for _, key := range []string{"global", "client:" + p.ClientID, "app:" + p.AppID} {
		s.counts[key]--
	}
	if p.Actor != "" {
		s.counts["actor:"+p.Owner()]--
	}
}
func (s *Service) admin(w http.ResponseWriter, r *http.Request) {
	expected, _ := hex.DecodeString(s.currentConfig().AdminKeySHA256)
	got, _ := hex.DecodeString(digest([]byte(bearer(r))))
	if len(expected) != 32 || subtle.ConstantTimeCompare(expected, got) != 1 {
		http.Error(w, "admin authentication required", http.StatusUnauthorized)
		return
	}
	if r.Method == "POST" && r.URL.Path == "/admin/reload" {
		if s.ConfigPath == "" {
			http.Error(w, "configuration path unavailable", 400)
			return
		}
		c, err := LoadConfig(s.ConfigPath)
		if err == nil {
			err = s.Reload(c)
		}
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	} else if r.Method != "GET" || r.URL.Path != "/admin/status" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encode(s.Diagnostics()))
}
func bearer(r *http.Request) string {
	if len(r.Header.Values("Authorization")) != 1 {
		return ""
	}
	h := r.Header.Get("Authorization")
	if len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	return ""
}
func (s *Service) Diagnostics() map[string]any {
	c := s.currentConfig()
	s.limitsMu.Lock()
	active := s.counts["global"]
	s.limitsMu.Unlock()
	data := map[string]any{"catalog_revision": CatalogRevision(), "operations": len(s.Catalog.Operations), "applications": len(c.Apps), "clients": len(c.Clients), "active": active, "policy_revision": s.policy.Load().revision}
	if cli, ok := s.executor.(*CLIExecutor); ok {
		data["tokens"] = cli.broker.tokens.Stats()
	}
	return data
}

func cloneConfigValue(c Config) Config {
	var out Config
	_ = json.Unmarshal(encode(c), &out)
	return out
}
