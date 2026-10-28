// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// 文件引用只在声明了 FileRef 的位置解析；普通 JSON 业务字段不参与挂载。
func inputFileIDs(op Operation, args map[string]any) []string {
	ids := map[string]bool{}
	var add func(any)
	add = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if id, ok := x["file_id"].(string); ok {
				ids[id] = true
			}
		case []any:
			for _, v := range x {
				add(v)
			}
		}
	}
	for _, v := range object(args["workspace_files"]) {
		if id, ok := v.(string); ok {
			ids[id] = true
		}
	}
	for _, v := range object(args["files"]) {
		add(v)
	}
	for k, b := range op.FlagBindings {
		if b.File || contains(b.Input, "file") {
			add(args[k])
		}
	}
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func logicalPath(name string) (string, error) {
	if name == "" || !filepath.IsLocal(name) || strings.ContainsAny(name, "\\\x00") {
		return "", fault("invalid_file_path", "使用工作区内的相对文件名")
	}
	p := filepath.ToSlash(filepath.Clean(name))
	if p == ".gateway" || strings.HasPrefix(p, ".gateway/") || strings.Contains(p, "\n") {
		return "", fault("invalid_file_path", "文件名使用了内部保留目录")
	}
	return p, nil
}

type workspace struct {
	hashes    map[string]string
	urls      []string
	root      *os.Root
	dir       string
	inputs    map[string]string
	filePaths map[string]string
}

