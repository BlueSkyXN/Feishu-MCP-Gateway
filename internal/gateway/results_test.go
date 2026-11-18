// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestResultArchiveRecoveryPreservesOutcome(t *testing.T) {
	for _, tc := range []struct {
		name, operation, status string
		async                   bool
		problem                 *Fault
		uncertain               bool
	}{
		{name: "read", operation: "wiki.spaces.get", status: "succeeded"},
		{name: "write", operation: "drive.files.create_folder", status: "succeeded"},
		{name: "queued-write", operation: "drive.files.create_folder", status: "succeeded", async: true},
		{name: "partial", operation: "drive.files.create_folder", status: "partial", problem: fault("partial_failure", "fixture partial")},
		{name: "unknown", operation: "drive.files.create_folder", status: "unknown", problem: fault("upstream_transport", "fixture uncertainty"), uncertain: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testConfig(t)
			c.MaxFileBytes, c.MaxStorageBytes, c.MaxInlineBytes = 8192, 8192, 2048
			var s *Service
			data := map[string]any{"token": "created-folder", "space": map[string]any{"name": strings.Repeat("x", 4096)}}
			meta := map[string]any{"fixture": "original metadata"}
			f := &fakeExecutor{fn: func(_ context.Context, in Invocation, _ App) ExecutionResult {
				file, err := s.Store.PutFile(in.Principal, "artifact.txt", "text/plain", []byte("already archived"), in.ID)
				if err != nil {
					return ExecutionResult{Error: asFault(err)}
				}
				return ExecutionResult{Data: data, Meta: meta, Files: []File{file}, Error: tc.problem, Uncertain: tc.uncertain}
			}}
			var err error
			s, err = NewService(c, f)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if s != nil {
					s.Close()
				}
			}()
			occupied, err := s.Store.PutFile(actor("alice"), "occupied.bin", "application/octet-stream", make([]byte, 6000))
			if err != nil {
				t.Fatal(err)
			}
			args := argsSpace("space-a")
			if tc.operation == "drive.files.create_folder" {
				args = map[string]any{"data": map[string]any{"folder_token": "folder-a", "name": "fixture"}, "idempotency_key": "same-write", "async": tc.async}
			}
			r := s.Invoke(context.Background(), actor("alice"), tc.operation, args)
			id := r.InvocationID
			if tc.async {
				deadline := time.Now().Add(10 * time.Second)
				for r.Status == "queued" || r.Status == "running" {
					if time.Now().After(deadline) {
						t.Fatal("worker did not finish")
					}
					time.Sleep(10 * time.Millisecond)
					r = s.Job(actor("alice"), id, false)
				}
			}
			wantStatus := tc.status
			if wantStatus == "succeeded" {
				wantStatus = "failed"
			}
			if r.Status != wantStatus || r.Error == nil || r.Error.Kind != "result_storage_failed" || r.Data != nil || r.Meta["result_spilled"] != false || r.Meta["execution_status"] != tc.status || len(r.Files) != 1 || len(encode(r)) > c.MaxInlineBytes {
				t.Fatalf("incorrect delivery failure: %+v", r)
			}
			artifact := r.Files[0].ID
			record, err := s.Store.Record(actor("alice"), id)
			if err != nil || record.State != wantStatus || record.RetainedResult == nil || record.RetainedResult.Status != tc.status || string(encode(record.RetainedResult.Data)) != string(encode(data)) || string(encode(record.RetainedResult.Meta)) != string(encode(meta)) {
				t.Fatalf("complete result not retained: %+v %v", record, err)
			}
			if r := s.Job(actor("bob"), id, false); r.Error == nil || r.Error.Kind != "not_found" {
				t.Fatal("cross-actor receipt access", r)
			}
			if tc.operation == "drive.files.create_folder" {
				retried := s.Invoke(context.Background(), actor("alice"), tc.operation, args)
				if retried.InvocationID != id || retried.Error == nil || retried.Error.Kind != "result_storage_failed" {
					t.Fatal("same-key result changed", retried)
				}
			}
			if f.calls.Load() != 1 {
				t.Fatal("execution repeated before recovery")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s = nil
			afterRestart := &fakeExecutor{}
			s, err = NewService(c, afterRestart)
			if err != nil {
				t.Fatal(err)
			}
			if r := s.Job(actor("alice"), id, false); r.Error == nil || r.Error.Kind != "result_storage_failed" {
				t.Fatal("restart lost pending result", r)
			}
			if err := s.Store.DeleteFile(actor("alice"), occupied.ID); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			results := make(chan Result, 4)
			for range 4 {
				wg.Add(1)
				go func() { defer wg.Done(); results <- s.Job(actor("alice"), id, false) }()
			}
			wg.Wait()
			close(results)
			fileID := ""
			for recovered := range results {
				if recovered.Status != tc.status || string(encode(recovered.Error)) != string(encode(tc.problem)) || len(recovered.Files) != 1 || recovered.Meta["result_spilled"] != true {
					t.Fatalf("recovered outcome: %+v", recovered)
				}
				if fileID != "" && recovered.Files[0].ID != fileID {
					t.Fatal("concurrent recovery created duplicate files")
				}
				fileID = recovered.Files[0].ID
			}
			_, raw, err := s.Store.ReadFile(actor("alice"), fileID, 0, 8192)
			if err != nil {
				t.Fatal(err)
			}
			var original Result
			if err := json.Unmarshal(raw, &original); err != nil {
				t.Fatal(err)
			}
			if original.Status != tc.status || original.InvocationID != id || string(encode(original.Data)) != string(encode(data)) || string(encode(original.Meta)) != string(encode(meta)) || string(encode(original.Error)) != string(encode(tc.problem)) || len(original.Files) != 1 || original.Files[0].ID != artifact {
				t.Fatal("original receipt changed during recovery")
			}
			stored, err := s.Store.Record(actor("alice"), id)
			if err != nil || stored.RetainedResult != nil || stored.State != tc.status {
				t.Fatal("recovery did not replace retained result", err)
			}
			files, err := s.Store.ListFiles(actor("alice"), "", 100)
			if err != nil || len(files) != 2 {
				t.Fatal("recovery leaked files", files, err)
			}
			if tc.operation == "drive.files.create_folder" {
				if retried := s.Invoke(context.Background(), actor("alice"), tc.operation, args); retried.InvocationID != id || len(retried.Files) != 1 || retried.Files[0].ID != fileID {
					t.Fatal("idempotent retry lost recovered result", retried)
				}
			}
			if afterRestart.calls.Load() != 0 {
				t.Fatal("recovery executed an upstream operation")
			}
		})
	}
}

