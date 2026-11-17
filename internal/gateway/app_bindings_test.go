// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package gateway

import (
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func openBindingTestStore(t *testing.T, c Config) (*Store, func()) {
	t.Helper()
	s, err := OpenStore(c)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	closeStore := func() {
		once.Do(func() {
			if err := s.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(closeStore)
	return s, closeStore
}

func reserveBindingTestCall(t *testing.T, s *Store, async bool) Record {
	t.Helper()
	op := Operation{ID: "wiki.spaces.get", Risk: "read"}
	r, _, err := s.Reserve(actor("alice"), op, argsSpace("space-a"), "", async)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func bindingTestDB(t *testing.T, c Config) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(c.DataDir, "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAppBindingsProtectQueuedJobsAfterRestart(t *testing.T) {
	for _, field := range []string{"app_id", "organization_id", "brand"} {
		t.Run(field, func(t *testing.T) {
			c := testConfig(t)
			s, closeStore := openBindingTestStore(t, c)
			running := reserveBindingTestCall(t, s, false)
			queued := reserveBindingTestCall(t, s, true)
			closeStore()

			changed := cloneConfigValue(c)
			switch field {
			case "app_id":
				changed.Apps[0].AppID = "cli_other_application"
			case "organization_id":
				changed.Apps[0].OrgID = "other-organization"
			case "brand":
				changed.Apps[0].Brand = "lark"
			}
			if wrongStore, err := OpenStore(changed); err == nil {
				_ = wrongStore.Close()
				t.Fatal("restarted store accepted a different application binding")
			}

			// A rejected startup must neither run queued work nor alter running receipts.
			db := bindingTestDB(t, c)
			for id, want := range map[string]string{running.ID: "running", queued.ID: "queued"} {
				var state string
				if err := db.QueryRow(`SELECT state FROM invocations WHERE id=?`, id).Scan(&state); err != nil {
					t.Fatal(err)
				}
				if state != want {
					t.Fatalf("failed startup changed %s from %s to %s", id, want, state)
				}
			}
			_ = db.Close()

			reopened, _ := openBindingTestStore(t, c)
			r, err := reopened.Record(actor("alice"), running.ID)
			if err != nil || r.State != "unknown" {
				t.Fatalf("valid restart must recover running receipt: %+v, %v", r, err)
			}
			r, err = reopened.Claim(func(p Principal) bool { return p == actor("alice") })
			if err != nil || r.ID != queued.ID || r.Principal.AppID != c.Apps[0].ID {
				t.Fatalf("queued call was not preserved for the original binding: %+v, %v", r, err)
			}
		})
	}
}

func TestAppBindingsAllowSecretRotation(t *testing.T) {
	c := testConfig(t)
	s, closeStore := openBindingTestStore(t, c)
	rotated := cloneConfigValue(c)
	t.Setenv("ROTATED_FEISHU_SECRET", "rotated-fixture-secret")
	rotated.Apps[0].SecretEnv = "ROTATED_FEISHU_SECRET"
	if err := rotated.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureAppBindings(rotated.Apps); err != nil {
		t.Fatalf("secret rotation changed application identity: %v", err)
	}
	closeStore()
	reopened, _ := openBindingTestStore(t, rotated)
	var appID string
	if err := reopened.db.QueryRow(`SELECT app_id FROM app_bindings WHERE connection_id=?`, c.Apps[0].ID).Scan(&appID); err != nil {
		t.Fatal(err)
	}
	if appID != c.Apps[0].AppID {
		t.Fatal("secret rotation replaced the app binding")
	}
}

func TestAppBindingsSurviveConfigurationRemoval(t *testing.T) {
	c := testConfig(t)
	_, closeStore := openBindingTestStore(t, c)
	closeStore()
	removed := cloneConfigValue(c)
	removed.Apps = removed.Apps[1:]
	removed.Clients = removed.Clients[1:]
	s, closeRemoved := openBindingTestStore(t, removed)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM app_bindings`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("removing an app from configuration released its persistent binding")
	}
	closeRemoved()
	changed := cloneConfigValue(c)
	changed.Apps[0].AppID = "cli_reused_connection"
	if wrongStore, err := OpenStore(changed); err == nil {
		_ = wrongStore.Close()
		t.Fatal("removed connection ID was reused for a different application")
	}
	openBindingTestStore(t, c)
}

func TestEnsureAppBindingsIsAtomic(t *testing.T) {
	c := testConfig(t)
	s, _ := openBindingTestStore(t, c)
	added := App{ID: "app-new", AppID: "cli_new", OrgID: "org-new", Brand: "feishu"}
	conflict := c.Apps[0]
	conflict.AppID = "cli_conflict"
	if err := s.EnsureAppBindings([]App{added, conflict}); err == nil {
		t.Fatal("conflicting binding was accepted")
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM app_bindings WHERE connection_id=?`, added.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed binding transaction left a partial insertion")
	}
	if err := s.EnsureAppBindings([]App{added, c.Apps[0]}); err != nil {
		t.Fatalf("failed transaction modified the original binding: %v", err)
	}
}

func createVersion4BindingFixture(t *testing.T, c Config) {
	t.Helper()
	db := bindingTestDB(t, c)
	_, err := db.Exec(`
CREATE TABLE invocations(id TEXT PRIMARY KEY,owner TEXT NOT NULL,principal TEXT NOT NULL,operation TEXT NOT NULL,args TEXT NOT NULL,idem TEXT NOT NULL,hash TEXT NOT NULL,state TEXT NOT NULL,result TEXT NOT NULL,async INTEGER NOT NULL,created INTEGER NOT NULL,updated INTEGER NOT NULL);
CREATE UNIQUE INDEX invocation_idem ON invocations(owner,operation,idem) WHERE idem<>'';
PRAGMA user_version=4;`)
	if err != nil {
		t.Fatal(err)
	}
	p := actor("alice")
	for _, state := range []string{"running", "queued"} {
		id := "legacy-" + state
		r := Result{InvocationID: id, OperationID: "wiki.spaces.get", Status: state}
		_, err = db.Exec(`INSERT INTO invocations VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, p.Owner(), string(encode(p)), r.OperationID, string(encode(argsSpace("space-a"))), "", "fixture-hash", state, string(encode(r)), state == "queued", 1, 1)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAppBindingsMigrateVersion4(t *testing.T) {
	c := testConfig(t)
	createVersion4BindingFixture(t, c)
	s, _ := openBindingTestStore(t, c)
	var version, count int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 5 {
		t.Fatalf("database version=%d, want 5", version)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM app_bindings`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(c.Apps) {
		t.Fatal("legacy migration did not register configured applications")
	}
	for id, want := range map[string]string{"legacy-running": "unknown", "legacy-queued": "queued"} {
		r, err := s.Record(actor("alice"), id)
		if err != nil || r.State != want {
			t.Fatalf("legacy receipt recovery %s: %+v, %v", id, r, err)
		}
	}
	rows, err := s.db.Query(`EXPLAIN QUERY PLAN SELECT ` + recordColumns + ` FROM invocations WHERE state='queued' ORDER BY created,id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	usesQueueIndex := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		usesQueueIndex = usesQueueIndex || strings.Contains(detail, "invocation_queue")
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !usesQueueIndex {
		t.Fatal("queue query still scans historical invocations")
	}
}

func TestAppBindingsMigrationFailurePreservesVersion4(t *testing.T) {
	c := testConfig(t)
	createVersion4BindingFixture(t, c)
	conflict := c.Apps[0]
	conflict.AppID = "cli_conflict"
	c.Apps = append(c.Apps, conflict)
	if s, err := OpenStore(c); err == nil {
		_ = s.Close()
		t.Fatal("migration accepted conflicting app bindings")
	}
	db := bindingTestDB(t, c)
	var version, count int
	var state string
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('app_bindings','invocation_queue')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT state FROM invocations WHERE id='legacy-running'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if version != 4 || count != 0 || state != "running" {
		t.Fatalf("failed migration changed database: version=%d new_objects=%d running_state=%s", version, count, state)
	}
}
