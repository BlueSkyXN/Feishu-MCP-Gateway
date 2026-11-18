// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestEventRevocationCoversReceiptsAndArtifacts(t *testing.T) {
	const typ = "im.message.receive_v1"
	const marker = "fixture-event-content-"
	for _, layer := range []string{"app", "client", "actor"} {
		for _, mode := range []string{"inline", "spilled", "retained"} {
			t.Run(layer+"/"+mode, func(t *testing.T) {
				c := testConfig(t)
				t.Setenv("TEST_EVENT_KEY", "fixture-event-key")
				t.Setenv("TEST_EVENT_TOKEN", "fixture-event-token")
				c.Apps[0].EventEncryptKeyEnv, c.Apps[0].EventVerifyTokenEnv = "TEST_EVENT_KEY", "TEST_EVENT_TOKEN"
				c.Apps[0].EventTypes = []string{typ}
				c.Clients[0].Packs = []string{"events.read"}
				c.Clients[0].ResourceMode = "application_scope"
				c.Clients[0].EventTypes = []string{typ}
				c.Clients[0].Actors = map[string]ActorPolicy{actor("alice").Actor: {EventTypes: []string{typ}}}
				c.Clients[0].AllowTransferTickets = true
				c.MaxFileBytes, c.MaxStorageBytes = 8192, 8192
				if mode != "inline" {
					c.MaxInlineBytes = 1024
				}
				s := startService(t, c, &fakeExecutor{})
				payload := map[string]any{"message": map[string]any{"text": marker + strings.Repeat("x", 4096)}}
				if _, err := s.Store.db.Exec(`INSERT INTO events(app,event_id,event_type,payload,created) VALUES(?,?,?,?,?)`, "app-a", "fixture-event", typ, string(encode(payload)), time.Now().Unix()); err != nil {
					t.Fatal(err)
				}
				var occupied File
				if mode == "retained" {
					var err error
					occupied, err = s.Store.PutFile(actor("alice"), "occupied.bin", "application/octet-stream", make([]byte, 6000))
					if err != nil {
						t.Fatal(err)
					}
				}
				server := httptest.NewServer(s.Handler())
				defer server.Close()
				session, transport := connect(t, server.URL, "2025-11-25", "fixture-key", "alice")
				args := map[string]any{"params": map[string]any{"event_type": typ}}
				initial := call(t, session, "feishu_events_read", args)
				if mode == "retained" {
					if initial.Error == nil || initial.Error.Kind != "result_storage_failed" {
						t.Fatal("fixture did not retain", initial)
					}
				} else if initial.Error != nil {
					t.Fatal(initial)
				}
				var ticket Transfer
				if mode == "spilled" {
					if len(initial.Files) != 1 {
						t.Fatal("fixture did not archive", initial)
					}
					var err error
					ticket, err = s.issueTransfer(actor("alice"), map[string]any{"direction": "download", "file_id": initial.Files[0].ID})
					if err != nil {
						t.Fatal(err)
					}
				}
				next := cloneConfigValue(c)
				switch layer {
				case "app":
					next.Apps[0].EventTypes = nil
				case "client":
					next.Clients[0].EventTypes = nil
				case "actor":
					next.Clients[0].Actors[actor("alice").Actor] = ActorPolicy{}
				}
				if err := s.Reload(next); err != nil {
					t.Fatal(err)
				}
				if mode == "retained" {
					if err := s.Store.DeleteFile(actor("alice"), occupied.ID); err != nil {
						t.Fatal(err)
					}
				}
				for _, got := range []Result{
					call(t, session, "feishu_events_read", args),
					call(t, session, "feishu_job_get", map[string]any{"job_id": initial.InvocationID}),
					call(t, session, "feishu_job_cancel", map[string]any{"job_id": initial.InvocationID}),
				} {
					if got.Error == nil || got.Error.Kind != "event_type_denied" || strings.Contains(string(encode(got)), marker) {
						t.Fatal("revoked event accessible", got)
					}
				}
				if mode == "retained" {
					record, err := s.Store.Record(actor("alice"), initial.InvocationID)
					if err != nil || record.RetainedResult == nil {
						t.Fatal("revoked recovery changed pending result", err)
					}
					files, err := s.Store.ListFiles(actor("alice"), "", 100)
					if err != nil || len(files) != 0 {
						t.Fatal("revoked recovery archived a file", files, err)
					}
				}
				if mode == "spilled" {
					file := initial.Files[0]
					got := call(t, session, "feishu_file_read", map[string]any{"file_id": file.ID, "limit": 8192})
					if got.Error == nil || got.Error.Kind != "event_type_denied" {
						t.Fatal("revoked file tool allowed", got)
					}
					if _, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: file.URI}); err == nil || !strings.Contains(err.Error(), "event_type_denied") {
						t.Fatal("revoked resource did not check event type", err)
					}
					if _, err := s.issueTransfer(actor("alice"), map[string]any{"direction": "download", "file_id": file.ID}); err == nil || asFault(err).Kind != "event_type_denied" {
						t.Fatal("new ticket allowed", err)
					}
					for _, path := range []string{"/files/" + file.ID, ticket.Path} {
						req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
						if err != nil {
							t.Fatal(err)
						}
						client := &http.Client{Transport: transport}
						if path == ticket.Path {
							req.Header.Set("Authorization", "Bearer "+ticket.Ticket)
							client = http.DefaultClient
						}
						response, err := client.Do(req)
						if err != nil {
							t.Fatal(err)
						}
						body, err := io.ReadAll(response.Body)
						response.Body.Close()
						if err != nil || response.StatusCode < 400 || strings.Contains(string(body), marker) {
							t.Fatalf("revoked download %s: %d %v", path, response.StatusCode, err)
						}
					}
				}
				if err := s.Reload(c); err != nil {
					t.Fatal(err)
				}
				recovered := call(t, session, "feishu_job_get", map[string]any{"job_id": initial.InvocationID})
				if recovered.Error != nil || recovered.Status != "succeeded" {
					t.Fatal("restored authorization cannot retrieve result", recovered)
				}
				if mode == "inline" {
					if !strings.Contains(string(encode(recovered.Data)), marker) {
						t.Fatal("restored receipt lost payload")
					}
				} else {
					if len(recovered.Files) != 1 {
						t.Fatal("restored result missing archive", recovered)
					}
					file := call(t, session, "feishu_file_read", map[string]any{"file_id": recovered.Files[0].ID, "limit": 8192})
					if file.Error != nil || !strings.Contains(string(encode(file.Data)), marker) {
						t.Fatal("restored file lost payload", file)
					}
				}
			})
		}
	}
}
