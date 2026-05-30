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
	"sort"
	"strings"
	"time"

	"codex-session-migrator/internal/codex"
)

type Mode string

const (
	ModeRetag Mode = "retag"
	ModeClone Mode = "clone"
)

type Options struct {
	IDs            []string
	Target         string
	Mode           Mode
	DryRun         bool
	RequireFrom    string
	RequireFromAny []string
	SnapshotName   string
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

type ReorderProviderOptions struct {
	Provider     string
	SnapshotName string
	DryRun       bool
}

func Run(paths codex.Paths, opts Options) (Result, error) {
	if opts.Target == "" {
		return Result{}, fmt.Errorf("target provider is required")
	}
	if len(opts.IDs) == 0 {
		return Result{}, fmt.Errorf("at least one thread id is required")
	}
	if opts.Mode == "" {
		opts.Mode = ModeClone
	}
	requireFrom := providerSet(append(splitCSV(opts.RequireFrom), opts.RequireFromAny...))
	if opts.Mode == ModeClone && codex.DesktopRunning() && !opts.DryRun {
		return Result{}, fmt.Errorf("clone requires Codex Desktop to be stopped before apply")
	}
	db, err := codex.OpenDB(paths)
	if err != nil {
		return Result{}, err
	}
	defer db.Close()

	result := Result{}
	threads := make([]codex.Thread, 0, len(opts.IDs))
	for _, id := range opts.IDs {
		t, err := codex.GetThread(db, id)
		if err != nil {
			return Result{}, fmt.Errorf("load thread %s: %w", id, err)
		}
		if len(requireFrom) > 0 && !requireFrom[t.ModelProvider] {
			return Result{}, fmt.Errorf("thread %s provider is %s, not one of %s", id, t.ModelProvider, providerSetLabel(requireFrom))
		}
		if opts.Mode == ModeRetag && t.ModelProvider == opts.Target {
			result.Lines = append(result.Lines, fmt.Sprintf("skip retag %s -> %s %s (already target provider)", t.ModelProvider, opts.Target, t.ID))
			continue
		}
		threads = append(threads, t)
	}

	for _, t := range threads {
		line := fmt.Sprintf("%s %s -> %s %s", opts.Mode, t.ModelProvider, opts.Target, t.ID)
		if opts.Mode == ModeClone {
			line += " (new id will be generated)"
		}
		result.Lines = append(result.Lines, line)
	}
	if opts.DryRun || len(threads) == 0 {
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

func ReorderProvider(paths codex.Paths, opts ReorderProviderOptions) (Result, error) {
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
	result.Lines = append(result.Lines, fmt.Sprintf("reorder provider %s: %d sessions", opts.Provider, len(threads)))
	if opts.DryRun {
		return result, nil
	}
	name := opts.SnapshotName
	if name == "" {
		name = "reorder-provider-" + opts.Provider
	}
	rollouts := make([]string, 0, len(threads))
	manifest := Manifest{Mode: "reorder-provider"}
	for _, t := range threads {
		rollouts = append(rollouts, t.RolloutPath)
		manifest.Entries = append(manifest.Entries, ManifestEntry{
			OldID: t.ID, OldProvider: t.ModelProvider, RolloutPath: t.RolloutPath,
		})
	}
	snap, err := createSnapshot(paths, name, backupFiles(paths, false, false), rollouts, manifest)
	if err != nil {
		return Result{}, err
	}
	result.Snapshot = snap
	tx, err := db.Begin()
	if err != nil {
		return result, err
	}
	if err := codex.ReorderProviderThreads(tx, opts.Provider); err != nil {
		_ = tx.Rollback()
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	if integrity, err := codex.Integrity(db); err != nil || integrity != "ok" {
		return result, fmt.Errorf("sqlite integrity_check: %s %v", integrity, err)
	}
	result.Lines = append(result.Lines, "snapshot: "+snap)
	return result, nil
}

func providerSet(names []string) map[string]bool {
	out := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			out[name] = true
		}
	}
	return out
}

func providerSetLabel(set map[string]bool) string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
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
	if err := codex.ReorderProviderThreads(tx, opts.Target); err != nil {
		_ = tx.Rollback()
		return result, err
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
	for _, job := range cloneOrder(threads) {
		i := job.index
		t := job.thread
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
		if err := appendSessionIndex(paths.SessionIdx, t, newID); err != nil {
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
	if err := codex.ReorderProviderThreads(tx, opts.Target); err != nil {
		_ = tx.Rollback()
		removeFiles(created)
		return result, err
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

type cloneJob struct {
	index  int
	thread codex.Thread
}

func cloneOrder(threads []codex.Thread) []cloneJob {
	jobs := make([]cloneJob, 0, len(threads))
	for _, i := range sourceOrderForAppendIndex(threads) {
		t := threads[i]
		jobs = append(jobs, cloneJob{index: i, thread: t})
	}
	return jobs
}

func sourceOrderForAppendIndex(threads []codex.Thread) []int {
	order := make([]int, 0, len(threads))
	for i := len(threads) - 1; i >= 0; i-- {
		order = append(order, i)
	}
	return order
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

func appendSessionIndex(path string, t codex.Thread, newID string) error {
	f, err := os.Open(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			line := sc.Bytes()
			var obj map[string]any
			if err := json.Unmarshal(line, &obj); err != nil {
				continue
			}
			if id, ok := obj["id"].(string); !ok || id != t.ID {
				continue
			}
			obj["id"] = newID
			return appendSessionIndexObject(path, obj)
		}
		if err := sc.Err(); err != nil {
			return err
		}
	}
	return appendSessionIndexObject(path, fallbackSessionIndexEntry(t, newID))
}

func appendSessionIndexObject(path string, obj map[string]any) error {
	out, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	w, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer w.Close()
	_, err = w.Write(append(out, '\n'))
	return err
}

func fallbackSessionIndexEntry(t codex.Thread, newID string) map[string]any {
	entry := map[string]any{
		"id":          newID,
		"thread_name": codex.DisplayThreadTitle(t),
	}
	if t.UpdatedAt > 0 {
		entry["updated_at"] = time.Unix(t.UpdatedAt, 0).UTC().Format(time.RFC3339Nano)
	}
	return entry
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
