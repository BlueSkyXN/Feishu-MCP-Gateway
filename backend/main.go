// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	_ "github.com/larksuite/cli/extension/credential/sidecar"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/shortcuts"
	"github.com/larksuite/cli/shortcuts/common"
)

var sourceCommit = "unknown"

type shortcutInfo struct {
	Service           string                     `json:"service"`
	Command           string                     `json:"command"`
	Description       string                     `json:"description"`
	Risk              string                     `json:"risk"`
	AuthTypes         []string                   `json:"auth_types"`
	Flags             []common.Flag              `json:"flags"`
	Scopes            []string                   `json:"app_scopes"`
	ConditionalScopes []string                   `json:"conditional_app_scopes"`
	Hidden            bool                       `json:"hidden"`
	HasDryRun         bool                       `json:"has_dry_run"`
	FlagSchemas       map[string]json.RawMessage `json:"flag_schemas,omitempty"`
	Tips              []string                   `json:"tips,omitempty"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--gateway-capabilities" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"contract": "feishu-gateway-backend-v1", "source_commit": sourceCommit, "app_only": true, "rooted_fileio": true, "sidecar_required": true, "artifact_envelope": true})
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--gateway-export-shortcuts" {
		var all []shortcutInfo
		for _, s := range shortcuts.AllShortcuts() {
			v := shortcutInfo{Service: s.Service, Command: s.Command, Description: s.Description, Risk: s.Risk, AuthTypes: s.AuthTypes, Flags: s.Flags, Scopes: s.ScopesForIdentity("bot"), ConditionalScopes: s.ConditionalScopesForIdentity("bot"), Hidden: s.Hidden, HasDryRun: s.DryRun != nil, Tips: s.Tips}
			if s.PrintFlagSchema != nil {
				v.FlagSchemas = map[string]json.RawMessage{}
				for _, f := range s.Flags {
					if b, err := s.PrintFlagSchema(f.Name); err == nil && json.Valid(b) {
						v.FlagSchemas[f.Name] = b
					}
				}
			}
			all = append(all, v)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"source_commit": sourceCommit, "shortcuts": all})
		return
	}
	if os.Getenv("LARKSUITE_CLI_AUTH_PROXY") == "" || os.Getenv("LARKSUITE_CLI_PROXY_KEY") == "" || os.Getenv("LARKSUITE_CLI_APP_ID") == "" {
		fail("missing_proxy", "受控后端需要 Gateway 提供调用代理")
		return
	}
	root := os.Getenv("FEISHU_GATEWAY_WORKDIR")
	if root == "" {
		fail("missing_workspace", "受控后端需要独立工作目录")
		return
	}
	if err := os.Chdir(root); err != nil {
		fail("invalid_workspace", "工作目录不可用")
		return
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		fail("invalid_workspace", "无法建立文件访问边界")
		return
	}
	defer fs.Close()
	fileio.Register(&rootedFiles{root: fs, path: root, limit: envLimit("FEISHU_GATEWAY_FILE_LIMIT", 16<<20), total: envLimit("FEISHU_GATEWAY_WORKSPACE_LIMIT", 64<<20)})
	transport.Register(gatewayTransport{})
	_ = os.Setenv("LARKSUITE_CLI_DEFAULT_AS", "bot")
	_ = os.Setenv("LARKSUITE_CLI_STRICT_MODE", "bot")
	os.Unsetenv("LARKSUITE_CLI_USER_ACCESS_TOKEN")
	os.Unsetenv("LARKSUITE_CLI_APP_SECRET")
	os.Unsetenv("LARKSUITE_CLI_TENANT_ACCESS_TOKEN")
	os.Exit(executeCLI())
}
func fail(kind, message string) {
	_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"ok": false, "identity": "bot", "error": map[string]any{"type": "policy", "subtype": kind, "message": message}})
	os.Exit(6)
}

type rootedFiles struct {
	root                  *os.Root
	path                  string
	limit, total, written int64
	mu                    sync.Mutex
}

func (r *rootedFiles) Name() string                                { return "gateway-rooted" }
func (r *rootedFiles) ResolveFileIO(context.Context) fileio.FileIO { return r }
func (r *rootedFiles) relative(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty file path")
	}
	if filepath.IsAbs(path) {
		var err error
		path, err = filepath.Rel(r.path, path)
		if err != nil {
			return "", err
		}
	}
	path = filepath.Clean(path)
	if path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file path outside invocation workspace")
	}
	return path, nil
}
func (r *rootedFiles) Open(path string) (fileio.File, error) {
	p, e := r.relative(path)
	if e != nil {
		return nil, e
	}
	return r.root.Open(p)
}
func (r *rootedFiles) Stat(path string) (fileio.FileInfo, error) {
	p, e := r.relative(path)
	if e != nil {
		return nil, e
	}
	return r.root.Stat(p)
}
func (r *rootedFiles) ResolvePath(path string) (string, error) {
	p, e := r.relative(path)
	if e != nil {
		return "", e
	}
	if _, e = r.root.Stat(filepath.Dir(p)); e != nil && !os.IsNotExist(e) {
		return "", e
	}
	return filepath.Join(r.path, p), nil
}

type saved int64

func (s saved) Size() int64 { return int64(s) }
func (r *rootedFiles) Save(path string, opts fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	return r.save(path, opts, body, false)
}
func (r *rootedFiles) SaveExclusive(path string, opts fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	return r.save(path, opts, body, true)
}
func (r *rootedFiles) save(path string, opts fileio.SaveOptions, body io.Reader, exclusive bool) (fileio.SaveResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, e := r.relative(path)
	if e != nil {
		return nil, e
	}
	if e = r.root.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return nil, e
	}
	if r.limit <= 0 || r.total <= 0 {
		return nil, fmt.Errorf("file limits unavailable")
	}
	if opts.ContentLength > r.limit {
		return nil, fmt.Errorf("file exceeds invocation limit")
	}
	tmp := filepath.Join(filepath.Dir(p), ".gateway-write-"+rand.Text())
	f, e := r.root.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if e != nil {
		return nil, e
	}
	defer r.root.Remove(tmp)
	n, e := io.Copy(f, io.LimitReader(body, min(r.limit, r.total-r.written)+1))
	if e == nil && (n > r.limit || n+r.written > r.total) {
		e = fmt.Errorf("file or workspace capacity exceeded")
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return nil, e
	}
	if exclusive {
		e = r.root.Link(tmp, p)
	} else {
		e = r.root.Rename(tmp, p)
	}
	if e != nil {
		return nil, e
	}
	r.written += n

	return saved(n), nil
}
func (r *rootedFiles) RemoveWorkspaceEntry(path string) error {
	p, e := r.relative(path)
	if e != nil {
		return e
	}
	return r.root.Remove(p)
}

func envLimit(name string, fallback int64) int64 {
	n, e := strconv.ParseInt(os.Getenv(name), 10, 64)
	if e != nil || n <= 0 {
		return fallback
	}
	return n
}
