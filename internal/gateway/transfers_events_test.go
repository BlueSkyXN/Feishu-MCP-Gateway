package gateway

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTransferTicketSingleUseReauthorizationAndLocalAdapter(t *testing.T) {
	c := testConfig(t)
	c.Clients[0].AllowTransferTickets = true
	s := startService(t, c, &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	if _, err := s.issueTransfer(Principal{ClientID: "wake", AppID: "app-a", OrgID: "org-a"}, map[string]any{"direction": "upload", "name": "test.txt"}); err == nil {
		t.Fatal("missing actor ticket")
	}
	ticket, err := s.issueTransfer(actor("alice"), map[string]any{"direction": "upload", "name": "sample.bin"})
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "input.bin")
	os.WriteFile(input, []byte("file adapter fixture"), 0600)
	run := func(tr Transfer, path string) ([]byte, error) {
		cmd := exec.Command("python3", "../../scripts/transfer_file.py", "--endpoint", server.URL, "--file", path)
		cmd.Stdin = bytes.NewReader(encode(tr))
		return cmd.CombinedOutput()
	}
	b, err := run(ticket, input)
	if err != nil {
		t.Fatalf("adapter upload %s %v", b, err)
	}
	var f File
	if json.Unmarshal(b, &f) != nil || f.ID == "" {
		t.Fatalf("missing file %s", b)
	}
	if _, err = run(ticket, input); err == nil {
		t.Fatal("ticket replay succeeded")
	}
	download, err := s.issueTransfer(actor("alice"), map[string]any{"direction": "download", "file_id": f.ID})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "download.bin")
	b, err = run(download, target)
	if err != nil {
		t.Fatalf("download adapter %s %v", b, err)
	}
	actual, _ := os.ReadFile(target)
	if string(actual) != "file adapter fixture" {
		t.Fatal("download mismatch")
	}
	if _, err = s.issueTransfer(actor("bob"), map[string]any{"direction": "download", "file_id": f.ID}); err == nil {
		t.Fatal("cross actor ticket")
	}
	download, err = s.issueTransfer(actor("alice"), map[string]any{"direction": "download", "file_id": f.ID})
	if err != nil {
		t.Fatal(err)
	}
	next := cloneConfig(c)
	delete(next.Clients[0].Actors, actor("alice").Actor)
	if err = s.Reload(next); err != nil {
		t.Fatal(err)
	}
	if _, err = run(download, filepath.Join(t.TempDir(), "revoked.bin")); err == nil {
		t.Fatal("revoked ticket remained usable")
	}
}
func encryptedEvent(t *testing.T, v any, key string) []byte {
	t.Helper()
	payload := encode(v)
	padding := aes.BlockSize - len(payload)%aes.BlockSize
	payload = append(payload, bytes.Repeat([]byte{byte(padding)}, padding)...)
	iv := bytes.Repeat([]byte{7}, aes.BlockSize)
	block, err := aes.NewCipher(sha256Bytes(key))
	if err != nil {
		t.Fatal(err)
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(payload, payload)
	return encode(map[string]any{"encrypt": base64.StdEncoding.EncodeToString(append(iv, payload...))})
}
func TestApplicationEventIngressDedupeCursorAndPolicy(t *testing.T) {
	c := testConfig(t)
	t.Setenv("TEST_EVENT_KEY", "fixture-key")
	t.Setenv("TEST_EVENT_TOKEN", "fixture-token")
	c.Apps[0].EventEncryptKeyEnv = "TEST_EVENT_KEY"
	c.Apps[0].EventVerifyTokenEnv = "TEST_EVENT_TOKEN"
	c.Apps[0].EventTypes = []string{"im.message.receive_v1", "im.chat.updated_v1"}
	c.Clients[0].Packs = append(c.Clients[0].Packs, "events.read")
	c.Clients[0].ResourceMode = "application_scope"
	c.Clients[0].EventTypes = c.Apps[0].EventTypes
	c.Clients[0].Actors = map[string]ActorPolicy{actor("alice").Actor: {EventTypes: []string{"im.message.receive_v1"}}, actor("bob").Actor: {EventTypes: []string{"im.chat.updated_v1"}}}
	s := startService(t, c, &fakeExecutor{})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	send := func(event any, signature bool) int {
		raw := encryptedEvent(t, event, "fixture-key")
		r, _ := http.NewRequest("POST", server.URL+"/events/app-a", bytes.NewReader(raw))
		if signature {
			stamp := fmt.Sprint(time.Now().Unix())
			r.Header.Set("X-Lark-Request-Timestamp", stamp)
			r.Header.Set("X-Lark-Request-Nonce", "nonce")
			r.Header.Set("X-Lark-Signature", digest([]byte(stamp+"nonce"+"fixture-key"+string(raw))))
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := send(map[string]any{"type": "url_verification", "token": "fixture-token", "challenge": "fixture-challenge"}, false); code != 200 {
		t.Fatal("unsigned challenge rejected", code)
	}
	event := map[string]any{"schema": "2.0", "header": map[string]any{"app_id": "cli_fixture_a", "event_id": "event-1", "event_type": "im.message.receive_v1", "token": "fixture-token"}, "event": map[string]any{"message": map[string]any{"message_id": "message-1"}}}
	if send(event, false) != 401 {
		t.Fatal("unsigned event accepted")
	}
	for i := 0; i < 2; i++ {
		if code := send(event, true); code != 200 {
			t.Fatal(code)
		}
	}
	args := map[string]any{"params": map[string]any{"event_type": "im.message.receive_v1", "limit": float64(1)}}
	r := s.Invoke(context.Background(), actor("alice"), "events.read", args)
	if r.Error != nil {
		t.Fatal(r.Error)
	}
	data := object(r.Data)
	if len(data["events"].([]any)) != 1 || strings.Contains(string(encode(r)), "fixture-token") {
		t.Fatalf("event duplicate or secret exposed: %s", encode(r))
	}
	if r = s.Invoke(context.Background(), actor("bob"), "events.read", args); r.Error == nil || r.Error.Kind != "event_type_denied" {
		t.Fatal("event type isolation failed", r)
	}
	args = cloneArguments(args)
	object(args["params"])["page_token"] = data["page_token"]
	r = s.Invoke(context.Background(), actor("alice"), "events.read", args)
	if r.Error != nil || len(object(r.Data)["events"].([]any)) != 0 {
		t.Fatal("event cursor failed", r)
	}
	if r = s.Invoke(context.Background(), actor("bob"), "events.read", args); r.Error == nil || r.Error.Kind != "invalid_cursor" {
		t.Fatal("actor stole cursor", r)
	}
	s.Store.db.Exec(`UPDATE events SET created=0`)
	if err := s.Store.Collect(); err != nil {
		t.Fatal(err)
	}
}
