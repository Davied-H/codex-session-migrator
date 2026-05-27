package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAddGlobalThread(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex-global-state.json")
	body := `{
	  "projectless-thread-ids": ["old"],
	  "thread-workspace-root-hints": {"old": "/tmp/work"},
	  "electron-persisted-atom-state": {
	    "heartbeat-thread-permissions-by-id": {"old": {"allow": true}}
	  }
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AddGlobalThread(path, "old", "new", "/fallback"); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	hints := got["thread-workspace-root-hints"].(map[string]any)
	if hints["new"] != "/tmp/work" {
		t.Fatalf("hint = %#v", hints["new"])
	}
	ids := got["projectless-thread-ids"].([]any)
	if ids[len(ids)-1] != "new" {
		t.Fatalf("new id not appended: %#v", ids)
	}
}

func TestAddGlobalThreadKeepsProjectSessionOutOfProjectless(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex-global-state.json")
	body := `{
	  "projectless-thread-ids": [],
	  "thread-workspace-root-hints": {"old": "/tmp/project"}
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AddGlobalThread(path, "old", "new", "/fallback"); err != nil {
		t.Fatal(err)
	}
	idx, err := ReadGlobalIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Projectless["new"] {
		t.Fatalf("project clone should not be projectless: %+v", idx)
	}
	if got := idx.ThreadWorkspaceRoot["new"]; got != "/tmp/project" {
		t.Fatalf("project clone root = %q, want /tmp/project", got)
	}
}

func TestReadGlobalIndexProjectRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex-global-state.json")
	body := `{
	  "projectless-thread-ids": ["ordinary"],
	  "thread-workspace-root-hints": {"project": "/tmp/project"},
	  "project-order": ["/tmp/project", "/tmp/other"],
	  "electron-saved-workspace-roots": ["/tmp/project", "/tmp/saved"],
	  "active-workspace-roots": ["/tmp/active"]
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := ReadGlobalIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := idx.ProjectRoot(Thread{ID: "ordinary", CWD: "/tmp/ignored"}); got != OrdinaryConversationGroup {
		t.Fatalf("ordinary root = %q", got)
	}
	if got := idx.ProjectRoot(Thread{ID: "project", CWD: "/tmp/ignored"}); got != "/tmp/project" {
		t.Fatalf("project root = %q", got)
	}
	if got := idx.ProjectRoot(Thread{ID: "fallback", CWD: "/tmp/fallback"}); got != OrdinaryConversationGroup {
		t.Fatalf("fallback root = %q", got)
	}
	wantProjects := []string{"/tmp/project", "/tmp/other", "/tmp/saved", "/tmp/active"}
	if len(idx.ProjectRoots) != len(wantProjects) {
		t.Fatalf("project roots = %#v", idx.ProjectRoots)
	}
	for i, want := range wantProjects {
		if idx.ProjectRoots[i] != want {
			t.Fatalf("project root %d = %q, want %q", i, idx.ProjectRoots[i], want)
		}
	}
}

func TestRemoveGlobalThreads(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex-global-state.json")
	body := `{
	  "projectless-thread-ids": ["remove", "keep"],
	  "thread-workspace-root-hints": {"remove": "/tmp/remove", "keep": "/tmp/keep"},
	  "electron-persisted-atom-state": {
	    "heartbeat-thread-permissions-by-id": {"remove": {"allow": true}, "keep": {"allow": false}}
	  }
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveGlobalThreads(path, []string{"remove"}); err != nil {
		t.Fatal(err)
	}
	idx, err := ReadGlobalIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Projectless["remove"] || idx.ThreadWorkspaceRoot["remove"] != "" {
		t.Fatalf("removed thread still present: %+v", idx)
	}
	if !idx.Projectless["keep"] || idx.ThreadWorkspaceRoot["keep"] != "/tmp/keep" {
		t.Fatalf("kept thread missing: %+v", idx)
	}
}
