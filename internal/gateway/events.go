package gateway

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// 以应用配置的加密 HTTP 事件接入代替 CLI 的长期 daemon；MCP 读取是有界操作。
func eventReadOperation() Operation {
	return Operation{ID: "events.read", Tool: "feishu_events_read", Kind: "native", Command: []string{"events", "read"}, Pack: "events.read", Risk: "read", Description: "读取当前应用已接收的事件；按 event_type 授权，使用分页句柄继续，最多等待 30 秒。", InputSchema: schemaObject(map[string]any{"params": schemaObject(map[string]any{"event_type": map[string]any{"type": "string", "minLength": 1}, "page_token": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}, "wait_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 30}}, []string{"event_type"}), "async": map[string]any{"type": "boolean"}}, []string{"params"}), DataSchema: map[string]any{"type": "object", "properties": map[string]any{"events": map[string]any{"type": "array"}, "page_token": map[string]any{"type": "string"}, "has_more": map[string]any{"type": "boolean"}}}}
}
func (s *Service) eventHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	appID := strings.TrimPrefix(r.URL.Path, "/events/")
	a, ok := s.currentConfig().app(appID)
	if !ok || len(a.EventTypes) == 0 {
		http.NotFound(w, r)
		return
	}
	raw, err := readBounded(http.MaxBytesReader(w, r.Body, 1<<20), 1<<20)
	if err != nil {
		http.Error(w, "event too large", 413)
		return
	}
	stamp, nonce, signature := r.Header.Get("X-Lark-Request-Timestamp"), r.Header.Get("X-Lark-Request-Nonce"), r.Header.Get("X-Lark-Signature")
	n, signatureErr := strconv.ParseInt(stamp, 10, 64)
	expected := digest([]byte(stamp + nonce + a.eventKey + string(raw)))

	plain, err := decryptEvent(raw, a.eventKey)
	if err != nil {
		http.Error(w, "invalid encrypted event", 400)
		return
	}
	var event map[string]any
	if json.Unmarshal(plain, &event) != nil {
		http.Error(w, "invalid event", 400)
		return
	}
	header := object(event["header"])
	token, _ := header["token"].(string)
	if event["type"] == "url_verification" {
		token, _ = event["token"].(string)
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(a.eventToken)) != 1 {
		http.Error(w, "invalid event token", 403)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if event["type"] == "url_verification" {
		challenge, ok := event["challenge"].(string)
		if !ok {
			http.Error(w, "invalid challenge", 400)
			return
		}
		_, _ = w.Write(encode(map[string]any{"challenge": challenge}))
		return
	}
	if signatureErr != nil || n < time.Now().Unix()-300 || n > time.Now().Unix()+300 || nonce == "" || subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		http.Error(w, "invalid event signature", 401)
		return
	}
	eventID, _ := header["event_id"].(string)
	eventType, _ := header["event_type"].(string)
	if header["app_id"] != a.AppID || eventID == "" || len(eventID) > 256 || !slices.Contains(a.EventTypes, eventType) {
		http.Error(w, "event outside application subscription", 403)
		return
	}
	delete(header, "token")
	delete(event, "token")
	tx, err := s.Store.db.Begin()
	if err != nil {
		http.Error(w, "event storage unavailable", 503)
		return
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT OR IGNORE INTO events(app,event_id,event_type,payload,created) VALUES(?,?,?,?,?)`, a.ID, eventID, eventType, string(encode(event)), time.Now().Unix())
	if err == nil {
		err = pruneEvents(tx, a.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		http.Error(w, "event storage unavailable", 503)
		return
	}
	_, _ = w.Write([]byte(`{"code":0}`))
}

// 飞书回调的 CBC 帧与签名格式核对于官方 oapi-sdk-go v3.7.2/event/event.go。
func decryptEvent(raw []byte, key string) ([]byte, error) {
	var envelope struct {
		Encrypt string `json:"encrypt"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	buf, err := base64.StdEncoding.DecodeString(envelope.Encrypt)
	if err != nil || len(buf) < 2*aes.BlockSize || len(buf)%aes.BlockSize != 0 {
		return nil, fault("invalid_event", "事件密文格式无效")
	}
	hash := sha256Bytes(key)
	block, err := aes.NewCipher(hash)
	if err != nil {
		return nil, err
	}
	plain := append([]byte(nil), buf[aes.BlockSize:]...)
	cipher.NewCBCDecrypter(block, buf[:aes.BlockSize]).CryptBlocks(plain, plain)
	start, end := bytes.IndexByte(plain, '{'), bytes.LastIndexByte(plain, '}')
	if start < 0 || end < start || !json.Valid(plain[start:end+1]) {
		return nil, fault("invalid_event", "事件内容无效")
	}
	return plain[start : end+1], nil
}
func sha256Bytes(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }
func (s *Service) readEvents(ctx context.Context, p Principal, args map[string]any) ExecutionResult {
	params := object(args["params"])
	typ, _ := params["event_type"].(string)
	after := int64(0)
	if token, _ := params["page_token"].(string); token != "" {
		var err error
		after, err = strconv.ParseInt(token, 10, 64)
		if err != nil || after < 0 {
			return ExecutionResult{Error: fault("invalid_cursor", "事件游标无效")}
		}
	}
	limit := 100
	if n, ok := params["limit"].(float64); ok {
		limit = int(n)
	}
	wait := time.Duration(0)
	if n, ok := params["wait_seconds"].(float64); ok {
		wait = time.Duration(n) * time.Second
	}
	deadline := time.Now().Add(wait)
	for {
		cfg := s.currentConfig()
		c, a, err := cfg.checkPrincipal(p, true)
		if err == nil {
			_, err = authorize(cfg, p, eventReadOperation(), args)
		}
		if err != nil {
			return ExecutionResult{Error: asFault(err)}
		}
		if !slices.Contains(c.EventTypes, typ) || !slices.Contains(a.EventTypes, typ) || (p.Actor != "" && !slices.Contains(c.Actors[p.Actor].EventTypes, typ)) {
			return ExecutionResult{Error: fault("event_type_denied", "当前主体未被授予该事件类型")}
		}
		var floor int64
		_ = s.Store.db.QueryRow(`SELECT floor FROM event_retention WHERE app=? AND event_type=?`, a.ID, typ).Scan(&floor)
		if after > 0 && after < floor {
			return ExecutionResult{Error: fault("event_cursor_gap", "此前事件已超出保留范围；核对缺口后从第一页重新读取")}
		}
		rows, err := s.Store.db.Query(`SELECT seq,payload FROM events WHERE app=? AND event_type=? AND seq>? ORDER BY seq LIMIT ?`, a.ID, typ, after, limit+1)
		if err != nil {
			return ExecutionResult{Error: asFault(err)}
		}
		list := []any{}
		last := after
		more := false
		for rows.Next() {
			var seq int64
			var raw string
			if err = rows.Scan(&seq, &raw); err != nil {
				break
			}
			if len(list) >= limit {
				more = true
				break
			}
			var v any
			if err = json.Unmarshal([]byte(raw), &v); err != nil {
				break
			}
			list = append(list, v)
			last = seq
		}
		rowErr := rows.Err()
		rows.Close()
		if err != nil || rowErr != nil {
			return ExecutionResult{Error: fault("event_storage_failed", "无法读取事件")}
		}
		if len(list) > 0 || !time.Now().Before(deadline) {
			return ExecutionResult{Data: map[string]any{"events": list, "page_token": strconv.FormatInt(last, 10), "has_more": more}}
		}
		select {
		case <-ctx.Done():
			return ExecutionResult{Error: fault("execution_interrupted", "事件读取已结束")}
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func pruneEvents(tx *sql.Tx, app string) error {
	const expired = `SELECT seq FROM (SELECT seq,created,SUM(length(payload)) OVER(ORDER BY seq DESC) bytes,ROW_NUMBER() OVER(ORDER BY seq DESC) position FROM events WHERE app=?) WHERE created<? OR bytes>? OR position>10000`
	args := []any{app, time.Now().Unix() - 86400, 64 << 20}
	_, err := tx.Exec(`INSERT INTO event_retention(app,event_type,floor) SELECT app,event_type,MAX(seq) FROM events WHERE seq IN (`+expired+`) GROUP BY app,event_type ON CONFLICT(app,event_type) DO UPDATE SET floor=MAX(event_retention.floor,excluded.floor)`, args...)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM events WHERE seq IN (`+expired+`)`, args...)
	return err
}
