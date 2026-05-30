package migrate

import (
	"database/sql"
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
	if err := appendSessionIndex(path, codex.Thread{ID: "old", Title: "hello"}, "new"); err != nil {
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

func TestAppendSessionIndexFallsBackWhenEntryMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session_index.jsonl")
	thread := codex.Thread{
		ID:        "missing",
		Title:     "fallback title",
		UpdatedAt: 1770000000,
	}
	if err := appendSessionIndex(path, thread, "new"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `"id":"new"`) || !strings.Contains(got, `"thread_name":"fallback title"`) || !strings.Contains(got, `"updated_at":`) {
		t.Fatalf("unexpected fallback session index:\n%s", got)
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

func TestCloneOrderAppendsInReverseSourceOrder(t *testing.T) {
	threads := []codex.Thread{
		{ID: "newest", UpdatedAt: 300},
		{ID: "middle", UpdatedAt: 200},
		{ID: "oldest", UpdatedAt: 100},
	}
	jobs := cloneOrder(threads)
	var got []string
	for _, job := range jobs {
		got = append(got, job.thread.ID)
	}
	want := []string{"oldest", "middle", "newest"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("clone order = %v, want %v", got, want)
	}
	if jobs[2].index != 0 {
		t.Fatalf("clone order should preserve original manifest index, got %d", jobs[2].index)
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

func TestRunRetagSupportsMultipleSourceProvidersAndSkipsTarget(t *testing.T) {
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
	rollouts := map[string]string{
		"openai-1":  filepath.Join(home, "openai-1.jsonl"),
		"sub2api-1": filepath.Join(home, "sub2api-1.jsonl"),
		"custom-1":  filepath.Join(home, "custom-1.jsonl"),
	}
	for id, path := range rollouts {
		provider := strings.TrimSuffix(id, "-1")
		if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+id+`","model_provider":"`+provider+`"}}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(`insert into threads values (?, ?, 1, 2, 2, '', ?, '/tmp/project', ?, 0, 'user', '')`, id, path, provider, id)
		if err != nil {
			t.Fatal(err)
		}
	}

	res, err := Run(paths, Options{
		IDs:            []string{"openai-1", "sub2api-1", "custom-1"},
		Target:         "openai",
		Mode:           ModeRetag,
		RequireFromAny: []string{"openai", "sub2api", "custom"},
		SnapshotName:   "merge-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Snapshot == "" {
		t.Fatal("expected snapshot")
	}
	gotLines := strings.Join(res.Lines, "\n")
	if !strings.Contains(gotLines, "skip retag openai -> openai openai-1") {
		t.Fatalf("expected target provider skip line, got:\n%s", gotLines)
	}
	for _, id := range []string{"openai-1", "sub2api-1", "custom-1"} {
		var provider string
		if err := db.QueryRow(`select model_provider from threads where id = ?`, id).Scan(&provider); err != nil {
			t.Fatal(err)
		}
		if provider != "openai" {
			t.Fatalf("%s provider = %s, want openai", id, provider)
		}
		rolloutProvider, err := codex.ReadRolloutProvider(rollouts[id])
		if err != nil {
			t.Fatal(err)
		}
		if rolloutProvider != "openai" {
			t.Fatalf("%s rollout provider = %s, want openai", id, rolloutProvider)
		}
	}
}

func TestRunRetagReinsertsRowsInReverseSourceOrder(t *testing.T) {
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
	rows := []struct {
		id        string
		updatedAt int
	}{
		{"newest", 300},
		{"middle", 200},
		{"oldest", 100},
	}
	for _, row := range rows {
		path := filepath.Join(home, row.id+".jsonl")
		if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+row.id+`","model_provider":"openai"}}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(`insert into threads values (?, ?, 1, ?, ?, '', 'openai', '/tmp/project', ?, 0, 'user', '')`,
			row.id, path, row.updatedAt, row.updatedAt*1000, row.id)
		if err != nil {
			t.Fatal(err)
		}
	}

	_, err = Run(paths, Options{
		IDs:          []string{"newest", "middle", "oldest"},
		Target:       "sub2api",
		Mode:         ModeRetag,
		RequireFrom:  "openai",
		SnapshotName: "retag-order",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := rowIDsByRowIDDesc(t, db)
	want := []string{"newest", "middle", "oldest"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rowid desc order = %v, want %v", got, want)
	}
}

func TestReorderProviderSortsExistingRowsByUpdatedTime(t *testing.T) {
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
	rows := []struct {
		id        string
		updatedAt int
	}{
		{"newest", 300},
		{"oldest", 100},
		{"middle", 200},
	}
	for _, row := range rows {
		path := filepath.Join(home, row.id+".jsonl")
		if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+row.id+`","model_provider":"sub2api"}}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(`insert into threads values (?, ?, 1, ?, ?, '', 'sub2api', '/tmp/project', ?, 0, 'user', '')`,
			row.id, path, row.updatedAt, row.updatedAt*1000, row.id)
		if err != nil {
			t.Fatal(err)
		}
	}

	res, err := ReorderProvider(paths, ReorderProviderOptions{Provider: "sub2api"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Snapshot == "" {
		t.Fatal("expected snapshot")
	}
	got := rowIDsByRowIDDesc(t, db)
	want := []string{"newest", "middle", "oldest"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rowid desc order = %v, want %v", got, want)
	}
}

func rowIDsByRowIDDesc(t *testing.T, db interface {
	Query(query string, args ...any) (*sql.Rows, error)
}) []string {
	t.Helper()
	rows, err := db.Query(`select id from threads order by rowid desc`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestRunCLISupportsMultipleSourceProviders(t *testing.T) {
	home := t.TempDir()
	paths := codex.NewPaths(home)
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
	for _, row := range []struct {
		id       string
		provider string
	}{
		{"openai-1", "openai"},
		{"sub2api-1", "sub2api"},
		{"custom-1", "custom"},
	} {
		_, err = db.Exec(`insert into threads values (?, ?, 1, 2, 2, '', ?, '/tmp/project', ?, 0, 'user', '')`,
			row.id, filepath.Join(home, row.id+".jsonl"), row.provider, row.id)
		if err != nil {
			t.Fatal(err)
		}
	}

	report, err := RunCLI(paths, CLIOptions{
		FromProvider: "openai,sub2api",
		Target:       "custom",
		Mode:         string(ModeRetag),
		DryRun:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report, "retag openai -> custom openai-1") ||
		!strings.Contains(report, "retag sub2api -> custom sub2api-1") ||
		strings.Contains(report, "custom-1") {
		t.Fatalf("unexpected report:\n%s", report)
	}
}
