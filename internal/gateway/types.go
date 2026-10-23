package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type Principal struct {
	ClientID string `json:"client_id"`
	AppID    string `json:"app_connection_id"`
	OrgID    string `json:"organization_id"`
	Actor    string `json:"actor,omitempty"`
}

func (p Principal) Owner() string { b, _ := json.Marshal(p); return digest(b) }
func digest(b []byte) string      { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
func newID(prefix string) string  { return prefix + rand.Text() }

type Fault struct {
	Kind          string   `json:"kind"`
	Message       string   `json:"message"`
	UpstreamCode  *int     `json:"upstream_code,omitempty"`
	LogID         string   `json:"log_id,omitempty"`
	MissingScopes []string `json:"missing_scopes,omitempty"`
	RetryAfter    *float64 `json:"retry_after_seconds,omitempty"`
}

func (e *Fault) Error() string      { return e.Kind + ": " + e.Message }
func fault(kind, msg string) *Fault { return &Fault{Kind: kind, Message: msg} }

type File struct {
	SourceInvocation string `json:"source_invocation,omitempty"`
	ID               string `json:"file_id"`
	Name             string `json:"name"`
	MIME             string `json:"mime_type"`
	Size             int64  `json:"size"`
	SHA256           string `json:"sha256"`
	Expires          int64  `json:"expires_at"`
	URI              string `json:"uri"`
}

type Result struct {
	InvocationID string         `json:"invocation_id,omitempty"`
	OperationID  string         `json:"operation_id,omitempty"`
	Status       string         `json:"status"`
	Data         any            `json:"data,omitempty"`
	Meta         map[string]any `json:"meta,omitempty"`
	JobID        string         `json:"job_id,omitempty"`
	Files        []File         `json:"files,omitempty"`
	Error        *Fault         `json:"error,omitempty"`
}

type Invocation struct {
	ID        string
	Principal Principal
	Operation Operation
	Arguments map[string]any
}

type ExecutionResult struct {
	Data      any
	Meta      map[string]any
	Error     *Fault
	Uncertain bool
}
type Executor interface {
	Execute(context.Context, Invocation, App) ExecutionResult
}

func encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("invalid internal JSON: %v", err))
	}
	return b
}
func object(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}
