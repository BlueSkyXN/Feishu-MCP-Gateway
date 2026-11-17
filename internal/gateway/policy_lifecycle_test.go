// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 已结束的动态 actor 不得在并发计数表中留下闲置条目（A01）。
// 通过真实 HTTP Handler 触发计数增减，规模沿用缺陷复现的 2,000 个不同 actor。
func TestPrincipalCountersReleasedAfterHTTPRequests(t *testing.T) {
	const callers = 2000
	c := requestBearerConfig(t)
	c.Clients[0].IdentityMode = "actor"
	c.Clients[0].Actors = nil
	// 配置客户端级速率限额，使速率窗口真实存在：并发归零不得连带清空它。
	c.Clients[0].MaxRequestsPerMinute = callers + 100
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	s := startService(t, c, &fakeExecutor{})
	handler := s.Handler()
	for i := 0; i < callers; i++ {
		r := httptest.NewRequest(http.MethodGet, "/files/f_missing", nil)
		r.Header = requestHeaders("")
		r.Header.Set("X-QoderWake-AtWaker-Sender-Id", fmt.Sprintf("review-caller-%d", i))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("request %d: HTTP %d", i, w.Code)
		}
	}
	s.limitsMu.Lock()
	idle := 0
	for key, n := range s.counts {
		if n < 0 {
			t.Errorf("counter %s went negative: %d", key, n)
		}
		if strings.HasPrefix(key, "actor:") && n == 0 {
			idle++
		}
	}
	global := s.counts["global"]
	actorEntries := 0
	for key := range s.counts {
		if strings.HasPrefix(key, "actor:") {
			actorEntries++
		}
	}
	// rates 是按分钟的调用频率窗口，随并发归零清空会绕过速率限制。
	clientRates := s.rates["client:wake"].used
	s.limitsMu.Unlock()
	if idle != 0 || actorEntries != 0 {
		t.Errorf("completed dynamic callers retained counter keys: idle=%d entries=%d", idle, actorEntries)
	}
	if global != 0 {
		t.Errorf("global counter not released: %d", global)
	}
	// rates 是按分钟的调用频率窗口，随并发归零清空会绕过速率限制。
	// client:wake 的并发计数已归零删除，但它的速率窗口必须保留全部调用次数。
	// 动态 actor 未配置速率限额，不会产生 actor 速率条目，这里不要求。
	if clientRates < callers {
		t.Errorf("rate windows were cleared together with concurrency counters: client_used=%d", clientRates)
	}
}

