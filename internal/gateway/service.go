package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"
)

type Service struct {
	config   Config
	Catalog  *Catalog
	Store    *Store
	executor Executor
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	global   chan struct{}
	clients  map[string]chan struct{}
	activeMu sync.Mutex
	active   map[string]context.CancelFunc
}

func NewService(c Config, executor Executor) (*Service, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	catalog, err := LoadCatalog()
	if err != nil {
		return nil, err
	}
	knownPacks := map[string]bool{"files": true}
	for _, op := range catalog.Operations {
		knownPacks[op.Pack] = true
	}
	for _, client := range c.Clients {
		for _, pack := range client.Packs {
			if !knownPacks[pack] {
				return nil, errors.New("接入配置包含未知能力包: " + pack)
			}
		}
		for _, id := range client.Operations {
			if _, ok := catalog.Operation(id); !ok {
				return nil, errors.New("接入配置包含未开放操作: " + id)
			}
		}
		for _, actor := range client.Actors {
			for _, id := range actor.Operations {
				if _, ok := catalog.Operation(id); !ok {
					return nil, errors.New("actor 配置包含未开放操作: " + id)
				}
			}
		}
	}
	store, err := OpenStore(c)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{config: c, Catalog: catalog, Store: store, executor: executor, ctx: ctx, cancel: cancel, global: make(chan struct{}, c.Workers), clients: map[string]chan struct{}{}, active: map[string]context.CancelFunc{}}
	for _, client := range c.Clients {
		s.clients[client.ID] = make(chan struct{}, client.MaxConcurrent)
	}
	for i := 0; i < c.Workers; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	return s, nil
}
func (s *Service) Close() error {
	s.cancel()
	s.activeMu.Lock()
	for _, cancel := range s.active {
		cancel()
	}
	s.activeMu.Unlock()
	s.wg.Wait()
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
	op, ok := s.Catalog.Operation(operation)
	if !ok {
		return failed(fault("unknown_operation", "操作未登记或尚未适配"))
	}
	if err := op.Validate(args); err != nil {
		return failed(err)
	}
	if _, err := authorize(s.config, p, op, args); err != nil {
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
	if !async {
		if err := s.acquire(ctx, p.ClientID); err != nil {
			return failed(err)
		}
		defer s.release(p.ClientID)
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

func (s *Service) acquire(ctx context.Context, client string) error {
	sem, ok := s.clients[client]
	if !ok {
		return fault("access_revoked", "接入已不存在")
	}
	select {
	case sem <- struct{}{}:
	default:
		return fault("concurrency_limit", "该接入并发已达上限，请稍后重试")
	}
	select {
	case s.global <- struct{}{}:
		return nil
	case <-ctx.Done():
		<-sem
		return ctx.Err()
	case <-s.ctx.Done():
		<-sem
		return s.ctx.Err()
	}
}
func (s *Service) release(client string) { <-s.global; <-s.clients[client] }

func (s *Service) execute(parent context.Context, record Record) Result {
	op, ok := s.Catalog.Operation(record.OperationID)
	if !ok {
		r := record.Result
		r.Status = "failed"
		r.Error = fault("operation_removed", "原操作已不在当前目录中")
		_ = s.Store.Finish(record.ID, r)
		return r
	}
	app, err := authorize(s.config, record.Principal, op, record.Arguments)
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
	x := s.executor.Execute(ctx, Invocation{ID: record.ID, Principal: record.Principal, Operation: op, Arguments: args}, app)
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
			for _, field := range []string{"page_token", "next_page_token"} {
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
		payload := encode(r.Data)
		if len(payload) > s.config.MaxInlineBytes || r.Error != nil {
			f, e := s.Store.PutFile(record.Principal, "result.json", "application/json", payload, record.ID)
			r.Data = nil
			if e != nil {
				r.Error = fault("result_storage_failed", "外部操作已返回，但结果保存失败，请核对调用记录")
				// 已确认的外部成功不能变成可直接重试的失败。
			} else {
				r.Files = []File{f}
			}
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
			err = s.acquire(s.ctx, r.Principal.ClientID)
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
		s.release(r.Principal.ClientID)
	}
}

func (s *Service) Job(p Principal, id string, cancel bool) Result {
	if _, _, err := s.config.checkPrincipal(p, true); err != nil {
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
	if _, err = authorize(s.config, p, op, r.Arguments); err != nil {
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
	if _, _, err := s.config.checkPrincipal(p, true); err != nil {
		return File{}, err
	}
	f, err := s.Store.File(p, id)
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
		if _, err = authorize(s.config, p, op, r.Arguments); err != nil {
			return f, err
		}
	}
	return f, nil
}
