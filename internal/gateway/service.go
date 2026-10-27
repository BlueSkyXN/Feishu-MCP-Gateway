// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

type Service struct {
	lifecycleMu sync.Mutex
	closing     bool
	calls       sync.WaitGroup
	policy      policyRef
	reloadMu    sync.Mutex
	limitsMu    sync.Mutex
	counts      map[string]int
	rates       map[string]rateWindow
	bindings    map[string]App
	ConfigPath  string
	config      Config
	Catalog     *Catalog
	Guidance    *Guidance
	search      *searchIndex
	// 目录摘要计算一次即可：目录随二进制固定。
	catalogRevision string
	Store           *Store
	executor        Executor
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	activeMu        sync.Mutex
	active          map[string]context.CancelFunc
}

func NewService(c Config, executor Executor) (*Service, error) {
	c = cloneConfigValue(c)
	if err := c.Validate(); err != nil {
		return nil, err
	}
	catalog, err := LoadCatalog()
	if err != nil {
		return nil, err
	}
	if err = validateCatalogConfig(c, catalog); err != nil {
		return nil, err
	}
	guidance, err := LoadGuidance(catalog)
	if err != nil {
		return nil, err
	}
	store, err := OpenStore(c)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{config: c, Catalog: catalog, Guidance: guidance, search: newSearchIndex(catalog, guidance), catalogRevision: CatalogRevision(), Store: store, executor: executor, ctx: ctx, cancel: cancel, active: map[string]context.CancelFunc{}}

	if cli, ok := executor.(*CLIExecutor); ok {
		cli.store = store
		cli.authorizeFile = s.authorizeInputFile
		if cli.broker == nil {
			cli.broker = NewBroker(c.MaxFileBytes, nil, func(in Invocation) (App, error) {
				return authorize(s.currentConfig(), in.Principal, in.Operation, in.Arguments)
			})
		}
	}
	s.counts = map[string]int{}
	s.rates = map[string]rateWindow{}
	s.bindings = map[string]App{}
	for _, a := range c.Apps {
		s.bindings[a.ID] = a
	}

	s.policy.Store(s.makePolicy(c))
	for i := 0; i < c.Workers; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	return s, nil
}
func (s *Service) Close() error {
	s.lifecycleMu.Lock()
	s.closing = true
	s.lifecycleMu.Unlock()
	s.cancel()
	s.activeMu.Lock()
	for _, cancel := range s.active {
		cancel()
	}
	s.activeMu.Unlock()
	s.wg.Wait()
	s.calls.Wait()
	return s.Store.Close()
}
func asFault(err error) *Fault {
	var f *Fault
	if errors.As(err, &f) {
		return f
	}
	slog.Error("gateway internal error", "error", err)
	return fault("internal", "服务内部错误，请通过调用记录核对")
}
func failed(err error) Result { return Result{Status: "failed", Error: asFault(err)} }

func (s *Service) Invoke(ctx context.Context, p Principal, operation string, args map[string]any) Result {
	if !s.beginRequest() {
		return failed(fault("service_stopping", "服务正在退出"))
	}
	defer s.calls.Done()
	op, ok := s.Catalog.Operation(operation)
	if !ok {
		return failed(fault("unknown_operation", "操作未登记或尚未适配"))
	}
	if err := op.Validate(args); err != nil {
		return failed(err)
	}
	if _, err := authorize(s.currentConfig(), p, op, args); err != nil {
		return failed(err)
	}
	// 验证分页句柄，但保存 Gateway 参数，后台执行时再次解析。
	var copyArgs map[string]any
	_ = json.Unmarshal(encode(args), &copyArgs)
	if err := s.Store.UnwrapCursor(p, op.ID, copyArgs); err != nil {
		return failed(err)
	}
	idem, _ := args["idempotency_key"].(string)
	async, _ := args["async"].(bool)
	if err := s.rate(p); err != nil {
		return failed(err)
	}
	if !async {
		if err := s.acquirePrincipal(p); err != nil {
			return failed(err)
		}
		defer s.releasePrincipal(p)
	}
	record, existing, err := s.Store.Reserve(p, op, args, idem, async)
	if err != nil {
		return failed(err)
	}
	if existing || async {
		return record.Result
	}
	return s.execute(ctx, record)
}