func TestResultRecoveryRechecksPolicyAndKeepsDataAfterStorageError(t *testing.T) {
	c := testConfig(t)
	c.MaxFileBytes, c.MaxStorageBytes, c.MaxInlineBytes = 8192, 8192, 1024
	f := &fakeExecutor{fn: func(context.Context, Invocation, App) ExecutionResult {
		return ExecutionResult{Data: map[string]any{"space": map[string]any{"name": strings.Repeat("x", 4096)}}}
	}}
	s := startService(t, c, f)
	occupied, err := s.Store.PutFile(actor("alice"), "occupied.bin", "application/octet-stream", make([]byte, 6000))
	if err != nil {
		t.Fatal(err)
	}
	r := s.Invoke(context.Background(), actor("alice"), "wiki.spaces.get", argsSpace("space-a"))
	if r.Error == nil || r.Error.Kind != "result_storage_failed" {
		t.Fatal(r)
	}
	if err := s.Store.DeleteFile(actor("alice"), occupied.ID); err != nil {
		t.Fatal(err)
	}
	revoked := cloneConfigValue(c)
	policy := revoked.Clients[0].Actors["feishu:user_id:alice"]
	policy.Operations = []string{"wiki.nodes.list"}
	revoked.Clients[0].Actors["feishu:user_id:alice"] = policy
	if err := s.Reload(revoked); err != nil {
		t.Fatal(err)
	}
	if got := s.Job(actor("alice"), r.InvocationID, false); got.Error == nil || got.Error.Kind != "operation_denied" {
		t.Fatal("revoked recovery allowed", got)
	}
	if err := s.Reload(c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.db.Exec(`CREATE TRIGGER reject_recovery BEFORE UPDATE ON invocations WHEN json_type(OLD.result,'$.retained_result')='object' BEGIN SELECT RAISE(ABORT,'fixture storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if got := s.Job(actor("alice"), r.InvocationID, false); got.Error == nil || got.Error.Kind != "receipt_storage_failed" {
		t.Fatal("failed recovery update accepted", got)
	}
	stored, err := s.Store.Record(actor("alice"), r.InvocationID)
	if err != nil || stored.RetainedResult == nil {
		t.Fatal("storage error lost complete result", err)
	}
	files, err := s.Store.ListFiles(actor("alice"), "", 100)
	if err != nil || len(files) != 0 {
		t.Fatal("unsaved recovery leaked result file", files, err)
	}
	if _, err := s.Store.db.Exec(`DROP TRIGGER reject_recovery`); err != nil {
		t.Fatal(err)
	}
	if got := s.Job(actor("alice"), r.InvocationID, false); got.Status != "succeeded" || got.Error != nil {
		t.Fatal("recovery did not resume", got)
	}
	if f.calls.Load() != 1 {
		t.Fatal("recovery repeated the operation")
	}
}

func TestCursorStorageFailureDoesNotReportSuccess(t *testing.T) {
	s := startService(t, testConfig(t), &fakeExecutor{fn: func(context.Context, Invocation, App) ExecutionResult {
		return ExecutionResult{Data: map[string]any{"items": []any{}, "page_token": "fixture-upstream-cursor"}}
	}})
	if _, err := s.Store.db.Exec(`CREATE TRIGGER reject_cursor BEFORE INSERT ON cursors BEGIN SELECT RAISE(ABORT,'fixture cursor failure'); END`); err != nil {
		t.Fatal(err)
	}
	r := s.Invoke(context.Background(), actor("alice"), "wiki.nodes.list", argsSpace("space-a"))
	if r.Status != "failed" || r.Error == nil || r.Error.Kind != "storage_failed" {
		t.Fatal("cursor failure reported success", r)
	}
	if strings.Contains(string(encode(r)), "fixture-upstream-cursor") {
		t.Fatal("raw upstream cursor was returned")
	}
}
