// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func retainedWriteArgs(key string) map[string]any {
	return map[string]any{"data": map[string]any{"folder_token": "folder-a", "name": key}, "idempotency_key": key}
}

func TestRetainedResultBlocksNewWorkUntilRecovery(t *testing.T) {
	c := testConfig(t)
	c.MaxFileBytes, c.MaxStorageBytes, c.MaxInlineBytes = 8192, 8192, 1024
	f := &fakeExecutor{fn: func(_ context.Context, in Invocation, _ App) ExecutionResult {
		if in.Operation.ID == "drive.files.create_folder" {
			return ExecutionResult{Data: map[string]any{"token": strings.Repeat("x", 4096)}}
		}
		return ExecutionResult{Data: map[string]any{"space": map[string]any{"name": "fixture"}}}
	}}
	s := startService(t, c, f)
	occupied, err := s.Store.PutFile(actor("alice"), "occupied.bin", "application/octet-stream", make([]byte, 6000))
	if err != nil {
		t.Fatal(err)
	}
	initial := s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", retainedWriteArgs("original-write"))
	if initial.Error == nil || initial.Error.Kind != "result_storage_failed" {
		t.Fatal("fixture did not retain", initial)
	}
	for _, who := range []string{"alice", "bob"} {
		for _, async := range []bool{false, true} {
			space := "space-a"
			if who == "bob" {
				space = "space-b"
			}
			args := argsSpace(space)
			args["async"] = async
			got := s.Invoke(context.Background(), actor(who), "wiki.spaces.get", args)
			if got.Error == nil || got.Error.Kind != "result_recovery_required" || got.InvocationID != "" {
				t.Fatalf("new call admitted: actor=%s async=%t result=%+v", who, async, got)
			}
		}
	}
	if got := s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", retainedWriteArgs("new-write")); got.Error == nil || got.Error.Kind != "result_recovery_required" {
		t.Fatal("new write admitted", got)
	}
	if got := s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", retainedWriteArgs("original-write")); got.InvocationID != initial.InvocationID || got.Error == nil || got.Error.Kind != "result_storage_failed" {
		t.Fatal("original key blocked", got)
	}
	conflict := retainedWriteArgs("original-write")
	object(conflict["data"])["name"] = "different"
	if got := s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", conflict); got.Error == nil || got.Error.Kind != "idempotency_conflict" {
		t.Fatal("conflicting key accepted", got)
	}
	var count int
	if err := s.Store.db.QueryRow(`SELECT COUNT(*) FROM invocations`).Scan(&count); err != nil || count != 1 || f.calls.Load() != 1 {
		t.Fatalf("blocked calls inserted or executed: records=%d calls=%d err=%v", count, f.calls.Load(), err)
	}
	desktop := Principal{ClientID: "desktop", AppID: "app-b", OrgID: "org-b"}
	if got := s.Invoke(context.Background(), desktop, "wiki.spaces.get", argsSpace("space-a")); got.Status != "succeeded" {
		t.Fatal("other client blocked", got)
	}
	if err := s.Store.DeleteFile(actor("alice"), occupied.ID); err != nil {
		t.Fatal(err)
	}
	if got := s.Job(actor("alice"), initial.InvocationID, false); got.Status != "succeeded" || len(got.Files) != 1 {
		t.Fatal("cannot recover", got)
	}
	if got := s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", argsSpace("space-a")); got.Status != "succeeded" {
		t.Fatal("recovery did not reopen admission", got)
	}
	if f.calls.Load() != 3 {
		t.Fatal("recovery executed original write", f.calls.Load())
	}
}