// 相同 actor 的两个重叠请求：一次完成不能删除另一次仍占用的计数（A02）。
// 第一个请求结束后 actor/client/app 计数仍为 1；两个请求都结束后归零删除，
// 速率窗口不随并发清空。用同步屏障控制重叠，不依赖 sleep。
func TestOverlappingSameActorKeepsCounterUntilLastRelease(t *testing.T) {
	c := requestBearerConfig(t)
	alice := c.Clients[0].Actors["feishu:user_id:alice"]
	alice.Resources["wiki_space"] = []string{"space-a", "space-b"}
	// actor 级速率限额使速率窗口真实存在：并发归零不得连带清空它。
	alice.MaxRequestsPerMinute = 100
	c.Clients[0].Actors["feishu:user_id:alice"] = alice
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	// 两条独立屏障：第一条请求先放行，第二条保持阻塞，验证 2→1→0 的中间状态。
	entered := make(chan string, 2)
	proceedFirst := make(chan struct{})
	proceedSecond := make(chan struct{})
	f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, _ App) ExecutionResult {
		space, _ := object(in.Arguments["params"])["space_id"].(string)
		select {
		case entered <- space:
		case <-ctx.Done():
			return ExecutionResult{Error: fault("test_timeout", "entry cancelled")}
		}
		gate := proceedFirst
		if space == "space-b" {
			gate = proceedSecond
		}
		select {
		case <-gate:
		case <-ctx.Done():
			return ExecutionResult{Error: fault("test_timeout", "barrier cancelled")}
		}
		return ExecutionResult{Data: map[string]any{"space": map[string]any{"space_id": space}}}
	}}
	s := startService(t, c, f)
	server := httptest.NewServer(s.Handler())
	t.Cleanup(server.Close)
	first, _ := connectRequestHeaders(t, server.URL, requestHeaders("overlap-token-a"))
	second, _ := connectRequestHeaders(t, server.URL, requestHeaders("overlap-token-b"))
	op, ok := s.Catalog.Operation("wiki.spaces.get")
	if !ok {
		t.Fatal("fixture operation missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	// 有界超时与清理：断言失败也放行/取消两条阻塞路径。
	t.Cleanup(func() {
		cancel()
		safeClose(proceedFirst)
		safeClose(proceedSecond)
	})
	done := make(chan error, 2)
	call := func(session *mcp.ClientSession, space string) {
		go func() {
			_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: op.Tool, Arguments: argsSpace(space)})
			done <- err
		}()
	}
	owner := "actor:" + actor("alice").Owner()
	assertCounters := func(want int, stage string) {
		t.Helper()
		s.limitsMu.Lock()
		defer s.limitsMu.Unlock()
		for _, key := range []string{owner, "client:wake", "app:app-a", "global"} {
			if got := s.counts[key]; got != want {
				t.Fatalf("%s: counter %s = %d; want %d", stage, key, got, want)
			}
		}
	}
	call(first, "space-a")
	call(second, "space-b")
	// 两个请求都进入执行器：两个 acquire 都已发生，四类计数均为 2。
	for range 2 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("both requests did not reach the executor before timeout")
		}
	}
	assertCounters(2, "overlapping")
	// 只放行第一条：它真正返回后，第二条仍阻塞，四类计数都应为 1。
	close(proceedFirst)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("first call failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("first call did not finish before timeout")
	}
	assertCounters(1, "one released")
	// 放行第二条：归零后 key 被删除。
	close(proceedSecond)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second call failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("second call did not finish before timeout")
	}
	assertCountersEmpty(t, s, map[string]bool{owner: true})
	// 速率窗口按调用累计，不能随并发归零一起消失。
	s.limitsMu.Lock()
	rateUsed := s.rates[owner].used
	s.limitsMu.Unlock()
	if rateUsed < 2 {
		t.Fatalf("actor rate window lost usage: %d; want >= 2", rateUsed)
	}
	if f.calls.Load() != 2 {
		t.Fatalf("executor calls = %d; want 2", f.calls.Load())
	}
}

// R08：取消一条阻塞中的活动同步请求后，计数被释放且归零删除。
// F4/R08：客户端放弃一条阻塞中的同步 HTTP 请求后，服务端收尾并释放全部计数。
//
// 覆盖边界：本用例证明“请求收尾→计数释放”，不证明服务端执行器观察到取消——
// 当前 Streamable JSONResponse 模式未开启 SDK 的 PropagateRequestCancellation
// （且该选项仅对 >=2026-07-28 协议生效），客户端断开不传播为执行 ctx 取消。
// 服务端取消语义由 TestSyncInvokeCancellationObserved（Service.Invoke 直连）
// 与 managed worker 取消路径证明。
//
// 说明：JSONResponse 无状态模式下，客户端断开不保证传播为执行 ctx 的取消
// （既有实测：取消后的调用可能以 succeeded 收尾）。因此本用例证明的是
// “请求结束 → 计数释放”这一生命周期；可证实的服务端取消语义由
// TestManagedWorkerReleasesCountersAfterFinishAndCancel 的取消路径覆盖
// （Job(cancel) 触发执行 ctx 取消，终态为 cancelled）。
func TestAbandonedHTTPRequestStillReleasesCounters(t *testing.T) {
	c := requestBearerConfig(t)
	alice := c.Clients[0].Actors["feishu:user_id:alice"]
	alice.Resources["wiki_space"] = []string{"space-a"}
	c.Clients[0].Actors["feishu:user_id:alice"] = alice
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 1)
	blocked := make(chan struct{})
	returned := make(chan struct{}, 1)
	f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, _ App) ExecutionResult {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-blocked
		select {
		case returned <- struct{}{}:
		default:
		}
		return ExecutionResult{Data: map[string]any{}}
	}}
	s := startService(t, c, f)
	server := httptest.NewServer(s.Handler())
	t.Cleanup(server.Close)
	owner := "actor:" + actor("alice").Owner()
	op, _ := s.Catalog.Operation("wiki.spaces.get")
	session, _ := connectRequestHeaders(t, server.URL, requestHeaders("cancel-token"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// 清理独立于成功路径：只取消上下文、放行自己创建的屏障；断言失败也能结束。
	t.Cleanup(func() {
		cancel()
		safeClose(blocked)
	})
	done := make(chan error, 1)
	go func() {
		_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: op.Tool, Arguments: argsSpace("space-a")})
		done <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("request did not reach the executor")
	}
	s.limitsMu.Lock()
	if got := s.counts[owner]; got != 1 {
		s.limitsMu.Unlock()
		t.Fatalf("counter before cancel = %d; want 1", got)
	}
	s.limitsMu.Unlock()
	// 客户端取消并放行执行器：请求必须完整返回（执行器已进入返回路径），
	// 随后网关释放全部计数。
	cancel()
	safeClose(blocked)
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("executor never returned after the client cancelled")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled call did not return")
	}
	// 有界等待计数归零：不靠客户端返回与服务端 defer 释放之间的竞态时序。
	if !waitForCountersEmpty(t, s, 5*time.Second) {
		s.limitsMu.Lock()
		leaked := fmt.Sprint(s.counts)
		s.limitsMu.Unlock()
		t.Fatalf("counters were not released after the cancelled request: %s", leaked)
	}
	assertCountersEmpty(t, s, map[string]bool{owner: true})
}

