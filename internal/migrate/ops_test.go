package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codex-session-migrator/internal/codex"
)

func TestAppendSessionIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session_index.jsonl")
	body := `{"id":"old","thread_name":"hello","updated_at":"2026-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := appendSessionIndex(path, "old", "new"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `"id":"old"`) || !strings.Contains(got, `"id":"new"`) {
		t.Fatalf("unexpected session index:\n%s", got)
	}
}

func TestRemoveSessionIndexEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session_index.jsonl")
	body := `{"id":"remove","thread_name":"old"}` + "\n" +
		`{"id":"keep","thread_name":"new"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeSessionIndexEntries(path, []string{"remove"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, `"id":"remove"`) || !strings.Contains(got, `"id":"keep"`) {
		t.Fatalf("unexpected session index:\n%s", got)
	}
}

func TestClearProvider(t *testing.T) {
	home := t.TempDir()
	paths := codex.NewPaths(home)
	if err := os.MkdirAll(paths.Snapshots, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := codex.OpenDB(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`create table threads (
		id text primary key,
		rollout_path text,
		created_at integer,
		updated_at integer,
		updated_at_ms integer,
		source text,
		model_provider text,
		cwd text,
		title text,
		archived integer,
		thread_source text,
		preview text
	)`)
	if err != nil {
		t.Fatal(err)
	}
	removeRollout := filepath.Join(home, "remove.jsonl")
	keepRollout := filepath.Join(home, "keep.jsonl")
	if err := os.WriteFile(removeRollout, []byte(`{"type":"session_meta","payload":{"id":"remove","model_provider":"old"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keepRollout, []byte(`{"type":"session_meta","payload":{"id":"keep","model_provider":"other"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`insert into threads values
		('remove', ?, 1, 2, 2, '', 'old', '/tmp/remove', 'remove title', 0, 'user', ''),
		('keep', ?, 1, 2, 2, '', 'other', '/tmp/keep', 'keep title', 0, 'user', '')`, removeRollout, keepRollout)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.SessionIdx, []byte(`{"id":"remove"}`+"\n"+`{"id":"keep"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.GlobalState, []byte(`{
	  "projectless-thread-ids": ["remove", "keep"],
	  "thread-workspace-root-hints": {"remove": "/tmp/remove", "keep": "/tmp/keep"}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := ClearProvider(paths, ClearProviderOptions{Provider: "old", SnapshotName: "test-clear"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Snapshot == "" {
		t.Fatal("expected snapshot")
	}
	if _, err := os.Stat(removeRollout); !os.IsNotExist(err) {
		t.Fatalf("removed rollout still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(keepRollout); err != nil {
		t.Fatalf("keep rollout missing: %v", err)
	}
	var count int
	if err := db.QueryRow(`select count(*) from threads where id = 'remove'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("removed thread count = %d", count)
	}
	if err := db.QueryRow(`select count(*) from threads where id = 'keep'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("kept thread count = %d", count)
	}
	idxData, _ := os.ReadFile(paths.SessionIdx)
	if strings.Contains(string(idxData), `"remove"`) || !strings.Contains(string(idxData), `"keep"`) {
		t.Fatalf("unexpected session index:\n%s", string(idxData))
	}
	global, err := codex.ReadGlobalIndex(paths.GlobalState)
	if err != nil {
		t.Fatal(err)
	}
	if global.Projectless["remove"] || global.ThreadWorkspaceRoot["remove"] != "" {
		t.Fatalf("global state still contains removed id: %+v", global)
	}
}
