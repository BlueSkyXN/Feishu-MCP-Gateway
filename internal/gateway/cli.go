// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type backendEnvelope struct {
	OK       *bool          `json:"ok"`
	Identity string         `json:"identity"`
	Data     any            `json:"data"`
	Meta     map[string]any `json:"meta"`
	Error    struct {
		Type          string   `json:"type"`
		Subtype       string   `json:"subtype"`
		Code          *int     `json:"code"`
		Message       string   `json:"message"`
		LogID         string   `json:"log_id"`
		MissingScopes []string `json:"missing_scopes"`
		RetryAfter    *float64 `json:"retry_after_seconds"`
	} `json:"error"`
}

type CLIExecutor struct {
	binary, sha, root string
	limit             int64
	workspaceLimit    int64
	broker            *Broker
	store             *Store
	authorizeFile     func(Principal, string, string) (File, error)
}

func NewCLIExecutor(c Config) (*CLIExecutor, error) {
	if !filepath.IsAbs(c.CLIBinary) || len(c.CLISHA256) != 64 {
		return nil, errors.New("cli_binary 必须为绝对路径，并配置固定文件的 cli_sha256")
	}
	e := &CLIExecutor{binary: c.CLIBinary, sha: strings.ToLower(c.CLISHA256), root: filepath.Join(c.DataDir, "work"), limit: c.MaxFileBytes, workspaceLimit: c.MaxWorkspaceBytes}
	if err := e.verify(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, e.binary, "--gateway-capabilities")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "NO_COLOR=1"}
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.New("需要 lark-cli-gateway 受控后端，无法读取后端契约")
	}
	var caps struct {
		Contract  string `json:"contract"`
		Source    string `json:"source_commit"`
		AppOnly   bool   `json:"app_only"`
		Rooted    bool   `json:"rooted_fileio"`
		Artifacts bool   `json:"artifact_envelope"`
		Proxy     bool   `json:"sidecar_required"`
	}
	cat, err := LoadCatalog()
	if err != nil {
		return nil, err
	}
	if err = validateCatalogConfig(c, cat); err != nil {
		return nil, err
	}
	if json.Unmarshal(out, &caps) != nil || caps.Contract != "feishu-gateway-backend-v1" || caps.Source != cat.SourceCommit || !caps.AppOnly || !caps.Rooted || !caps.Proxy || !caps.Artifacts {
		return nil, errors.New("受控后端契约、产物回执能力或源码版本与目录不一致；请重新构建配套 wrapper 并更新 cli_sha256")
	}
	if err := os.MkdirAll(e.root, 0700); err != nil {
		return nil, err
	}
	return e, nil
}
func (e *CLIExecutor) verify() error {
	b, err := os.ReadFile(e.binary)
	if err != nil {
		return err
	}
	if digest(b) != e.sha {
		return errors.New("lark-cli 校验值与 cli_sha256 不一致")
	}
	return nil
}

type cappedBuffer struct {
	buffer   bytes.Buffer
	max      int64
	overflow bool
}