// T2：同步 Service.Invoke 路径证明执行器实际观察到取消（request_bearer）。
// 从测试 Header 取得 requestContext 绑定可取消父 context；屏障确认执行器进入
// 并占有四层计数 → 取消 → 独立屏障确认执行器观察到 ctx.Done → 核对真实回执
// （cancelled 状态与错误）与 SQLite → 四层计数释放且归零 key 删除。
func TestSyncInvokeCancellationObserved(t *testing.T) {
	c := requestBearerConfig(t)
	entered := make(chan string, 1)
	observed := make(chan struct{}, 1)
	abort := make(chan struct{})
	f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, _ App) ExecutionResult {
		entered <- in.ID
		select {
		case <-ctx.Done():
			observed <- struct{}{}
			return ExecutionResult{Error: fault("cancelled", "sync executor observed context cancellation")}
		case <-abort:
			return ExecutionResult{Error: fault("test_cleanup", "cleanup was needed")}
		}
	}}
	s := startService(t, c, f)
	parent, cancel := context.WithCancel(context.Background())
	// 失败清理独立于成功路径：取消上下文并放行执行器，等待全部有界。
	t.Cleanup(func() {
		cancel()
		safeClose(abort)
	})
	state, err := c.authenticateRequest(&http.Request{Header: requestHeaders("sync-cancel-token")})
	if err != nil {
		t.Fatal(err)
	}
	ctx := withRequestInfo(parent, state)
	result := make(chan Result, 1)
	go func() { result <- s.Invoke(ctx, state.Principal, "wiki.spaces.get", argsSpace("space-a")) }()
	var id string
	select {
	case id = <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("executor never entered")
	}
	// 调用正在执行：四层计数（actor/client/app/global）都被占有。
	s.limitsMu.Lock()
	held := len(s.counts) == 4
	counts := fmt.Sprint(s.counts)
	s.limitsMu.Unlock()
	if !held {
		t.Fatalf("active call did not hold all four counters: %s", counts)
	}
	cancel()
	select {
	case <-observed:
	case <-time.After(5 * time.Second):
		t.Fatal("executor did not observe the cancellation")
	}
	var got Result
	select {
	case got = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled invocation did not return")
	}
	if got.InvocationID != id || got.Status != "cancelled" || got.Error == nil || got.Error.Kind != "cancelled" {
		t.Fatalf("unexpected receipt for cancelled invocation: %+v", got)
	}
	saved, err := s.Store.Record(state.Principal, id)
	if err != nil || saved.State != "cancelled" {
		t.Fatalf("cancelled receipt not stored: %+v / %v", saved, err)
	}
	if !waitForCountersEmpty(t, s, 5*time.Second) {
		s.limitsMu.Lock()
		leaked := fmt.Sprint(s.counts)
		s.limitsMu.Unlock()
		t.Fatalf("counters were not released after cancellation: %s", leaked)
	}
	assertCountersEmpty(t, s, nil)
}

