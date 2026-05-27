package migrate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"codex-session-migrator/internal/codex"
)

type ManifestEntry struct {
	OldID       string `json:"old_id"`
	NewID       string `json:"new_id,omitempty"`
	OldProvider string `json:"old_provider"`
	NewProvider string `json:"new_provider"`
	RolloutPath string `json:"rollout_path"`
	NewRollout  string `json:"new_rollout,omitempty"`
}

type Manifest struct {
	Mode      string          `json:"mode"`
	CreatedAt string          `json:"created_at"`
	Entries   []ManifestEntry `json:"entries"`
}

func createSnapshot(paths codex.Paths, name string, files []string, rollouts []string, manifest Manifest) (string, error) {
	dir := filepath.Join(paths.Snapshots, time.Now().Format("20060102-150405")+"-"+name)
	if err := os.MkdirAll(filepath.Join(dir, "rollouts"), 0o700); err != nil {
		return "", err
	}
	for _, f := range files {
		if fileExists(f) {
			if err := copyFile(f, filepath.Join(dir, filepath.Base(f))); err != nil {
				return "", err
			}
		}
	}
	for _, r := range rollouts {
		if fileExists(r) {
			if err := copyFile(r, filepath.Join(dir, "rollouts", filepath.Base(r))); err != nil {
				return "", err
			}
		}
	}
	manifest.CreatedAt = time.Now().Format(time.RFC3339)
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return dir, nil
}

func Rollback(paths codex.Paths, snapshot string) error {
	dir := snapshot
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(paths.Snapshots, snapshot)
	}
	if !fileExists(filepath.Join(dir, "manifest.json")) {
		return fmt.Errorf("snapshot manifest not found: %s", dir)
	}
	for _, name := range []string{"state_5.sqlite", "state_5.sqlite-wal", "state_5.sqlite-shm", "session_index.jsonl", ".codex-global-state.json"} {
		src := filepath.Join(dir, name)
		if fileExists(src) {
			var dst string
			switch name {
			case "state_5.sqlite", "state_5.sqlite-wal", "state_5.sqlite-shm":
				dst = filepath.Join(paths.Home, name)
			case "session_index.jsonl":
				dst = paths.SessionIdx
			case ".codex-global-state.json":
				dst = paths.GlobalState
			}
			if err := copyFile(src, dst); err != nil {
				return err
			}
		}
	}
	return filepath.WalkDir(filepath.Join(dir, "rollouts"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		var manifest Manifest
		data, readErr := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if readErr != nil {
			return readErr
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return err
		}
		for _, e := range manifest.Entries {
			if filepath.Base(e.RolloutPath) == filepath.Base(path) {
				return copyFile(path, e.RolloutPath)
			}
		}
		return nil
	})
}

func backupFiles(paths codex.Paths, includeIndex, includeGlobal bool) []string {
	files := []string{paths.DB, paths.DB + "-wal", paths.DB + "-shm"}
	if includeIndex {
		files = append(files, paths.SessionIdx)
	}
	if includeGlobal {
		files = append(files, paths.GlobalState)
	}
	return files
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
