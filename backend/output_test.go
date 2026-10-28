// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestArtifactEnvelopePreservesJSONValues(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`{"format":"ndjson","records_count":2}`, `{"format":"ndjson","records_count":2}`},
		{"\"first\"\n\"second\"\n", `["first","second"]`},
		{"", `[]`},
		{`null`, `null`},
		{`{"ok":false,"identity":"user","n":9007199254740993}`, `{"ok":false,"identity":"user","n":9007199254740993}`},
	} {
		var out bytes.Buffer
		if err := writeArtifactEnvelope(&out, []byte(tc.raw)); err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			OK       bool
			Identity string
			Data     json.RawMessage
		}
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, envelope.Data); err != nil {
			t.Fatal(err)
		}
		if !envelope.OK || envelope.Identity != "bot" || compact.String() != tc.want {
			t.Fatalf("envelope: %s", out.String())
		}
	}
	var out bytes.Buffer
	if err := writeArtifactEnvelope(&out, []byte("{}\ninvalid")); err == nil || out.Len() != 0 {
		t.Fatal("invalid stream produced a success receipt")
	}
}

func TestArtifactBufferIsBounded(t *testing.T) {
	out := &artifactBuffer{limit: 8}
	if n, err := out.Write([]byte("123456")); n != 6 || err != nil {
		t.Fatal(n, err)
	}
	if n, err := out.Write([]byte("789012")); n != 6 || err != nil {
		t.Fatal(n, err)
	}
	if _, err := out.Write([]byte(strings.Repeat("x", 100))); err != nil {
		t.Fatal(err)
	}
	if !out.overflow || out.buffer.String() != "12345678" {
		t.Fatalf("overflow=%t data=%q", out.overflow, out.buffer.String())
	}
}
