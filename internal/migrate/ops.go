package migrate

import (
	"bufio"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codex-session-migrator/internal/codex"
)

type Mode string

const (
	ModeRetag Mode = "retag"
	ModeClone Mode = "clone"
)

type Options struct {
	IDs          []string
	Target       string
	Mode         Mode
	DryRun       bool
	RequireFrom  string
	SnapshotName string
}

type Result struct {
	Snapshot string
	Lines    []string
	Entries  []ManifestEntry
}

type ClearProviderOptions struct {
	Provider     string
	SnapshotName string
}

type ClearThreadsOptions struct {
	IDs          []string
	Label        string
	SnapshotName string
}

func Run(paths codex.Paths, opts Options) (Result, error) {
	if opts.Target == "" {
		return Result{}, fmt.Errorf("target provider is required")
	}
	if len(opts.IDs) == 0 {
		return Result{}, fmt.Errorf("at least one thread id is required")
	}
	if opts.Mode == "" {
		opts.Mode = ModeRetag
	}
	if opts.Mode == ModeClone && codex.DesktopRunning() && !opts.DryRun {
		return Result{}, fmt.Errorf("clone requires Codex Desktop to be stopped before apply")
	}
	db, err := codex.OpenDB(paths)
	if err != nil {
		return Result{}, err
	}
	defer db.Close()

	threads := make([]codex.Thread, 0, len(opts.IDs))
	for _, id := range opts.IDs {
		t, err := codex.GetThread(db, id)
		if err != nil {
			return Result{}, fmt.Errorf("load thread %s: %w", id, err)
		}
		if opts.RequireFrom != "" && t.ModelProvider != opts.RequireFrom {
			return Result{}, fmt.Errorf("thread %s provider is %s, not %s", id, t.ModelProvider, opts.RequireFrom)
		}
		threads = append(threads, t)
	}

	result := Result{}
	for _, t := range threads {
		line := fmt.Sprintf("%s %s -> %s %s", opts.Mode, t.ModelProvider, opts.Target, t.ID)
		if opts.Mode == ModeClone {
			line += " (new id will be generated)"
		}
		result.Lines = append(result.Lines, line)
	}
	if opts.DryRun {
		return result, nil
	}

	switch opts.Mode {
	case ModeRetag:
		return retag(paths, db, threads, opts, result)
	case ModeClone:
		return clone(paths, db, threads, opts, result)
	default:
		return Result{}, fmt.Errorf("unknown mode: %s", opts.Mode)
	}
}

func ClearProvider(paths codex.Paths, opts ClearProviderOptions) (Result, error) {
	if strings.TrimSpace(opts.Provider) == "" {
		return Result{}, fmt.Errorf("provider is required")
	}
	db, err := codex.OpenDB(paths)
	if err != nil {
		return Result{}, err
	}
	defer db.Close()
	threads, err := codex.ListThreads(db, opts.Provider, "", true, true, 0)
	if err != nil {
		return Result{}, err
	}
	result := Result{}
	if len(threads) == 0 {
		result.Lines = append(result.Lines, fmt.Sprintf("provider %s 没有 session", opts.Provider))
		return result, nil
	}
	ids := make([]string, len(threads))
	for i, t := range threads {
		ids[i] = t.ID
	}
	label := "provider " + opts.Provider
	name := opts.SnapshotName
	if name == "" {
		name = "clear-provider-" + opts.Provider
	}
	return ClearThreads(paths, ClearThreadsOptions{IDs: ids, Label: label, SnapshotName: name})
}

