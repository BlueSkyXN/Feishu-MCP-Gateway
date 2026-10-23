// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"errors"
	"github.com/larksuite/cli/extension/fileio"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type brokenReader struct{ read bool }

func (b *brokenReader) Read(p []byte) (int, error) {
	if !b.read {
		b.read = true
		return copy(p, "partial"), nil
	}
	return 0, errors.New("fixture interruption")
}
func TestRootedFileIOAtomicLimitsAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	r := &rootedFiles{root: root, path: dir, limit: 10, total: 30}
	if _, err = r.Save("out.txt", fileio.SaveOptions{}, strings.NewReader("original")); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Save("out.txt", fileio.SaveOptions{}, &brokenReader{}); err == nil {
		t.Fatal("interrupted overwrite accepted")
	}
	f, _ := r.Open("out.txt")
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "original" {
		t.Fatal("failed write destroyed prior file")
	}
	if _, err = r.SaveExclusive("out.txt", fileio.SaveOptions{}, strings.NewReader("new")); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	if _, err = r.Save("large.txt", fileio.SaveOptions{}, strings.NewReader(strings.Repeat("a", 11))); err == nil {
		t.Fatal("oversize file")
	}
	if _, err = r.Open("../outside"); err == nil {
		t.Fatal("parent traversal accepted")
	}
	outside := t.TempDir()
	os.WriteFile(outside+"/secret", []byte("fixture"), 0600)
	os.Symlink(outside, dir+"/link")
	if _, err = r.Open("link/secret"); err == nil {
		t.Fatal("symlink escaped root")
	}
	if _, err = r.Save("link/new", fileio.SaveOptions{}, strings.NewReader("x")); err == nil {
		t.Fatal("symlink write escaped root")
	}
}
func TestBackendTransportRejectsUserAndRoutesAllRequests(t *testing.T) {
	g := &gatewayInterceptor{address: "http://127.0.0.1:12345", key: "fixture"}
	r, _ := http.NewRequestWithContext(context.Background(), "POST", "https://open.feishu.cn/open-apis/test", strings.NewReader("body"))
	r.Header.Set("Authorization", "Bearer sidecar-managed-tat")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Cookie", "fixture")
	if _, err := g.PreRoundTripE(r); err != nil {
		t.Fatal(err)
	}
	if r.URL.Host != "127.0.0.1:12345" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Lark-Proxy-Identity") != "bot" || r.Header.Get("X-Lark-Proxy-Signature") == "" {
		t.Fatal("invalid proxy rewrite")
	}
	for _, token := range []string{"sidecar-managed-uat", "real-raw-token"} {
		r, _ := http.NewRequest("GET", "https://open.feishu.cn/test", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		if _, err := g.PreRoundTripE(r); err == nil {
			t.Fatal("raw/user token accepted")
		}
	}
	r, _ = http.NewRequest("GET", "https://cdn.example.test/artifact", nil)
	if _, err := g.PreRoundTripE(r); err != nil {
		t.Fatal(err)
	}
	if r.URL.Host != "127.0.0.1:12345" || r.Header.Get("X-Lark-Proxy-Identity") != "external" {
		t.Fatal("external request bypassed broker")
	}
}
