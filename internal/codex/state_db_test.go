package codex

import (
	"database/sql"
	"testing"
)

func TestListThreadsHidesSubagentSourceWhenSubagentsDisabled(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`create table threads (
		id text primary key,
		rollout_path text not null,
		created_at integer not null,
		updated_at integer not null,
		source text not null,
		model_provider text not null,
		cwd text not null,
		title text not null,
		archived integer not null default 0,
		thread_source text,
		preview text not null default '',
		updated_at_ms integer
	)`)
	if err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		id           string
		source       string
		threadSource string
	}{
		{"user", "vscode", "user"},
		{"guardian-empty-thread-source", `{"subagent":{"other":"guardian"}}`, ""},
		{"guardian-subagent-thread-source", `{"subagent":{"other":"guardian"}}`, "subagent"},
	}
	for i, row := range rows {
		_, err = db.Exec(`insert into threads
			(id, rollout_path, created_at, updated_at, source, model_provider, cwd, title, archived, thread_source, preview, updated_at_ms)
			values (?, ?, ?, ?, ?, 'openai', '/tmp/project', ?, 0, ?, '', ?)`,
			row.id, "/tmp/"+row.id+".jsonl", i+1, i+1, row.source, row.id, row.threadSource, i+1)
		if err != nil {
			t.Fatal(err)
		}
	}

	threads, err := ListThreads(db, "openai", "", false, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].ID != "user" {
		t.Fatalf("threads = %+v, want only user", threads)
	}

	threads, err = ListThreads(db, "openai", "", false, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 3 {
		t.Fatalf("threads with subagents = %d, want 3: %+v", len(threads), threads)
	}
}
