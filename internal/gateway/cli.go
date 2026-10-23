package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type CLIExecutor struct {
	binary, sha, root string
	limit             int64
}

func NewCLIExecutor(c Config) (*CLIExecutor, error) {
	if !filepath.IsAbs(c.CLIBinary) || len(c.CLISHA256) != 64 {
		return nil, errors.New("cli_binary 必须为绝对路径，并配置固定文件的 cli_sha256")
	}
	e := &CLIExecutor{binary: c.CLIBinary, sha: strings.ToLower(c.CLISHA256), root: filepath.Join(c.DataDir, "work"), limit: c.MaxFileBytes}
	if err := e.verify(); err != nil {
		return nil, err
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
	bytes.Buffer
	max      int64
	overflow bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	left := b.max - int64(b.Len())
	if left <= 0 {
		b.overflow = true
		return n, nil
	}
	if int64(n) > left {
		b.overflow = true
		p = p[:left]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func (e *CLIExecutor) Execute(ctx context.Context, in Invocation, app App) ExecutionResult {
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
	argv := append([]string{}, in.Operation.Command...)
	argv = append(argv, "--as", "bot", "--format", "json")
	for _, section := range []string{"params", "data"} {
		if value, ok := in.Arguments[section]; ok {
			path := filepath.Join(dir, section+".json")
			if err = os.WriteFile(path, encode(value), 0600); err != nil {
				return ExecutionResult{Error: fault("storage_failed", "无法写入执行参数")}
			}
			argv = append(argv, "--"+section, "@"+path)
		}
	}
	cmd := exec.CommandContext(ctx, e.binary, argv...)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + dir, "TMPDIR=" + dir,
		"LANG=en_US.UTF-8", "CI=1", "NO_COLOR=1",
		"LARKSUITE_CLI_CONFIG_DIR=" + filepath.Join(dir, "config"),
		"LARKSUITE_CLI_APP_ID=" + app.AppID, "LARKSUITE_CLI_APP_SECRET=" + app.secret,
		"LARKSUITE_CLI_BRAND=" + app.Brand, "LARKSUITE_CLI_DEFAULT_AS=bot", "LARKSUITE_CLI_STRICT_MODE=bot",
		"LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1", "LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1",
	}
	out, stderr := &cappedBuffer{max: e.limit}, &cappedBuffer{max: 1 << 20}
	cmd.Stdout = out
	cmd.Stderr = stderr
	cmd.Stdin = strings.NewReader("")
	err = cmd.Run()
	uncertain := in.Operation.Risk != "read"
	if out.overflow || stderr.overflow {
		return ExecutionResult{Error: fault("backend_output_limit", "执行输出超过上限，需要核对操作回执"), Uncertain: uncertain}
	}
	var envelope struct {
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
	valid := json.Unmarshal(out.Bytes(), &envelope) == nil && envelope.OK != nil
	if valid && envelope.Identity != "bot" {
		return ExecutionResult{Error: fault("identity_violation", "后端未确认应用身份，请核对 CLI 版本"), Uncertain: uncertain}
	}
	if err == nil && valid && *envelope.OK {
		return ExecutionResult{Data: envelope.Data, Meta: envelope.Meta}
	}
	if !valid || envelope.OK == nil || *envelope.OK {
		for _, line := range bytes.Split(stderr.Bytes(), []byte("\n")) {
			var tmp = envelope
			if json.Unmarshal(line, &tmp) == nil && tmp.OK != nil && !*tmp.OK {
				envelope = tmp
				valid = true
			}
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
