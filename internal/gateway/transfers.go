// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Transfer struct {
	Ticket   string `json:"ticket"`
	Method   string `json:"method"`
	Path     string `json:"path"`
	Expires  int64  `json:"expires_at"`
	MaxBytes int64  `json:"max_bytes"`
}

func (s *Service) addTransferTool(server *mcp.Server) {
	s.addUtility(server, "feishu_file_transfer", "为当前身份签发 60 秒有效的单次上传或下载票据，供本地文件适配器使用。持有票据者在有效期内可传输这个文件；票据不可用来调用其他工具。", schemaObject(map[string]any{"direction": map[string]any{"type": "string", "enum": []string{"upload", "download"}}, "file_id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string", "maxLength": 255}, "mime_type": map[string]any{"type": "string", "maxLength": 128}}, []string{"direction"}), false, func(ctx context.Context, p Principal, args map[string]any) Result {
		tr, err := s.issueTransfer(p, args)
		if err != nil {
			return failed(err)
		}
		return Result{Status: "succeeded", Data: tr}
	})
}
func (s *Service) issueTransfer(p Principal, args map[string]any) (Transfer, error) {
	var tr Transfer
	c, _, err := s.currentConfig().checkPrincipal(p, true)
	if err != nil {
		return tr, err
	}
	if !c.AllowTransferTickets {
		return tr, fault("transfer_disabled", "当前接入未开启文件票据")
	}
	direction, _ := args["direction"].(string)
	id, _ := args["file_id"].(string)
	name, _ := args["name"].(string)
	typ, _ := args["mime_type"].(string)
	if direction == "download" {
		if _, err = s.AuthorizeFile(p, id); err != nil {
			return tr, err
		}
		tr.Method = "GET"
	} else if direction == "upload" {
		if _, err = validFileName(name); err != nil {
			return tr, err
		}
		tr.Method = "POST"
		if typ == "" {
			typ = "application/octet-stream"
		}
		if _, _, err = mime.ParseMediaType(typ); err != nil {
			return tr, fault("invalid_mime_type", "mime_type 不是有效的 MIME 类型")
		}
	} else {
		return tr, fault("invalid_direction", "direction 需要 upload 或 download")
	}
	tr.Ticket = newID("transfer_")
	tr.Path = "/transfers/file"
	tr.Expires = time.Now().Unix() + 60
	tr.MaxBytes = s.config.MaxFileBytes
	_, err = s.Store.db.Exec(`INSERT INTO transfers VALUES(?,?,?,?,?,?,?,0)`, digest([]byte(tr.Ticket)), string(encode(p)), direction, id, name, typ, tr.Expires)
	return tr, err
}
func (s *Service) transferHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/transfers/file" {
		http.NotFound(w, r)
		return
	}
	tx, err := s.Store.db.Begin()
	if err != nil {
		http.Error(w, "storage unavailable", 500)
		return
	}
	defer tx.Rollback()
	var raw, direction, id, name, typ string
	key := digest([]byte(bearer(r)))
	err = tx.QueryRow(`SELECT principal,direction,file,name,mime FROM transfers WHERE hash=? AND used=0 AND expires>?`, key, time.Now().Unix()).Scan(&raw, &direction, &id, &name, &typ)
	if err != nil {
		http.Error(w, "transfer ticket unavailable or expired", 401)
		return
	}
	if (direction == "download" && r.Method != "GET") || (direction == "upload" && r.Method != "POST") {
		http.Error(w, "method does not match transfer", 405)
		return
	}
	var p Principal
	if json.Unmarshal([]byte(raw), &p) != nil {
		http.Error(w, "transfer unavailable", 500)
		return
	}
	c, _, err := s.currentConfig().checkPrincipal(p, true)
	if err != nil || !c.AllowTransferTickets {
		http.Error(w, "transfer authorization revoked", 403)
		return
	}
	if _, err = tx.Exec(`UPDATE transfers SET used=1 WHERE hash=?`, key); err != nil {
		http.Error(w, "transfer unavailable", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "transfer unavailable", 500)
		return
	}
	if err = s.rate(p); err != nil {
		http.Error(w, err.Error(), 429)
		return
	}
	if err = s.acquirePrincipal(p); err != nil {
		http.Error(w, err.Error(), 429)
		return
	}
	defer s.releasePrincipal(p)
	if direction == "download" {
		f, err := s.AuthorizeFile(p, id)
		if err != nil {
			http.Error(w, "file authorization unavailable", 403)
			return
		}
		in, err := s.Store.files.Open(id)
		if err != nil {
			http.Error(w, "file unavailable", 404)
			return
		}
		defer in.Close()
		w.Header().Set("Content-Type", f.MIME)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Name}))
		_, _ = io.Copy(w, io.LimitReader(in, f.Size))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxFileBytes)
	b, err := readBounded(r.Body, s.config.MaxFileBytes)
	if err != nil {
		http.Error(w, "file size exceeded or transfer interrupted", 413)
		return
	}
	if _, _, err = s.currentConfig().checkPrincipal(p, true); err != nil {
		http.Error(w, "authorization revoked", 403)
		return
	}
	f, err := s.Store.PutFile(p, name, typ, b)
	if err != nil {
		http.Error(w, asFault(err).Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	_, _ = w.Write(encode(f))
}
func validFileName(name string) (string, error) {
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\\r\n\x00") || name == "." || name == ".." || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", fault("invalid_filename", "需要没有目录的文件名")
	}
	return name, nil
}