// R08：额度已满时新 acquire 被拒且不产生释放副作用；占用中的计数保持。
func TestAcquireFailureDoesNotReleaseOccupiedCounters(t *testing.T) {
	c := requestBearerConfig(t)
	alice := c.Clients[0].Actors["feishu:user_id:alice"]
	alice.Resources["wiki_space"] = []string{"space-a", "space-b"}
	// 单独的小限额用例：actor 并发 1，无需改动其他请求的权限配置。
	alice.MaxConcurrent = 1
	c.Clients[0].Actors["feishu:user_id:alice"] = alice
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 1)
	proceed := make(chan struct{})
	f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, _ App) ExecutionResult {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-proceed
		return ExecutionResult{Data: map[string]any{}}
	}}
	s := startService(t, c, f)
	owner := "actor:" + actor("alice").Owner()
	state, err := c.authenticateRequest(&http.Request{Header: requestHeaders("limit-token")})
	if err != nil {
		t.Fatal(err)
	}
	// 直接占用一个 actor 名额并阻塞。
	hold := state.Principal
	if err := s.acquirePrincipal(hold); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { safeClose(proceed); s.releasePrincipal(hold) })
	s.limitsMu.Lock()
	if got := s.counts[owner]; got != 1 {
		s.limitsMu.Unlock()
		t.Fatalf("occupied counter = %d; want 1", got)
	}
	s.limitsMu.Unlock()
	// 第二次 acquire 必须以 concurrency_limit 失败。
	err = s.acquirePrincipal(hold)
	var faultPtr *Fault
	if !errors.As(err, &faultPtr) || faultPtr.Kind != "concurrency_limit" {
		t.Fatalf("second acquire error = %v; want concurrency_limit", err)
	}
	// 被拒的 acquire 不得产生释放副作用：占用计数保持 1。
	s.limitsMu.Lock()
	got := s.counts[owner]
	s.limitsMu.Unlock()
	if got != 1 {
		t.Fatalf("counter after rejected acquire = %d; want 1", got)
	}
	// 释放唯一占用后 key 删除。
	s.releasePrincipal(hold)
	assertCountersEmpty(t, s, map[string]bool{owner: true})
}