func TestRetainedResultQueueSkipsBlockedClient(t *testing.T) {
	c := testConfig(t)
	store, err := OpenStore(c)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	op := Operation{ID: "wiki.spaces.get"}
	reserve := func(p Principal, async bool) Record {
		t.Helper()
		r, _, err := store.Reserve(p, op, argsSpace("space-a"), "", async)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	running := reserve(actor("alice"), false)
	queued := reserve(actor("alice"), true)
	cancelled := reserve(actor("alice"), true)
	other := reserve(Principal{ClientID: "desktop", AppID: "app-b", OrgID: "org-b"}, true)
	original := running.Result
	original.Status, original.Data = "succeeded", strings.Repeat("x", 4096)
	failure := original
	failure.Status, failure.Data, failure.Error = "failed", nil, fault("result_storage_failed", "fixture")
	if err := store.Finish(running.ID, failure, &original); err != nil {
		t.Fatal(err)
	}
	ready := func(p Principal) bool {
		if p.ClientID == "wake" {
			t.Fatal("blocked client reached execution admission")
		}
		return true
	}
	claimed, err := store.Claim(ready)
	if err != nil || claimed.ID != other.ID {
		t.Fatal("other client starved", claimed, err)
	}
	if _, err := store.Claim(ready); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("blocked queue was claimed", err)
	}
	if r, err := store.Cancel(actor("alice"), cancelled.ID); err != nil || r.Status != "cancelled" {
		t.Fatal("blocked queue cannot be cancelled", r, err)
	}
	if err := store.SaveRecoveredResult(actor("alice"), running.ID, original); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.Claim(func(Principal) bool { return true })
	if err != nil || claimed.ID != queued.ID {
		t.Fatal("recovered queue did not resume", claimed, err)
	}
	// 无人查询回执时，调度也应主动释放已经过期的积压。
	afterExpiry := reserve(actor("alice"), true)
	original.InvocationID, failure.InvocationID = claimed.ID, claimed.ID
	if err := store.Finish(claimed.ID, failure, &original); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE invocations SET result=json_set(result,'$.retained_until',1) WHERE id=?`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.Claim(func(Principal) bool { return true })
	if err != nil || claimed.ID != afterExpiry.ID {
		t.Fatal("expired retention blocked the queue", claimed, err)
	}
}

func TestRetainedResultPreservesAlreadyRunningCalls(t *testing.T) {
	c := testConfig(t)
	c.MaxFileBytes, c.MaxStorageBytes, c.MaxInlineBytes = 8192, 8192, 1024
	started, release := make(chan struct{}, 3), make(chan struct{})
	f := &fakeExecutor{fn: func(ctx context.Context, in Invocation, _ App) ExecutionResult {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return ExecutionResult{Error: fault("cancelled", "fixture shutdown")}
		}
		return ExecutionResult{Data: map[string]any{"token": strings.Repeat("x", 4096), "name": object(in.Arguments["data"])["name"]}}
	}}
	s := startService(t, c, f)
	if _, err := s.Store.PutFile(actor("alice"), "occupied.bin", "application/octet-stream", make([]byte, 6000)); err != nil {
		t.Fatal(err)
	}
	results := make(chan Result, 3)
	for _, key := range []string{"first", "second", "third"} {
		go func() {
			results <- s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", retainedWriteArgs(key))
		}()
	}
	for range 3 {
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent calls did not start")
		}
	}
	close(release)
	seen := map[string]bool{}
	for range 3 {
		select {
		case result := <-results:
			if result.Error == nil || result.Error.Kind != "result_storage_failed" {
				t.Fatal("running call did not retain", result)
			}
			record, err := s.Store.Record(actor("alice"), result.InvocationID)
			if err != nil || record.RetainedResult == nil {
				t.Fatal("running result lost", err)
			}
			seen[object(record.RetainedResult.Data)["name"].(string)] = true
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent calls did not finish")
		}
	}
	if len(seen) != 3 || f.calls.Load() != 3 {
		t.Fatal("in-flight result missing", seen, f.calls.Load())
	}
	if result := s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", retainedWriteArgs("fourth")); result.Error == nil || result.Error.Kind != "result_recovery_required" {
		t.Fatal("new call admitted after retention", result)
	}
}

func TestRetainedResultExpiryKeepsReceiptAndIdempotency(t *testing.T) {
	for _, trigger := range []string{"collect", "reservation", "job", "restart"} {
		t.Run(trigger, func(t *testing.T) {
			c := testConfig(t)
			c.MaxFileBytes, c.MaxStorageBytes, c.MaxInlineBytes = 8192, 8192, 1024
			f := &fakeExecutor{fn: func(_ context.Context, in Invocation, _ App) ExecutionResult {
				if in.Operation.ID == "drive.files.create_folder" {
					return ExecutionResult{Data: map[string]any{"token": strings.Repeat("x", 4096)}}
				}
				return ExecutionResult{Data: map[string]any{"space": map[string]any{"name": "fixture"}}}
			}}
			s, err := NewService(c, f)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if s != nil {
					s.Close()
				}
			}()
			if _, err := s.Store.PutFile(actor("alice"), "occupied.bin", "application/octet-stream", make([]byte, 6000)); err != nil {
				t.Fatal(err)
			}
			initial := s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", retainedWriteArgs("original-write"))
			record, err := s.Store.Record(actor("alice"), initial.InvocationID)
			if err != nil || record.RetainedResult == nil || record.RetainedUntil <= time.Now().Unix() {
				t.Fatal("retention deadline missing", err)
			}
			// 用真实的过期记录触发清理，不靠等待 TTL 或改变测试机时间。
			if _, err := s.Store.db.Exec(`UPDATE invocations SET result=json_set(result,'$.retained_until',1) WHERE id=?`, initial.InvocationID); err != nil {
				t.Fatal(err)
			}
			switch trigger {
			case "collect":
				err = s.Store.Collect()
			case "reservation":
				got := s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", argsSpace("space-a"))
				if got.Status != "succeeded" {
					t.Fatal("expired result blocked new work", got)
				}
			case "job":
				got := s.Job(actor("alice"), initial.InvocationID, false)
				if got.Error == nil || got.Error.Kind != "result_expired" {
					t.Fatal("expired result recovered", got)
				}
			case "restart":
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				s = nil
				s, err = NewService(c, f)
			}
			if err != nil {
				t.Fatal(err)
			}
			record, err = s.Store.Record(actor("alice"), initial.InvocationID)
			if err != nil || record.RetainedResult != nil || record.RetainedUntil != 0 || record.State != initial.Status || record.Result.Meta["execution_status"] != "succeeded" || record.Result.Meta["result_retained"] != false || record.Result.Error == nil || record.Result.Error.Kind != "result_expired" {
				t.Fatalf("expiry damaged receipt: %+v %v", record, err)
			}
			before := f.calls.Load()
			got := s.Invoke(context.Background(), actor("alice"), "drive.files.create_folder", retainedWriteArgs("original-write"))
			if got.InvocationID != initial.InvocationID || got.Error == nil || got.Error.Kind != "result_expired" || f.calls.Load() != before {
				t.Fatal("expired write replayed", got, f.calls.Load())
			}
			if got := s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", argsSpace("space-a")); got.Status != "succeeded" {
				t.Fatal("expiry did not release admission", got)
			}
		})
	}
}

func TestRetainedResultBackfillsLegacyDeadline(t *testing.T) {
	c := testConfig(t)
	store, err := OpenStore(c)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if store != nil {
			store.Close()
		}
	}()
	updated := time.Now().Unix()
	ids := []string{}
	for range 2 {
		r, _, err := store.Reserve(actor("alice"), Operation{ID: "wiki.spaces.get"}, argsSpace("space-a"), "", false)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	for i, stamp := range []int64{updated, 1} {
		// 模拟上一版已落库、未包含 retained_until 的完整结果。
		result := storedResult{Result: Result{InvocationID: ids[i], Status: "failed", Meta: map[string]any{"execution_status": "succeeded"}}, RetainedResult: &Result{Status: "succeeded", Data: strings.Repeat("x", 4096)}}
		if _, err := store.db.Exec(`UPDATE invocations SET state='failed',result=?,updated=? WHERE id=?`, string(encode(result)), stamp, ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = nil
	store, err = OpenStore(c)
	if err != nil {
		t.Fatal(err)
	}
	live, err := store.Record(actor("alice"), ids[0])
	if err != nil || live.RetainedResult == nil || live.RetainedUntil != updated+c.FileTTL {
		t.Fatal("legacy deadline not derived from update time", live.RetainedUntil, err)
	}
	expired, err := store.Record(actor("alice"), ids[1])
	if err != nil || expired.RetainedResult != nil || expired.Result.Error == nil || expired.Result.Error.Kind != "result_expired" {
		t.Fatal("legacy expired body retained", err)
	}
}