func (e *CLIExecutor) prepareFiles(in Invocation, dir string) (*workspace, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	w := &workspace{root: root, dir: dir, inputs: map[string]string{}, filePaths: map[string]string{}, hashes: map[string]string{}}
	fail := func(err error) (*workspace, error) { root.Close(); return nil, err }
	mountedBytes := int64(0)
	mountedCount := 0
	mount := func(id, name string) error {
		f, err := e.authorizeFile(in.Principal, id, in.ID)
		if err != nil {
			return err
		}
		mountedBytes += f.Size
		mountedCount++
		if mountedBytes > e.workspaceLimit || mountedCount > 256 {
			return fault("workspace_limit", "输入集合超过工作区容量或 256 个文件限制")
		}
		b, err := e.store.readPinned(in.Principal, id, in.ID)
		if err != nil {
			return err
		}
		if err = root.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return err
		}
		dst, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = dst.Write(b)
		closeErr := dst.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		w.hashes[id] = f.SHA256
		if isText(f.MIME) || strings.HasSuffix(f.Name, ".md") || strings.HasSuffix(f.Name, ".xml") {
			w.urls = append(w.urls, explicitURLs(string(b))...)
		}
		w.inputs[name] = f.SHA256
		w.filePaths[id] = name
		return nil
	}
	for name, value := range object(in.Arguments["workspace_files"]) {
		p, err := logicalPath(name)
		if err != nil {
			return fail(err)
		}
		id, ok := value.(string)
		if !ok {
			return fail(fault("invalid_file_ref", "输入集合必须使用 file_id"))
		}
		if err = mount(id, "inputs/"+p); err != nil {
			return fail(err)
		}
	}
	for _, id := range inputFileIDs(in.Operation, in.Arguments) {
		if _, ok := w.filePaths[id]; ok {
			continue
		}
		f, err := e.authorizeFile(in.Principal, id, in.ID)
		if err != nil {
			return fail(err)
		}
		if err = mount(id, "inputs/"+id+"/"+f.Name); err != nil {
			return fail(err)
		}
	}
	return w, nil
}
func (w *workspace) fileArgument(value any, b FlagBinding) (string, error) {
	if ref, ok := value.(map[string]any); ok && len(ref) == 1 && ref["file_id"] != nil && (b.File || contains(b.Input, "file")) {
		id, _ := ref["file_id"].(string)
		p, ok := w.filePaths[id]
		if !ok {
			return "", fault("invalid_file_ref", "文件未挂载")
		}
		if contains(b.Input, "file") && !b.File {
			return "@" + p, nil
		}
		return p, nil
	}
	if b.JSON {
		switch value.(type) {
		case map[string]any, []any, float64, bool:
			return string(encode(value)), nil
		}
	}
	text, ok := value.(string)
	if !ok {
		return "", fault("invalid_arguments", "该参数需要文本或文件引用")
	}
	if b.Output {
		return logicalPath(text)
	}
	if strings.HasPrefix(text, "@") {
		p, err := logicalPath(strings.TrimPrefix(text, "@"))
		if err != nil {
			return "", err
		}
		if _, ok := w.inputs[p]; !ok {
			return "", fault("invalid_file_ref", "@file 必须指向已挂载的 inputs 文件")
		}
		return "@" + p, nil
	}
	if b.File && !strings.HasPrefix(text, "https://") {
		if filepath.IsAbs(text) || strings.Contains(text, "..") || strings.ContainsAny(text, "/\\") {
			p, err := logicalPath(text)
			if err != nil {
				return "", err
			}
			if _, ok := w.inputs[p]; !ok {
				return "", fault("invalid_file_ref", "本地文件使用 file_id 或 inputs 中已挂载的文件")
			}
		}
	}
	return text, nil
}
func (w *workspace) collect(e *CLIExecutor, in Invocation) ([]File, map[string]File, error) {
	var files []File
	paths := map[string]File{}
	var manifests []string
	var total int64
	read := func(name string) ([]byte, error) {
		f, err := w.root.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return readBounded(f, e.limit)
	}
	save := func(name string, b []byte) error {
		if len(files) >= 256 {
			return fault("output_file_limit", "产物超过 256 个文件，请分批执行")
		}
		typ := mime.TypeByExtension(filepath.Ext(name))
		if strings.EqualFold(filepath.Ext(name), ".ndjson") {
			typ = "application/x-ndjson"
		}
		if typ == "" {
			typ = "application/octet-stream"
		}
		saved, err := e.store.PutFile(in.Principal, filepath.Base(name), typ, b, in.ID)
		if err != nil {
			return err
		}
		files = append(files, saved)
		paths[name] = saved
		return nil
	}
	err := fs.WalkDir(w.root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if name == ".gateway" {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fault("unsafe_output", "输出包含符号链接，无法归档")
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fault("unsafe_output", "输出必须为普通文件")
		}
		if info.Size() > e.limit {
			return fault("file_too_large", "产物超过单文件上限")
		}
		total += info.Size()
		if total > e.workspaceLimit {
			return fault("workspace_limit", "工作区产物超过总量限制")
		}
		b, err := read(name)
		if err != nil {
			return err
		}
		if w.inputs[name] == digest(b) {
			return nil
		}
		if len(files)+len(manifests) >= 256 {
			return fault("output_file_limit", "产物超过 256 个文件，请分批执行")
		}
		if in.Operation.outputFormat(in.Arguments) == "ndjson" && strings.HasSuffix(name, ".manifest.json") {
			// 记录文件先归档，索引随后引用已落盘的文件和自身的新 ID。
			manifests = append(manifests, name)
			return nil
		}
		return save(name, b)
	})
	if err != nil {
		return files, paths, err
	}
	for _, name := range manifests {
		b, err := read(name)
		if err != nil {
			return files, paths, err
		}
		manifest, recordPath, err := w.recordManifest(name, b)
		if err != nil {
			return files, paths, err
		}
		if _, ok := paths[recordPath]; !ok && w.inputs[recordPath] != "" {
			// 显式导出也可能覆盖 inputs 中的文件且内容未变；索引需要本次产物的独立有效期。
			records, err := read(recordPath)
			if err == nil {
				err = save(recordPath, records)
			}
			if err != nil {
				return files, paths, err
			}
		}
		record, ok := paths[recordPath]
		if !ok {
			return files, paths, fault("invalid_manifest", "NDJSON manifest 引用的记录文件未归档，请核对导出结果")
		}
		if len(files) >= 256 {
			return files, paths, fault("output_file_limit", "产物超过 256 个文件，请分批执行")
		}
		saved, err := e.store.putPreparedFile(in.Principal, filepath.Base(name), "application/json", func(self File) ([]byte, error) {
			manifest["record_file"] = encode(record.URI)
			manifest["manifest_file"] = encode(self.URI)
			content := append(encode(manifest), '\n')
			if total+int64(len(content)-len(b)) > e.workspaceLimit {
				return nil, fault("workspace_limit", "工作区产物超过总量限制")
			}
			return content, nil
		}, in.ID)
		if err != nil {
			return files, paths, err
		}
		total += saved.Size - int64(len(b))
		files = append(files, saved)
		paths[name] = saved
	}
	return files, paths, nil
}

