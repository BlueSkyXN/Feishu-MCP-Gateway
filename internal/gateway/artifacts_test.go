// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestArchiveReferencesAndLimits(t *testing.T) {
	for _, scenario := range []string{"rewrite", "unchanged-input", "ordinary-json", "outside-reference", "missing-record", "quota", "workspace-growth", "final-file-size"} {
		t.Run(scenario, func(t *testing.T) {
			c := testConfig(t)
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			w := &workspace{root: root, dir: dir, inputs: map[string]string{}}
			records := []byte("{\"record_id\":\"rec_fixture\"}\n")
			recordPath := filepath.Join(dir, "records.ndjson")
			manifestPath := filepath.Join(dir, "records.manifest.json")
			if scenario == "outside-reference" {
				recordPath = filepath.Join(filepath.Dir(dir), "outside.ndjson")
			}
			if scenario == "workspace-growth" || scenario == "final-file-size" {
				recordPath = "records.ndjson"
				manifestPath = "records.manifest.json"
			}
			businessValue := filepath.Join(dir, "user-value.ndjson")
			raw := encode(map[string]json.RawMessage{
				"format": encode("ndjson"), "record_file": encode(recordPath), "manifest_file": encode(manifestPath),
				"record_file_size_bytes": encode(len(records)), "large_number": json.RawMessage("9007199254740993"),
				"business_text": encode(businessValue),
			})
			if scenario != "missing-record" {
				if err = root.WriteFile("records.ndjson", records, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err = root.WriteFile("records.manifest.json", raw, 0600); err != nil {
				t.Fatal(err)
			}
			if scenario == "unchanged-input" {
				w.inputs["records.ndjson"] = digest(records)
			}
			if scenario == "quota" {
				c.MaxFileBytes = 2048
				c.MaxStorageBytes = 2048
			}
			if scenario == "workspace-growth" {
				c.MaxFileBytes = int64(len(raw) + len(records))
				c.MaxWorkspaceBytes = int64(len(raw) + len(records))
			}
			if scenario == "final-file-size" {
				c.MaxFileBytes = int64(len(raw))
			}
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
			store, err := OpenStore(c)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if scenario == "quota" {
				if _, err = store.PutFile(actor("alice"), "occupied.bin", "application/octet-stream", make([]byte, 2048-len(records)-32)); err != nil {
					t.Fatal(err)
				}
			}
			executor := &CLIExecutor{store: store, limit: c.MaxFileBytes, workspaceLimit: c.MaxWorkspaceBytes}
			operation := Operation{ArtifactFormat: "ndjson", ArtifactArguments: []string{"output"}}
			if scenario == "ordinary-json" {
				operation = Operation{}
			}
			files, paths, err := w.collect(executor, Invocation{Principal: actor("alice"), Operation: operation, Arguments: map[string]any{"output": "records.ndjson"}, ID: "fixture-invocation"})
			wantError := map[string]string{"outside-reference": "invalid_manifest", "missing-record": "invalid_manifest", "quota": "storage_quota", "workspace-growth": "workspace_limit", "final-file-size": "file_too_large"}[scenario]
			if wantError != "" {
				if err == nil || asFault(err).Kind != wantError {
					t.Fatalf("want %s, got %v", wantError, err)
				}
				for _, f := range files {
					if strings.HasSuffix(f.Name, ".manifest.json") {
						t.Fatal("failed manifest was published")
					}
				}
				stored, e := store.ListFiles(actor("alice"), "", 100)
				if e != nil {
					t.Fatal(e)
				}
				for _, f := range stored {
					if strings.HasSuffix(f.Name, ".manifest.json") {
						t.Fatal("failed manifest remained in store")
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 2 {
				t.Fatalf("expected a complete pair, got %d files", len(files))
			}
			f := paths["records.manifest.json"]
			_, body, err := store.ReadFile(actor("alice"), f.ID, 0, 65536)
			if err != nil {
				t.Fatal(err)
			}
			if digest(body) != f.SHA256 || int64(len(body)) != f.Size || f.SourceInvocation != "fixture-invocation" {
				t.Fatal("manifest metadata differs from stored bytes")
			}
			if scenario == "ordinary-json" {
				if string(body) != string(raw) {
					t.Fatal("unrelated JSON artifact was changed")
				}
				return
			}
			var m map[string]json.RawMessage
			if json.Unmarshal(body, &m) != nil {
				t.Fatal("invalid stored JSON")
			}
			var recordURI, selfURI, business string
			json.Unmarshal(m["record_file"], &recordURI)
			json.Unmarshal(m["manifest_file"], &selfURI)
			json.Unmarshal(m["business_text"], &business)
			if recordURI != paths["records.ndjson"].URI || selfURI != f.URI {
				t.Fatal("manifest does not reference stored files")
			}
			if string(m["large_number"]) != "9007199254740993" || business != businessValue {
				t.Fatal("business metadata was changed")
			}
			if _, err = store.File(actor("bob"), f.ID); err == nil {
				t.Fatal("cross-owner manifest access")
			}
		})
	}
}