func (b *cappedBuffer) Len() int      { return b.buffer.Len() }
func (b *cappedBuffer) Bytes() []byte { return b.buffer.Bytes() }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	left := max(0, b.max-int64(b.buffer.Len()))
	if int64(n) > left {
		b.overflow = true
		p = p[:left]
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

func (e *CLIExecutor) Execute(ctx context.Context, in Invocation, app App) (result ExecutionResult) {
	if e.broker == nil || e.store == nil {
		return ExecutionResult{Error: fault("backend_unbound", "执行器需要绑定 Gateway 服务")}
	}
	if err := e.verify(); err != nil {
		return ExecutionResult{Error: fault("backend_version_mismatch", "CLI 文件已改变，请重新核对目录与后端版本")}
	}
	dir, err := os.MkdirTemp(e.root, "inv-")
	if err != nil {
		return ExecutionResult{Error: fault("storage_failed", "无法建立执行目录")}
	}
	defer os.RemoveAll(dir)
	// macOS 的临时目录可能通过 /var 指向 /private/var；统一真实路径，
	// 避免 HOME、PWD 与参数文件出现两个不同的目录表示。
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return ExecutionResult{Error: fault("storage_failed", "无法解析执行目录")}
	}
	if err = os.MkdirAll(filepath.Join(dir, ".gateway"), 0700); err != nil {
		return ExecutionResult{Error: fault("storage_failed", "无法建立私有参数目录")}
	}
	w, err := e.prepareFiles(in, dir)
	if err != nil {
		return ExecutionResult{Error: asFault(err)}
	}
	defer w.root.Close()
	argv, err := e.buildArgs(in, w)
	if err != nil {
		return ExecutionResult{Error: asFault(err)}
	}
	in.FileHashes = w.hashes
	in.ExternalURLs = w.urls
	proxy, err := e.broker.Open(ctx, in)
	if err != nil {
		return ExecutionResult{Error: asFault(err)}
	}
	defer proxy.Close()
	defer func() {
		proxyError, started := proxy.Outcome()
		if result.Error != nil && result.Error.Kind != "partial_failure" && proxy.HasConfirmedWrite() {
			result.Uncertain = true
		}
		if proxyError != nil && result.Error != nil {
			result.Error = proxyError
			result.Uncertain = result.Uncertain || started
		}
		files, paths, err := w.collect(e, in)
		result.Files = append(result.Files, files...)
		if result.Error != nil {
			result.Error.Message = strings.ReplaceAll(result.Error.Message, dir, ".")
		}
		result.Data = w.rewrite(result.Data, paths)
		if result.Meta != nil {
			result.Meta = object(w.rewrite(result.Meta, paths))
		}
		if err != nil {
			if result.Error == nil {
				result.Error = fault("artifact_collection_failed", "执行已返回，但部分产物未归档")
				result.Uncertain = started
			}
		}
	}()

	cmd := exec.CommandContext(ctx, e.binary, argv...)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + filepath.Join(dir, ".gateway"), "TMPDIR=" + filepath.Join(dir, ".gateway"),
		"LANG=en_US.UTF-8", "CI=1", "NO_COLOR=1",
		"LARKSUITE_CLI_CONFIG_DIR=" + filepath.Join(dir, ".gateway/config"),
		"LARKSUITE_CLI_APP_ID=" + app.AppID,
		"LARKSUITE_CLI_AUTH_PROXY=" + proxy.URL, "LARKSUITE_CLI_PROXY_KEY=" + proxy.Key,
		"FEISHU_GATEWAY_ARTIFACT_MODE=" + in.Operation.outputFormat(in.Arguments),
		"FEISHU_GATEWAY_WORKDIR=" + dir, "FEISHU_GATEWAY_FILE_LIMIT=" + fmt.Sprint(e.limit), "FEISHU_GATEWAY_WORKSPACE_LIMIT=" + fmt.Sprint(e.workspaceLimit),
		"LARKSUITE_CLI_BRAND=" + app.Brand, "LARKSUITE_CLI_DEFAULT_AS=bot", "LARKSUITE_CLI_STRICT_MODE=bot",
		"LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1", "LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1",
	}
	out, stderr := &cappedBuffer{max: e.limit}, &cappedBuffer{max: 1 << 20}
	cmd.Stdout = out
	cmd.Stderr = stderr
	cmd.Stdin = strings.NewReader("")
	err = cmd.Run()
	_, uncertain := proxy.Outcome()
	if out.overflow || stderr.overflow {
		return ExecutionResult{Error: fault("backend_output_limit", "执行输出超过上限，需要核对操作回执"), Uncertain: uncertain}
	}
	var envelope backendEnvelope
	valid := json.Unmarshal(out.Bytes(), &envelope) == nil && envelope.OK != nil
	if valid && envelope.Identity != "bot" {
		return ExecutionResult{Error: fault("identity_violation", "后端未确认应用身份，请核对 CLI 版本"), Uncertain: uncertain}
	}
	if err == nil && valid && *envelope.OK {
		return ExecutionResult{Data: envelope.Data, Meta: envelope.Meta}
	}
	if !valid || envelope.OK == nil || *envelope.OK {
		if problem, ok := decodeBackendError(stderr.Bytes()); ok {
			// CLI 在解析 flag 时尚未解析身份；这类错误不能携带业务数据。
			if (problem.Identity != "" && problem.Identity != "bot") ||
				(problem.Identity == "" && (problem.Data != nil || problem.Meta != nil || problem.Error.Type == "")) {
				return ExecutionResult{Error: fault("identity_violation", "后端未确认应用身份，请核对 CLI 版本"), Uncertain: uncertain}
			}
			envelope, valid = problem, true
		}
	}
	if valid && envelope.OK != nil && !*envelope.OK {
		problem := envelope.Error
		if problem.Type == "" && envelope.Data != nil {
			return ExecutionResult{Data: envelope.Data, Meta: envelope.Meta, Error: fault("partial_failure", "部分项目执行失败，请根据逐项回执核对；不要重试全部操作")}
		}
		msg := problem.Message
		if app.secret != "" {
			msg = strings.ReplaceAll(msg, app.secret, "[redacted]")
		}
		if problem.Type == "authentication" || problem.Type == "authorization" || problem.Type == "config" {
			msg = "请检查飞书应用凭据、应用权限和资源授权；此服务仅支持应用身份"
		}
		if len(msg) > 1024 {
			msg = msg[:1024]
		}
		u := uncertain && (problem.Type == "network" || problem.Type == "internal")
		return ExecutionResult{Data: envelope.Data, Meta: envelope.Meta, Error: &Fault{Kind: "upstream_" + problem.Type + "_" + problem.Subtype, Message: msg, UpstreamCode: problem.Code, LogID: problem.LogID, MissingScopes: problem.MissingScopes, RetryAfter: problem.RetryAfter}, Uncertain: u}
	}
	if ctx.Err() != nil {
		return ExecutionResult{Error: fault("execution_interrupted", "执行已中断，外部写入需要核对"), Uncertain: uncertain}
	}
	return ExecutionResult{Error: fault("backend_contract", "后端未返回有效的应用身份 JSON 回执"), Uncertain: uncertain}
}

// readBounded 对 HTTP 和资源输入使用统一的有界读取。
func readBounded(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fault("input_too_large", "输入超过大小限制")
	}
	return b, nil
}

// 错误可能是缩进 JSON，前后还可能有 CLI 提示；按完整 JSON 值读取，提示行跳过。
func decodeBackendError(raw []byte) (backendEnvelope, bool) {
	var last backendEnvelope
	found := false
	for len(raw) > 0 {
		raw = bytes.TrimSpace(raw)
		var value backendEnvelope
		decoder := json.NewDecoder(bytes.NewReader(raw))
		if err := decoder.Decode(&value); err == nil {
			raw = raw[decoder.InputOffset():]
			if value.OK != nil && !*value.OK {
				last, found = value, true
			}
			continue
		}
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			break
		}
		raw = raw[i+1:]
	}
	return last, found
}