func (s *Service) execute(parent context.Context, record Record) (result Result) {
	// 执行中的 panic 按进程中断处理：写入可能已经发生，回执记为 unknown；读取记为 failed。
	defer func() {
		if v := recover(); v != nil {
			slog.Error("invocation panic", "id", record.ID, "operation", record.OperationID, "panic", fmt.Sprint(v), "stack", string(debug.Stack()))
			result = record.Result
			result.Status = "unknown"
			if op, ok := s.Catalog.Operation(record.OperationID); ok && op.Risk == "read" {
				result.Status = "failed"
			}
			result.Error = fault("internal", "服务内部错误，请通过调用记录核对")
			_ = s.Store.Finish(record.ID, result)
		}
	}()
	op, ok := s.Catalog.Operation(record.OperationID)
	if !ok {
		r := record.Result
		r.Status = "failed"
		r.Error = fault("operation_removed", "原操作已不在当前目录中")
		_ = s.Store.Finish(record.ID, r)
		return r
	}
	app, err := authorize(s.currentConfig(), record.Principal, op, record.Arguments)
	r := record.Result
	if err != nil {
		r.Status = "failed"
		r.Error = asFault(err)
		_ = s.Store.Finish(record.ID, r)
		return r
	}
	args := map[string]any{}
	_ = json.Unmarshal(encode(record.Arguments), &args)
	if err = s.Store.UnwrapCursor(record.Principal, op.ID, args); err != nil {
		r.Status = "failed"
		r.Error = asFault(err)
		_ = s.Store.Finish(record.ID, r)
		return r
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(s.config.TimeoutSeconds)*time.Second)
	defer cancel()
	stopServiceCancel := context.AfterFunc(s.ctx, cancel)
	defer stopServiceCancel()
	s.activeMu.Lock()
	s.active[record.ID] = cancel
	s.activeMu.Unlock()
	defer func() { s.activeMu.Lock(); delete(s.active, record.ID); s.activeMu.Unlock() }()
	// 取消可能发生在领取任务与注册 cancel 函数之间。
	current, err := s.Store.Record(record.Principal, record.ID)
	if err != nil {
		return failed(err)
	}
	if current.State == "cancel_requested" {
		r.Status = "cancelled"
		_ = s.Store.Finish(record.ID, r)
		return r
	}
	var x ExecutionResult
	if op.Kind == "native" {
		x = s.readEvents(ctx, record.Principal, args)
	} else {
		x = s.executor.Execute(ctx, Invocation{ID: record.ID, Principal: record.Principal, Operation: op, Arguments: args}, app)
	}
	r.Files = x.Files
	r.Data = x.Data
	r.Meta = x.Meta
	r.Error = x.Error
	r.Status = "succeeded"
	if x.Error != nil {
		r.Status = "failed"
		if x.Error.Kind == "partial_failure" {
			r.Status = "partial"
		}
		if x.Uncertain {
			r.Status = "unknown"
		} else if ctx.Err() != nil && op.Risk == "read" {
			r.Status = "cancelled"
		}
	}
	if x.Error == nil {
		if err = op.output.Validate(x.Data); err != nil {
			r.Error = fault("backend_output_schema", "上游结果不符合目录 schema，请核对后端版本和回执")
			r.Status = "failed"
			if op.Risk != "read" {
				r.Status = "unknown"
			}
		}
	}
	if r.Data != nil {
		if data, ok := r.Data.(map[string]any); ok {
			for _, field := range []string{"page_token", "next_page_token", "cursor", "next_cursor"} {
				if token, ok := data[field].(string); ok && token != "" {
					wrapped, e := s.Store.WrapCursor(record.Principal, op.ID, record.Arguments, token)
					if e != nil {
						r.Error = fault("storage_failed", "无法保存分页标识")
						delete(data, field)
					} else {
						data[field] = wrapped
					}
				}
			}
		}
	}
	payload := encode(r)
	if len(payload) > s.config.MaxInlineBytes || (r.Error != nil && r.Data != nil) {
		f, e := s.Store.PutFile(record.Principal, "result.json", "application/json", payload, record.ID)
		count := len(r.Files)
		r.Data = nil
		r.Meta = map[string]any{"result_spilled": true, "artifact_count": count}
		if e != nil {
			r.Error = fault("result_storage_failed", "操作已返回，但结果保存失败，请核对调用记录")
			r.Files = nil
		} else {
			r.Files = []File{f}
		}
	}

	if err = s.Store.Finish(record.ID, r); err != nil {
		return Result{InvocationID: record.ID, OperationID: op.ID, Status: "unknown", Error: fault("receipt_storage_failed", "无法持久化执行回执，重试前请核对")}
	}
	slog.Info("invocation finished", "id", record.ID, "client", record.Principal.ClientID, "operation", op.ID, "status", r.Status)
	return r
}