func ClearThreads(paths codex.Paths, opts ClearThreadsOptions) (Result, error) {
	if len(opts.IDs) == 0 {
		return Result{}, fmt.Errorf("at least one thread id is required")
	}
	db, err := codex.OpenDB(paths)
	if err != nil {
		return Result{}, err
	}
	defer db.Close()
	threads := make([]codex.Thread, 0, len(opts.IDs))
	for _, id := range opts.IDs {
		t, err := codex.GetThread(db, id)
		if err != nil {
			return Result{}, fmt.Errorf("load thread %s: %w", id, err)
		}
		threads = append(threads, t)
	}
	ids := make([]string, len(threads))
	rollouts := make([]string, len(threads))
	manifest := Manifest{Mode: "clear-sessions"}
	for i, t := range threads {
		ids[i] = t.ID
		rollouts[i] = t.RolloutPath
		manifest.Entries = append(manifest.Entries, ManifestEntry{
			OldID: t.ID, OldProvider: t.ModelProvider, RolloutPath: t.RolloutPath,
		})
	}
	name := opts.SnapshotName
	if name == "" {
		name = "clear-sessions-" + shortID(threads[0].ID)
		if len(threads) > 1 {
			name = "clear-sessions-batch"
		}
	}
	snap, err := createSnapshot(paths, name, backupFiles(paths, true, true), rollouts, manifest)
	if err != nil {
		return Result{}, err
	}
	result := Result{}
	result.Snapshot = snap

	tx, err := db.Begin()
	if err != nil {
		return result, err
	}
	if err := codex.DeleteThreads(tx, ids); err != nil {
		_ = tx.Rollback()
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	if err := removeSessionIndexEntries(paths.SessionIdx, ids); err != nil {
		return result, err
	}
	if fileExists(paths.GlobalState) {
		if err := codex.RemoveGlobalThreads(paths.GlobalState, ids); err != nil {
			return result, err
		}
	}
	removeFiles(rollouts)
	if integrity, err := codex.Integrity(db); err != nil || integrity != "ok" {
		return result, fmt.Errorf("sqlite integrity_check: %s %v", integrity, err)
	}
	label := opts.Label
	if label == "" {
		label = "sessions"
	}
	result.Lines = append(result.Lines,
		fmt.Sprintf("cleared %s: %d sessions", label, len(threads)),
		"snapshot: "+snap,
	)
	return result, nil
}

func retag(paths codex.Paths, db *sql.DB, threads []codex.Thread, opts Options, result Result) (Result, error) {
	rollouts := make([]string, len(threads))
	manifest := Manifest{Mode: string(ModeRetag)}
	for i, t := range threads {
		rollouts[i] = t.RolloutPath
		manifest.Entries = append(manifest.Entries, ManifestEntry{
			OldID: t.ID, OldProvider: t.ModelProvider, NewProvider: opts.Target, RolloutPath: t.RolloutPath,
		})
	}
	name := opts.SnapshotName
	if name == "" {
		if len(threads) == 1 {
			name = "retag-" + shortID(threads[0].ID) + "-to-" + opts.Target
		} else {
			name = fmt.Sprintf("batch-retag-%s-to-%s", threads[0].ModelProvider, opts.Target)
		}
	}
	snap, err := createSnapshot(paths, name, backupFiles(paths, false, false), rollouts, manifest)
	if err != nil {
		return Result{}, err
	}
	result.Snapshot = snap

	tx, err := db.Begin()
	if err != nil {
		return Result{}, err
	}
	for _, t := range threads {
		if err := codex.RetagThread(tx, t.ID, opts.Target); err != nil {
			_ = tx.Rollback()
			return result, err
		}
		if err := codex.UpdateRolloutProvider(t.RolloutPath, opts.Target); err != nil {
			_ = tx.Rollback()
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	if err := verifyRetag(db, threads, opts.Target); err != nil {
		return result, err
	}
	result.Lines = append(result.Lines, "snapshot: "+snap)
	return result, nil
}

func clone(paths codex.Paths, db *sql.DB, threads []codex.Thread, opts Options, result Result) (Result, error) {
	manifest := Manifest{Mode: string(ModeClone)}
	rollouts := make([]string, len(threads))
	for i, t := range threads {
		rollouts[i] = t.RolloutPath
		manifest.Entries = append(manifest.Entries, ManifestEntry{
			OldID: t.ID, OldProvider: t.ModelProvider, NewProvider: opts.Target, RolloutPath: t.RolloutPath,
		})
	}
	name := opts.SnapshotName
	if name == "" {
		if len(threads) == 1 {
			name = "clone-" + shortID(threads[0].ID) + "-to-" + opts.Target
		} else {
			name = fmt.Sprintf("batch-clone-%s-to-%s", threads[0].ModelProvider, opts.Target)
		}
	}
	snap, err := createSnapshot(paths, name, backupFiles(paths, true, true), rollouts, manifest)
	if err != nil {
		return Result{}, err
	}
	result.Snapshot = snap

	tx, err := db.Begin()
	if err != nil {
		return Result{}, err
	}
	var created []string
	for i, t := range threads {
		newID := newUUID()
		newRollout, err := codex.CloneRollout(t.RolloutPath, t.ID, newID, opts.Target)
		if err != nil {
			_ = tx.Rollback()
			removeFiles(created)
			return result, err
		}
		created = append(created, newRollout)
		if err := codex.CloneThread(tx, t.ID, newID, newRollout, opts.Target); err != nil {
			_ = tx.Rollback()
			removeFiles(created)
			return result, err
		}
		if err := appendSessionIndex(paths.SessionIdx, t.ID, newID); err != nil {
			_ = tx.Rollback()
			removeFiles(created)
			return result, err
		}
		if err := codex.AddGlobalThread(paths.GlobalState, t.ID, newID, t.CWD); err != nil {
			_ = tx.Rollback()
			removeFiles(created)
			return result, err
		}
		entry := &manifest.Entries[i]
		entry.NewID = newID
		entry.NewRollout = newRollout
		result.Entries = append(result.Entries, *entry)
	}
	if err := tx.Commit(); err != nil {
		removeFiles(created)
		return result, err
	}
	if err := writeManifest(snap, manifest); err != nil {
		return result, err
	}
	result.Lines = append(result.Lines, "snapshot: "+snap)
	for _, e := range result.Entries {
		result.Lines = append(result.Lines, e.OldID+" cloned as "+e.NewID)
	}
	return result, nil
}

func verifyRetag(db *sql.DB, threads []codex.Thread, target string) error {
	for _, t := range threads {
		got, err := codex.GetThread(db, t.ID)
		if err != nil {
			return err
		}
		if got.ModelProvider != target {
			return fmt.Errorf("db verify failed for %s: %s", t.ID, got.ModelProvider)
		}
		p, err := codex.ReadRolloutProvider(t.RolloutPath)
		if err != nil {
			return err
		}
		if p != target {
			return fmt.Errorf("rollout verify failed for %s: %s", t.ID, p)
		}
	}
	if integrity, err := codex.Integrity(db); err != nil || integrity != "ok" {
		return fmt.Errorf("sqlite integrity_check: %s %v", integrity, err)
	}
	return nil
}

func appendSessionIndex(path, oldID, newID string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, oldID) {
			var obj map[string]any
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				return err
			}
			obj["id"] = newID
			out, _ := json.Marshal(obj)
			w, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			defer w.Close()
			_, err = w.Write(append(out, '\n'))
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return fmt.Errorf("session_index entry not found for %s", oldID)
}

func removeSessionIndexEntries(path string, ids []string) error {
	if !fileExists(path) {
		return nil
	}
	idSet := map[string]bool{}
	for _, id := range ids {
		idSet[id] = true
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var kept [][]byte
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err == nil {
			if id, ok := obj["id"].(string); ok && idSet[id] {
				continue
			}
		}
		kept = append(kept, append([]byte{}, line...))
	}
	if err := sc.Err(); err != nil {
		return err
	}
	var out []byte
	for _, line := range kept {
		out = append(out, line...)
		out = append(out, '\n')
	}
	return os.WriteFile(path, out, 0o600)
}

func writeManifest(snapshot string, manifest Manifest) error {
	data, _ := json.MarshalIndent(manifest, "", "  ")
	return os.WriteFile(filepath.Join(snapshot, "manifest.json"), append(data, '\n'), 0o600)
}

func removeFiles(paths []string) {
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b[:])
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