// R08：managed 后台 worker 完成与取消路径的计数释放（既有 TestQueuedJobsSkipBusyPrincipals
// 覆盖排队与撤权；此处断言 worker 执行/取消后计数归零删除）。
func TestManagedWorkerReleasesCountersAfterFinishAndCancel(t *testing.T) {
	c := testConfig(t)
	// managed 模式 + 异步任务由 worker 领取执行。
	started := make(chan struct{}, 1)
	proceed := make(chan struct{})
	f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, _ App) ExecutionResult {
		if in.Principal.Actor == "" {
			return ExecutionResult{Data: map[string]any{}}
		}
		select {
		case started <- struct{}{}:
		default:
		}
		<-proceed
		select {
		case <-ctx.Done():
			return ExecutionResult{Error: fault("cancelled", "worker fixture stopped")}
		default:
		}
		return ExecutionResult{Data: map[string]any{}}
	}}
	s := startService(t, c, f)
	t.Cleanup(func() { safeClose(proceed) })
	p := actor("alice")
	// 完成路径：异步任务执行完成后计数释放。
	args := argsSpace("space-a")
	args["async"] = true
	r := s.Invoke(context.Background(), p, "wiki.spaces.get", args)
	if r.Error != nil {
		t.Fatal(r.Error)
	}
	job := r.InvocationID
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("async job did not start")
	}
	safeClose(proceed)
	deadline := time.Now().Add(5 * time.Second)
	for {
		record, err := s.Store.Record(p, job)
		if err != nil {
			t.Fatal(err)
		}
		if record.State == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("async job stuck in %s", record.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 终态先于 release 写入：有界等待释放完成，再断言全空。
	if !waitForCountersEmpty(t, s, 5*time.Second) {
		s.limitsMu.Lock()
		leaked := fmt.Sprint(s.counts)
		s.limitsMu.Unlock()
		t.Fatalf("worker did not release counters after completion: %s", leaked)
	}
	assertCountersEmpty(t, s, nil)
	// 取消路径（T2）：先确认执行器已进入，再请求取消；用观察屏障证明执行器
	// 实际观察到 ctx.Done，回执必须是准确的 cancelled（不接受任意 failed）。
	proceed2 := make(chan struct{})
	workerEntered2 := make(chan struct{}, 1)
	workerObserved2 := make(chan struct{}, 1)
	// F4：第二条阻塞屏障也有独立清理，断言失败不挂起。
	t.Cleanup(func() { safeClose(proceed2) })
	var fn atomic.Value
	fn.Store(func(ctx context.Context, in Invocation, _ App) ExecutionResult {
		if in.Principal.Actor == "" {
			return ExecutionResult{Data: map[string]any{}}
		}
		select {
		case workerEntered2 <- struct{}{}:
		default:
		}
		<-proceed2
		<-ctx.Done()
		select {
		case workerObserved2 <- struct{}{}:
		default:
		}
		return ExecutionResult{Error: fault("cancelled", "worker executor observed cancellation")}
	})
	f.fn = func(ctx context.Context, in Invocation, a App) ExecutionResult {
		return fn.Load().(func(context.Context, Invocation, App) ExecutionResult)(ctx, in, a)
	}
	args2 := argsSpace("space-a")
	args2["async"] = true
	r2 := s.Invoke(context.Background(), p, "wiki.spaces.get", args2)
	if r2.Error != nil {
		t.Fatal(r2.Error)
	}
	job2 := r2.InvocationID
	// 屏障确认执行器已进入，避免取消尚未开始的任务。
	select {
	case <-workerEntered2:
	case <-time.After(5 * time.Second):
		t.Fatal("second async job never entered the executor")
	}
	if cr := s.Job(p, job2, true); cr.Error != nil {
		t.Fatalf("cancel failed: %+v", cr.Error)
	}
	// 放行执行器并确认其实际观察到取消。
	safeClose(proceed2)
	select {
	case <-workerObserved2:
	case <-time.After(5 * time.Second):
		t.Fatal("worker executor did not observe the cancellation")
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		record, err := s.Store.Record(p, job2)
		if err != nil {
			t.Fatal(err)
		}
		if record.State == "cancelled" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cancelled job ended in %s, want cancelled", record.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 取消路径同样在终态后释放：有界等待并断言。
	if !waitForCountersEmpty(t, s, 5*time.Second) {
		s.limitsMu.Lock()
		leaked := fmt.Sprint(s.counts)
		s.limitsMu.Unlock()
		t.Fatalf("worker did not release counters after cancellation: %s", leaked)
	}
	assertCountersEmpty(t, s, nil)
}

// safeClose 供测试清理路径二次触发时避免重复 close panic。
func safeClose(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

// waitForCountersEmpty 有界等待计数表清空：区分“数据库终态”与“worker 完成释放”。
// 终态先于 release 写入；直接断言会与释放竞争。失败时返回 false 由调用方断言。
func waitForCountersEmpty(t *testing.T, s *Service, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		s.limitsMu.Lock()
		empty := len(s.counts) == 0
		s.limitsMu.Unlock()
		if empty {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// assertCountersEmpty 断言并发计数表已完全清空。
// wanted=nil 表示“表必须为空”：任何残留条目（包括正数计数，即漏释放）都是失败。
// wanted 非 nil 时在空表检查外，额外点名必须删除的 key。
func assertCountersEmpty(t *testing.T, s *Service, wanted map[string]bool) {
	t.Helper()
	s.limitsMu.Lock()
	defer s.limitsMu.Unlock()
	if len(s.counts) != 0 {
		t.Errorf("concurrency counters not fully released: %v", s.counts)
	}
	for key := range wanted {
		if _, ok := s.counts[key]; ok {
			t.Errorf("counter %s still present after release", key)
		}
	}
}