// 仅解析 NDJSON 产物索引的关联字段；业务元数据用 RawMessage 保留精确数值和字符串。
func (w *workspace) recordManifest(name string, raw []byte) (map[string]json.RawMessage, string, error) {
	var manifest map[string]json.RawMessage
	invalid := fault("invalid_manifest", "NDJSON manifest 的文件引用无效，请核对导出结果")
	if json.Unmarshal(raw, &manifest) != nil {
		return nil, "", invalid
	}
	var format, recordFile, manifestFile string
	if json.Unmarshal(manifest["format"], &format) != nil || format != "ndjson" ||
		json.Unmarshal(manifest["record_file"], &recordFile) != nil || json.Unmarshal(manifest["manifest_file"], &manifestFile) != nil {
		return nil, "", invalid
	}
	relative := func(value string) (string, error) {
		if filepath.IsAbs(value) {
			var err error
			value, err = filepath.Rel(w.dir, value)
			if err != nil {
				return "", err
			}
		}
		return logicalPath(value)
	}
	recordPath, err := relative(recordFile)
	if err != nil || !strings.HasSuffix(recordPath, ".ndjson") {
		return nil, "", invalid
	}
	selfPath, err := relative(manifestFile)
	if err != nil || selfPath != name {
		return nil, "", invalid
	}
	return manifest, recordPath, nil
}

func (w *workspace) rewrite(v any, paths map[string]File) any {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, w.dir+string(filepath.Separator)) {
			name := filepath.ToSlash(strings.TrimPrefix(x, w.dir+string(filepath.Separator)))
			if f, ok := paths[name]; ok {
				return f.URI
			}
			return name
		}
		if f, ok := paths[x]; ok {
			return f.URI
		}
		return x
	case map[string]any:
		for k, v := range x {
			x[k] = w.rewrite(v, paths)
		}
		return x
	case []any:
		for i, v := range x {
			x[i] = w.rewrite(v, paths)
		}
		return x
	default:
		return v
	}
}
func cloneArguments(args map[string]any) map[string]any {
	var out map[string]any
	_ = json.Unmarshal(encode(args), &out)
	return out
}
func (e *CLIExecutor) buildArgs(in Invocation, w *workspace) ([]string, error) {
	argv := append([]string{}, in.Operation.Command...)
	argv = append(argv, "--as", "bot", "--format", in.Operation.outputFormat(in.Arguments))
	if in.Operation.Risk == "high-risk-write" || in.Operation.ConditionalConfirmation {
		argv = append(argv, "--yes")
	}
	if in.Operation.Kind != "shortcut" {
		for _, section := range []string{"params", "data"} {
			if v, ok := in.Arguments[section]; ok {
				p := ".gateway/" + section + ".json"
				if err := w.root.WriteFile(p, encode(v), 0600); err != nil {
					return nil, err
				}
				argv = append(argv, "--"+section, "@"+p)
			}
		}
		for _, field := range in.Operation.FileFields {
			if ref, ok := object(in.Arguments["files"])[field]; ok {
				p, err := w.fileArgument(ref, FlagBinding{File: true})
				if err != nil {
					return nil, err
				}
				argv = append(argv, "--file", field+"="+p)
			}
		}
		return argv, nil
	}
	keys := make([]string, 0, len(in.Operation.FlagBindings))
	for key := range in.Operation.FlagBindings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		binding := in.Operation.FlagBindings[key]
		value, ok := in.Arguments[key]
		if !ok {
			continue
		}
		values := []any{value}
		if a, ok := value.([]any); ok && !binding.JSON {
			values = a
		}
		if binding.Type == "string_slice" {
			a, _ := value.([]any)
			values := []string{}
			for _, v := range a {
				values = append(values, v.(string))
			}
			var buf bytes.Buffer
			writer := csv.NewWriter(&buf)
			if err := writer.Write(values); err != nil {
				return nil, err
			}
			writer.Flush()
			argv = append(argv, "--"+binding.Name+"="+strings.TrimSuffix(buf.String(), "\n"))
			continue
		}
		for _, v := range values {
			var text string
			var err error
			switch binding.Type {
			case "string", "string_array", "string_slice", "":
				text, err = w.fileArgument(v, binding)
			case "int", "int_array":
				if n, ok := v.(float64); ok {
					text = strconv.FormatFloat(n, 'f', -1, 64)
				} else {
					text = fmt.Sprint(v)
				}
			default:
				text = fmt.Sprint(v)
			}
			if err != nil {
				return nil, err
			}
			if binding.CollectResults && strings.TrimSpace(text) != "" {
				// CLI 的 jq 字符串输出没有引号；收集为数组后可完整保留类型和换行。
				text = "[(\n" + text + "\n)]"
			}
			argv = append(argv, "--"+binding.Name+"="+text)
		}
	}
	return argv, nil
}