func (s *Service) worker() {
	defer s.wg.Done()
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-tick.C:
		}
		r, err := s.Store.Claim()
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			slog.Error("job claim failed", "error", err)
			continue
		}
		// 已领取任务等待自身并发额度，不循环重新执行外部请求。
		for {
			err = s.acquirePrincipal(r.Principal)
			if err == nil {
				break
			}
			var f *Fault
			if !errors.As(err, &f) || f.Kind != "concurrency_limit" {
				break
			}
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
		if err != nil {
			result := r.Result
			result.Status = "failed"
			result.Error = asFault(err)
			_ = s.Store.Finish(r.ID, result)
			continue
		}
		s.execute(s.ctx, r)
		s.releasePrincipal(r.Principal)
	}
}

func (s *Service) Job(p Principal, id string, cancel bool) Result {
	if _, _, err := s.currentConfig().checkPrincipal(p, true); err != nil {
		return failed(err)
	}
	r, err := s.Store.Record(p, id)
	if err != nil {
		return failed(err)
	}
	op, ok := s.Catalog.Operation(r.OperationID)
	if !ok {
		return failed(fault("operation_removed", "原操作已不可用"))
	}
	if _, err = authorize(s.currentConfig(), p, op, r.Arguments); err != nil {
		return failed(err)
	}
	if !cancel {
		return r.Result
	}
	result, err := s.Store.Cancel(p, id)
	if err != nil {
		return failed(err)
	}
	s.activeMu.Lock()
	if fn := s.active[id]; fn != nil {
		fn()
	}
	s.activeMu.Unlock()
	return result
}

func (s *Service) AuthorizeFile(p Principal, id string) (File, error) {
	return s.authorizeInputFile(p, id, "")
}
func (s *Service) authorizeInputFile(p Principal, id, inv string) (File, error) {
	if _, _, err := s.currentConfig().checkPrincipal(p, true); err != nil {
		return File{}, err
	}
	f, err := s.Store.fileForInvocation(p, id, inv)
	if err != nil {
		return f, err
	}
	if f.SourceInvocation != "" {
		r, err := s.Store.Record(p, f.SourceInvocation)
		if err != nil {
			return f, err
		}
		op, ok := s.Catalog.Operation(r.OperationID)
		if !ok {
			return f, fault("operation_removed", "产物来源操作已不可用")
		}
		if _, err = authorize(s.currentConfig(), p, op, r.Arguments); err != nil {
			return f, err
		}
	}
	return f, nil
}

func (s *Service) beginRequest() bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.closing {
		return false
	}
	s.calls.Add(1)
	return true
}
