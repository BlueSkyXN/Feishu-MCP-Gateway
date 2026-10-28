// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/larksuite/cli/cmd"
)

// NDJSON 操作把记录保存在文件，stdout 是 manifest 或 jq_records 的 JSON 值流。
// 在受控 wrapper 中补上应用身份回执，Gateway 仍按同一协议读取结果。
func executeCLI() int {
	if os.Getenv("FEISHU_GATEWAY_ARTIFACT_MODE") != "ndjson" {
		return cmd.Execute()
	}
	out := &artifactBuffer{limit: envLimit("FEISHU_GATEWAY_FILE_LIMIT", 16<<20)}
	code := cmd.ExecuteWithOptions(cmd.WithIO(os.Stdin, out, os.Stderr))
	if code != 0 {
		return code
	}
	var err error
	if out.overflow {
		err = fmt.Errorf("artifact response exceeds invocation limit")
	} else {
		err = writeArtifactEnvelope(os.Stdout, out.buffer.Bytes())
	}
	if err != nil {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"ok": false, "identity": "bot", "error": map[string]any{"type": "internal", "subtype": "invalid_response", "message": "产物回执无法编码为有界 JSON"}})
		return 1
	}
	return 0
}

type artifactBuffer struct {
	buffer   bytes.Buffer
	limit    int64
	overflow bool
}

func (b *artifactBuffer) Write(p []byte) (int, error) {
	n := len(p)
	left := max(0, b.limit-int64(b.buffer.Len()))
	if int64(n) > left {
		b.overflow = true
		p = p[:left]
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

func writeArtifactEnvelope(out io.Writer, raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	values := []json.RawMessage{}
	for {
		var value json.RawMessage
		err := decoder.Decode(&value)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		values = append(values, value)
	}
	var data any = values
	if len(values) == 1 {
		data = values[0]
	}
	return json.NewEncoder(out).Encode(map[string]any{"ok": true, "identity": "bot", "data": data})
}
