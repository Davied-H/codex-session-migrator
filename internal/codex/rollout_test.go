package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateRolloutProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-old.jsonl")
	body := `{"type":"session_meta","payload":{"id":"old","model_provider":"openai"}}` + "\n" +
		`{"type":"event","payload":{"text":"keep"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := UpdateRolloutProvider(path, "sub2api"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRolloutProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "sub2api" {
		t.Fatalf("provider = %q, want sub2api", got)
	}
}

func TestCloneRolloutReplacesIDAndProvider(t *testing.T) {
	dir := t.TempDir()
	oldID := "11111111-1111-4111-8111-111111111111"
	newID := "22222222-2222-4222-8222-222222222222"
	path := filepath.Join(dir, "rollout-2026-"+oldID+".jsonl")
	body := `{"type":"session_meta","payload":{"id":"` + oldID + `","model_provider":"openai"}}` + "\n" +
		`{"type":"event","payload":{"thread_id":"` + oldID + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	newPath, err := CloneRollout(path, oldID, newID, "custom")
	if err != nil {
		t.Fatal(err)
	}
	if newPath == path {
		t.Fatal("clone reused old path")
	}
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == body || !strings.Contains(string(data), newID) || strings.Contains(string(data), oldID) {
		t.Fatalf("clone did not replace ids:\n%s", string(data))
	}
	got, err := ReadRolloutProvider(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != "custom" {
		t.Fatalf("provider = %q, want custom", got)
	}
}

func TestReadConversationInfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	body := `{"type":"session_meta","payload":{"id":"id","model_provider":"openai"}}` + "\n" +
		`{"type":"response_item","payload":{"item":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}}` + "\n" +
		`{"type":"response_item","payload":{"item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"world"}]}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := ReadConversationInfo(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if info.LineCount != 3 || info.UserMessages != 1 || info.AgentMessages != 1 || len(info.Items) != 2 {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info.Items[0].Role != "user" || info.Items[0].Text != "hello" {
		t.Fatalf("unexpected first item: %+v", info.Items[0])
	}
}

func TestDisplayThreadTitleSingleLine(t *testing.T) {
	got := DisplayThreadTitle(Thread{
		Title:   "ERROR:CONSOLE:1 Cannot read properties\n    at App.tsx:68:52\tmore",
		Preview: "unused",
	})
	want := "ERROR:CONSOLE:1 Cannot read properties at App.tsx:68:52 more"
	if got != want {
		t.Fatalf("DisplayThreadTitle = %q, want %q", got, want)
	}

	got = DisplayThreadTitle(Thread{Preview: "preview line\r\nnext line"})
	want = "preview line next line"
	if got != want {
		t.Fatalf("DisplayThreadTitle preview = %q, want %q", got, want)
	}

	got = DisplayThreadTitle(Thread{Title: "\n\t", Preview: "  "})
	want = "(无标题)"
	if got != want {
		t.Fatalf("DisplayThreadTitle empty = %q, want %q", got, want)
	}
}

func TestWriteConversationMarkdown(t *testing.T) {
	dir := t.TempDir()
	rollout := filepath.Join(dir, "rollout.jsonl")
	body := `{"type":"session_meta","payload":{"id":"id","model_provider":"openai"}}` + "\n" +
		`{"type":"response_item","payload":{"item":{"type":"message","role":"developer","content":[{"type":"input_text","text":"system rules"}]}}}` + "\n" +
		`{"type":"response_item","payload":{"item":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}}` + "\n" +
		`{"type":"response_item","payload":{"item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"world"}]}}}` + "\n" +
		`{"type":"response_item","payload":{"item":{"type":"function_call","name":"lookup"}}}` + "\n" +
		`{"type":"event","payload":{"text":"runtime event"}}` + "\n"
	if err := os.WriteFile(rollout, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := WriteConversationMarkdown(filepath.Join(dir, "details"), Thread{
		ID:            "019e5e8b-2eb9-7461-9dfa-979f8e4ec932",
		RolloutPath:   rollout,
		UpdatedAt:     1770000000,
		ModelProvider: "openai",
		CWD:           "/tmp/project",
		Title:         "Hello Markdown",
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(path) != ".md" {
		t.Fatalf("markdown path = %q, want .md", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	md := string(data)
	for _, want := range []string{"# Hello Markdown", "Provider: `openai`", "### user", "hello", "### assistant", "world"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
	for _, unwanted := range []string{"### developer", "system rules", "### tool", "lookup", "### event", "runtime event"} {
		if strings.Contains(md, unwanted) {
			t.Fatalf("markdown should hide %q:\n%s", unwanted, md)
		}
	}
}

func TestWriteConversationMarkdownUsesSingleLineTitle(t *testing.T) {
	dir := t.TempDir()
	rollout := filepath.Join(dir, "rollout.jsonl")
	body := `{"type":"response_item","payload":{"item":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}}` + "\n"
	if err := os.WriteFile(rollout, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := WriteConversationMarkdown(filepath.Join(dir, "details"), Thread{
		ID:            "019e5e8b-2eb9-7461-9dfa-979f8e4ec932",
		RolloutPath:   rollout,
		UpdatedAt:     1770000000,
		ModelProvider: "openai",
		CWD:           "/tmp/project",
		Title:         "Request Autofill.enable failed\n    at App.tsx:68:52",
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	md := string(data)
	if !strings.Contains(md, "# Request Autofill.enable failed at App.tsx:68:52\n\n") {
		t.Fatalf("markdown title should be single-line:\n%s", md)
	}
	if strings.Contains(md, "# Request Autofill.enable failed\n") {
		t.Fatalf("markdown title leaked newline:\n%s", md)
	}
}
