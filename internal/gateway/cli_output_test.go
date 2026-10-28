// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubprocessBufferCopyLimit(t *testing.T) {
	for _, size := range []int{0, 1024, 1025, 4096} {
		b := &cappedBuffer{max: 1024}
		n, err := io.Copy(b, struct{ io.Reader }{strings.NewReader(strings.Repeat("x", size))})
		if err != nil || n != int64(size) {
			t.Fatalf("output not drained: copied=%d error=%v", n, err)
		}
		_, _ = b.Write(nil)
		if b.Len() != min(size, 1024) || b.overflow != (size > 1024) {
			t.Fatalf("buffer limit: input=%d kept=%d overflow=%v", size, b.Len(), b.overflow)
		}
	}
}

func TestSubprocessOutputLimit(t *testing.T) {
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			c := testConfig(t)
			c.MaxFileBytes = 1024
			payload := `{"ok":true,"identity":"bot","data":{"value":"ok"}}`
			script := fixtureHeader()
			if stream == "stdout" {
				payload = string(encode(map[string]any{"ok": true, "identity": "bot", "data": map[string]any{"value": strings.Repeat("x", 4096)}}))
			} else {
				script += "printf '%s\\n' '" + strings.Repeat("x", (1<<20)+4096) + "' >&2\n"
			}
			script += "printf '%s\\n' '" + payload + "'\n"
			binary := filepath.Join(t.TempDir(), "fixture-backend")
			if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			c.CLIBinary = binary
			c.CLISHA256 = digest([]byte(script))
			executor, err := NewCLIExecutor(c)
			if err != nil {
				t.Fatal(err)
			}
			s, err := NewService(c, executor)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			op, _ := s.Catalog.Operation("wiki.spaces.get")
			app, _ := s.currentConfig().app("app-a")
			result := executor.Execute(context.Background(), Invocation{Principal: actor("alice"), Operation: op, Arguments: argsSpace("space-a")}, app)
			if result.Error == nil || result.Error.Kind != "backend_output_limit" {
				t.Fatalf("oversized %s accepted: error=%v result_bytes=%d", stream, result.Error, len(encode(result.Data)))
			}
		})
	}
}
