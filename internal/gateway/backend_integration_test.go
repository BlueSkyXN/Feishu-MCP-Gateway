// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jsonResponse(r *http.Request, status int, value any) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(encode(value)))), Request: r}
}

func realBackend(t *testing.T, handler roundTripFunc) (*Service, *CLIExecutor) {
	t.Helper()
	binary := os.Getenv("GATEWAY_BACKEND_BIN")
	if binary == "" {
		binary = "../../bin/lark-cli-gateway"
	}
	binary, _ = filepath.Abs(binary)
	b, err := os.ReadFile(binary)
	if os.IsNotExist(err) {
		t.Skip("build the pinned wrapper with scripts/build_backend.py to run backend integration")
	}
	if err != nil {
		t.Fatal(err)
	}
	c := testConfig(t)
	c.CLIBinary = binary
	c.CLISHA256 = digest(b)
	for i := range c.Apps {
		c.Apps[i].DPoPMode = "disabled"
	}
	c.Clients[0].ResourceMode = "application_scope"
	c.Clients[0].Actors = map[string]ActorPolicy{"feishu:user_id:alice": {}, "feishu:user_id:bob": {}}
	c.Clients[0].Packs = []string{"docs.read", "docs.write", "drive.read", "drive.write", "im.read", "im.write", "base.read", "base.write", "wiki.read", "sheets.read", "sheets.write"}
	e, err := NewCLIExecutor(c)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewService(c, e)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	e.broker = NewBroker(c.MaxFileBytes, &http.Client{Transport: handler}, func(in Invocation) (App, error) {
		return authorize(s.currentConfig(), in.Principal, in.Operation, in.Arguments)
	})
	return s, e
}
func TestRealBackendMCPAppOnlyFilesAndShortcuts(t *testing.T) {
	var mu sync.Mutex
	requests := []string{}
	issues := 0
	s, _ := realBackend(t, func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, r.Method+" "+r.URL.String())
		if r.URL.Path == "/oauth/v3/token" {
			issues++
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "cli_fixture_a" {
				t.Error("wrong token identity")
			}
			return jsonResponse(r, 200, map[string]any{"access_token": "fixture-tat", "expires_in": 7200, "token_type": "Bearer"}), nil
		}
		if r.Header.Get("Authorization") != "Bearer fixture-tat" {
			t.Errorf("missing broker token: %s", r.URL.Path)
		}
		var data any
		switch r.URL.Path {
		case "/open-apis/wiki/v2/spaces/space-a":
			data = map[string]any{"space": map[string]any{"space_id": "space-a"}}
		case "/open-apis/docs_ai/v1/documents/docFixture/fetch":
			data = map[string]any{"document": map[string]any{"document_id": "docFixture", "content": "<p>fixture</p>"}}
		case "/open-apis/docs_ai/v1/documents":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if !strings.Contains(body["content"].(string), "from file") {
				t.Error("input file not used")
			}
			data = map[string]any{"document": map[string]any{"document_id": "newDocFixture", "url": "https://example.feishu.cn/docx/newDocFixture"}}
		case "/open-apis/docs_ai/v1/documents/docFixture":
			if r.Method != "PUT" {
				t.Error("update method")
			}
			data = map[string]any{"revision_id": 2}
		case "/open-apis/sheet_ai/v2/spreadsheets/shtFixture/tools/invoke_read":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["tool_name"] != "get_workbook_structure" {
				t.Error("wrong sheet read tool")
			}
			data = map[string]any{"output": `{"sheets":[{"sheet_id":"sheetA","title":"Sheet1"}]}`}
		case "/open-apis/sheet_ai/v2/spreadsheets/shtFixture/tools/invoke_write":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["tool_name"] != "set_cell_range" {
				t.Error("wrong sheet write tool")
			}
			data = map[string]any{"output": `{"updated_cells":1}`}
		case "/open-apis/base/v3/bases/app_fixture/tables/tbl_fixture/records":
			data = map[string]any{"records": []any{}, "has_more": false}
		case "/open-apis/base/v3/bases/app_fixture/tables/tbl_fixture/records/batch_create":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if len(body["create_records"].([]any)) != 1 {
				t.Error("base JSON was not mapped")
			}
			data = map[string]any{"record_id_list": []string{"rec_fixture"}}
		case "/open-apis/drive/v1/export_tasks":
			data = map[string]any{"ticket": "exportFixture"}
		case "/open-apis/drive/v1/export_tasks/exportFixture":
			data = map[string]any{"result": map[string]any{"job_status": 0, "file_token": "exportFile", "file_name": "export.pdf", "file_extension": "pdf"}}
		case "/open-apis/drive/v1/export_tasks/file/exportFile/download":
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/pdf"}}, Body: io.NopCloser(strings.NewReader("export fixture")), Request: r}, nil
		case "/open-apis/drive/v2/files/query_by_token":
			data = map[string]any{"obj_token": "fileFixture", "obj_type": "file", "status": "normal", "is_wiki_token": false}
		case "/open-apis/drive/v1/permissions/fileFixture/members/auth":
			data = map[string]any{"auth_result": true}
		case "/open-apis/drive/v1/files/fileFixture/download":
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/octet-stream"}, "Content-Disposition": {"attachment; filename=result.bin"}}, Body: io.NopCloser(strings.NewReader("downloaded fixture")), Request: r}, nil
		case "/open-apis/im/v1/images":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			f, _, err := r.FormFile("image")
			if err != nil {
				t.Error(err)
			} else {
				b, _ := io.ReadAll(f)
				f.Close()
				if string(b) != "image fixture" {
					t.Error("multipart file mismatch")
				}
			}
			data = map[string]any{"image_key": "img_fixture"}
		default:
			t.Errorf("unexpected upstream request %s %s", r.Method, r.URL.Path)
			data = map[string]any{}
		}
		return jsonResponse(r, 200, map[string]any{"code": 0, "msg": "success", "data": data}), nil
	})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	session, _ := connect(t, server.URL, "2026-07-28", "fixture-key", "alice")
	ctx := context.Background()
	cases := []struct {
		op   string
		args map[string]any
	}{{"wiki.spaces.get", argsSpace("space-a")}, {"docs.+fetch", map[string]any{"doc": "docFixture"}}}
	for _, tc := range cases {
		op, _ := s.Catalog.Operation(tc.op)
		r := call(t, session, op.Tool, tc.args)
		if r.Status != "succeeded" {
			t.Fatalf("%s: %s", tc.op, encode(r))
		}
	}
	content, err := s.Store.PutFile(actor("alice"), "content.md", "text/markdown", []byte("# from file"))
	if err != nil {
		t.Fatal(err)
	}
	r := s.Invoke(ctx, actor("alice"), "docs.+create", map[string]any{"content": map[string]any{"file_id": content.ID}, "doc_format": "markdown", "idempotency_key": "create-fixture"})
	if r.Status != "succeeded" {
		t.Fatalf("docs create: %s", encode(r))
	}
	input, err := s.Store.PutFile(actor("alice"), "image.png", "image/png", []byte("image fixture"))
	if err != nil {
		t.Fatal(err)
	}
	r = s.Invoke(ctx, actor("alice"), "im.images.create", map[string]any{"data": map[string]any{"image_type": "message"}, "files": map[string]any{"image": map[string]any{"file_id": input.ID}}, "idempotency_key": "upload-fixture"})
	if r.Status != "succeeded" {
		t.Fatalf("image upload: %s", encode(r))
	}
	r = s.Invoke(ctx, actor("alice"), "drive.+download", map[string]any{"file_token": "fileFixture", "output": "result.bin"})
	if r.Status != "succeeded" {
		t.Fatalf("download: %s", encode(r))
	}
	if len(r.Files) != 1 {
		t.Fatalf("missing download artifact: %s", encode(r))
	}
	_, b, err := s.Store.ReadFile(actor("alice"), r.Files[0].ID, 0, 100)
	if err != nil || string(b) != "downloaded fixture" {
		t.Fatalf("download content %q %v", b, err)
	}
	if _, err = s.AuthorizeFile(actor("bob"), r.Files[0].ID); err == nil {
		t.Fatal("cross actor artifact access")
	}
	for _, tc := range []struct {
		op   string
		args map[string]any
	}{
		{"docs.+update", map[string]any{"doc": "docFixture", "command": "append", "content": "<p>updated</p>", "idempotency_key": "update"}},
		{"base.+record-list", map[string]any{"base_token": "app_fixture", "table_id": "tbl_fixture"}},
		{"base.+record-batch-create", map[string]any{"base_token": "app_fixture", "table_id": "tbl_fixture", "json": map[string]any{"create_records": []any{map[string]any{"Name": "fixture"}}}, "idempotency_key": "base-create"}},
		{"sheets.+workbook-info", map[string]any{"spreadsheet_token": "shtFixture"}},
		{"sheets.+cells-set", map[string]any{"spreadsheet_token": "shtFixture", "sheet_id": "sheetA", "range": "A1", "cells": []any{[]any{map[string]any{"value": "fixture"}}}, "idempotency_key": "sheet-write"}},
		{"drive.+export", map[string]any{"token": "docFixture", "doc_type": "docx", "file_extension": "pdf", "output_dir": "exports"}},
	} {
		r := s.Invoke(ctx, actor("alice"), tc.op, tc.args)
		if r.Status != "succeeded" {
			t.Fatalf("%s: %s", tc.op, encode(r))
		}
		if tc.op == "drive.+export" && len(r.Files) != 1 {
			t.Fatalf("export files: %s", encode(r))
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if issues != 1 {
		t.Fatalf("token cache did not coalesce sequential calls: %d; %v", issues, requests)
	}
}
