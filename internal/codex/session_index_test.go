package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSessionNamesUsesLatestEntryForID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session_index.jsonl")
	body := `{"id":"a","thread_name":"old"}` + "\n" +
		`{"id":"b","thread_name":"other"}` + "\n" +
		`{"id":"a","thread_name":"new"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	names, err := ReadSessionNames(path)
	if err != nil {
		t.Fatal(err)
	}
	if names["a"] != "new" {
		t.Fatalf("name a = %q, want new", names["a"])
	}
	if names["b"] != "other" {
		t.Fatalf("name b = %q, want other", names["b"])
	}
}
