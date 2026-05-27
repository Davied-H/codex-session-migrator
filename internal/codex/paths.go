package codex

import (
	"os"
	"path/filepath"
)

type Paths struct {
	Home        string
	DB          string
	SessionIdx  string
	GlobalState string
	Snapshots   string
}

func DefaultHome() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".codex")
}

func NewPaths(home string) Paths {
	return Paths{
		Home:        home,
		DB:          filepath.Join(home, "state_5.sqlite"),
		SessionIdx:  filepath.Join(home, "session_index.jsonl"),
		GlobalState: filepath.Join(home, ".codex-global-state.json"),
		Snapshots:   filepath.Join(home, "session-migrate-snapshots"),
	}
}
