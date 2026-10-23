// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var httpsLiteral = regexp.MustCompile(`https://[^\s<>"` + "`" + `]+`)

func explicitURLs(v any) []string {
	urls := map[string]bool{}
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case string:
			for _, raw := range httpsLiteral.FindAllString(x, 2048) {
				raw = strings.TrimRight(html.UnescapeString(raw), "),];'")
				if len(raw) <= 16384 {
					urls[raw] = true
				}
			}
		case map[string]any:
			for _, v := range x {
				visit(v)
			}
		case []any:
			for _, v := range x {
				visit(v)
			}
		}
	}
	visit(v)
	out := []string{}
	for url := range urls {
		if len(out) >= 2048 {
			break
		}
		out = append(out, url)
	}
	return out
}
func (g *proxyGrant) checkGenericInput(r *http.Request, body []byte) error {
	args := g.inv.Arguments
	op := g.inv.Operation
	params := object(args["params"])
	query := r.URL.Query()
	for key, values := range query {
		value, ok := params[key]
		if !ok {
			value, ok = op.ParameterDefaults[key]
		}
		if !ok || !slices.Contains(op.QueryFields, key) {
			return fault("egress_arguments", "上游查询参数未被此次调用授权")
		}
		expected := []string{}
		if a, ok := value.([]any); ok {
			for _, v := range a {
				expected = append(expected, scalar(v))
			}
		} else {
			expected = append(expected, scalar(value))
		}
		if !slices.Equal(expected, values) {
			return fault("egress_arguments", "上游查询参数与此次调用不一致")
		}
	}
	for key := range params {
		if slices.Contains(op.QueryFields, key) {
			if _, ok := query[key]; !ok {
				return fault("egress_arguments", "缺少已授权的查询参数")
			}
		}
	}
	typ, opts, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if typ != "multipart/form-data" {
		return nil
	}
	reader := multipart.NewReader(bytes.NewReader(body), opts["boundary"])
	seen := map[string]bool{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fault("egress_arguments", "multipart 请求格式无效")
		}
		name := part.FormName()
		if seen[name] {
			return fault("egress_arguments", "multipart 字段重复")
		}
		seen[name] = true
		data, err := readBounded(part, g.broker.limit)
		part.Close()
		if err != nil {
			return err
		}
		if slices.Contains(op.FileFields, name) {
			ref := object(object(args["files"])[name])
			id, _ := ref["file_id"].(string)
			if id == "" || g.inv.FileHashes[id] != digest(data) {
				return fault("egress_arguments", "上传字节与已授权文件不一致")
			}
		} else {
			value, ok := object(args["data"])[name]
			if !ok || scalar(value) != string(data) {
				return fault("egress_arguments", "multipart 字段与已授权参数不一致")
			}
		}
	}
	for field := range object(args["files"]) {
		if !seen[field] {
			return fault("egress_arguments", "上传缺少文件字段")
		}
	}
	for field := range object(args["data"]) {
		if !seen[field] {
			return fault("egress_arguments", "上传缺少数据字段")
		}
	}
	return nil
}

func scalar(v any) string {
	if n, ok := v.(float64); ok {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}
