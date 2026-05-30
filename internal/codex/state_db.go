package codex

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type ProviderCount struct {
	Provider string
	Archived bool
	Count    int
}

type Thread struct {
	ID            string
	RolloutPath   string
	CreatedAt     int64
	UpdatedAt     int64
	Source        string
	ModelProvider string
	CWD           string
	Title         string
	Archived      bool
	ThreadSource  string
	Preview       string
}

type Diagnostics struct {
	DBExists         bool
	SessionIndex     bool
	GlobalState      bool
	HasModelProvider bool
	Integrity        string
	CodexRunning     bool
	Writable         bool
	Counts           []ProviderCount
	SubagentCount    int
}

func OpenDB(paths Paths) (*sql.DB, error) {
	db, err := sql.Open("sqlite", paths.DB)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func Diagnose(paths Paths) (Diagnostics, error) {
	var d Diagnostics
	d.DBExists = fileExists(paths.DB)
	d.SessionIndex = fileExists(paths.SessionIdx)
	d.GlobalState = fileExists(paths.GlobalState)
	d.CodexRunning = DesktopRunning()
	d.Writable = writable(paths.Home)
	if !d.DBExists {
		return d, nil
	}
	db, err := OpenDB(paths)
	if err != nil {
		return d, err
	}
	defer db.Close()
	d.HasModelProvider, _ = HasColumn(db, "threads", "model_provider")
	d.Integrity, _ = Integrity(db)
	d.Counts, _ = ProviderCounts(db)
	d.SubagentCount, _ = CountSubagentThreads(db)
	return d, nil
}

func HasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query("pragma table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func Integrity(db *sql.DB) (string, error) {
	var s string
	return s, db.QueryRow("pragma integrity_check").Scan(&s)
}

func ProviderCounts(db *sql.DB) ([]ProviderCount, error) {
	rows, err := db.Query(`select model_provider, archived, count(*) from threads group by model_provider, archived order by model_provider, archived`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProviderCount
	for rows.Next() {
		var p ProviderCount
		var archived int
		if err := rows.Scan(&p.Provider, &archived, &p.Count); err != nil {
			return nil, err
		}
		p.Archived = archived != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

func ListThreads(db *sql.DB, provider, search string, includeArchived, includeSubagents bool, limit int) ([]Thread, error) {
	args := []any{provider}
	where := []string{"model_provider = ?"}
	if !includeArchived {
		where = append(where, "archived = 0")
	}
	if !includeSubagents {
		where = append(where, "(thread_source is null or thread_source = '' or thread_source = 'user')")
		where = append(where, "source not like '{\"subagent\":%'")
	}
	if search != "" {
		where = append(where, "(title like ? or preview like ? or cwd like ?)")
		q := "%" + search + "%"
		args = append(args, q, q, q)
	}
	q := `select id, rollout_path, created_at, updated_at, source, model_provider, cwd, title, archived, coalesce(thread_source,''), preview
		from threads where ` + strings.Join(where, " and ") + ` order by updated_at_ms desc, updated_at desc, id desc`
	if limit > 0 {
		q += fmt.Sprintf(" limit %d", limit)
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Thread
	for rows.Next() {
		var t Thread
		var archived int
		if err := rows.Scan(&t.ID, &t.RolloutPath, &t.CreatedAt, &t.UpdatedAt, &t.Source, &t.ModelProvider, &t.CWD, &t.Title, &archived, &t.ThreadSource, &t.Preview); err != nil {
			return nil, err
		}
		t.Archived = archived != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

func ListArchivedThreads(db *sql.DB) ([]Thread, error) {
	rows, err := db.Query(`select id, rollout_path, created_at, updated_at, source, model_provider, cwd, title, archived, coalesce(thread_source,''), preview
		from threads where archived = 1 order by updated_at_ms desc, updated_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Thread
	for rows.Next() {
		var t Thread
		var archived int
		if err := rows.Scan(&t.ID, &t.RolloutPath, &t.CreatedAt, &t.UpdatedAt, &t.Source, &t.ModelProvider, &t.CWD, &t.Title, &archived, &t.ThreadSource, &t.Preview); err != nil {
			return nil, err
		}
		t.Archived = archived != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

func ListSubagentThreads(db *sql.DB) ([]Thread, error) {
	rows, err := db.Query(`select id, rollout_path, created_at, updated_at, source, model_provider, cwd, title, archived, coalesce(thread_source,''), preview
		from threads where (thread_source is not null and thread_source != '' and thread_source != 'user')
			or source like '{"subagent":%'
		order by updated_at_ms desc, updated_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Thread
	for rows.Next() {
		var t Thread
		var archived int
		if err := rows.Scan(&t.ID, &t.RolloutPath, &t.CreatedAt, &t.UpdatedAt, &t.Source, &t.ModelProvider, &t.CWD, &t.Title, &archived, &t.ThreadSource, &t.Preview); err != nil {
			return nil, err
		}
		t.Archived = archived != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

func CountSubagentThreads(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`select count(*) from threads
		where (thread_source is not null and thread_source != '' and thread_source != 'user')
			or source like '{"subagent":%'`).Scan(&count)
	return count, err
}

func GetThread(db *sql.DB, id string) (Thread, error) {
	rows, err := db.Query(`select id, rollout_path, created_at, updated_at, source, model_provider, cwd, title, archived, coalesce(thread_source,''), preview from threads where id = ?`, id)
	if err != nil {
		return Thread{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return Thread{}, sql.ErrNoRows
	}
	var t Thread
	var archived int
	if err := rows.Scan(&t.ID, &t.RolloutPath, &t.CreatedAt, &t.UpdatedAt, &t.Source, &t.ModelProvider, &t.CWD, &t.Title, &archived, &t.ThreadSource, &t.Preview); err != nil {
		return Thread{}, err
	}
	t.Archived = archived != 0
	return t, rows.Err()
}

func DisplayThreadTitle(t Thread) string {
	title := singleLine(t.Title)
	if title == "" {
		title = singleLine(t.Preview)
	}
	if title == "" {
		return "(无标题)"
	}
	return title
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func RetagThread(tx *sql.Tx, id, target string) error {
	res, err := tx.Exec(`update threads set model_provider = ? where id = ?`, target, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("expected to update 1 row for %s, updated %d", id, n)
	}
	return nil
}

func CloneThread(tx *sql.Tx, oldID, newID, newRollout, target string) error {
	rows, err := tx.Query(`select * from threads where id = ?`, oldID)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	if !rows.Next() {
		return sql.ErrNoRows
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return err
	}
	for i, c := range cols {
		switch c {
		case "id":
			vals[i] = newID
		case "rollout_path":
			vals[i] = newRollout
		case "model_provider":
			vals[i] = target
		}
	}
	holders := make([]string, len(cols))
	for i := range holders {
		holders[i] = "?"
	}
	_, err = tx.Exec(`insert into threads (`+strings.Join(cols, ",")+`) values (`+strings.Join(holders, ",")+`)`, vals...)
	return err
}

func DeleteThreads(tx *sql.Tx, ids []string) error {
	for _, id := range ids {
		res, err := tx.Exec(`delete from threads where id = ?`, id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return fmt.Errorf("expected to delete 1 row for %s, deleted %d", id, n)
		}
	}
	return nil
}

func ReinsertThreads(tx *sql.Tx, ids []string) error {
	for _, id := range ids {
		if err := reinsertThread(tx, id); err != nil {
			return err
		}
	}
	return nil
}

func ReorderProviderThreads(tx *sql.Tx, provider string) error {
	rows, err := tx.Query(`select id from threads where model_provider = ?
		order by case when updated_at_ms is null or updated_at_ms = 0 then updated_at * 1000 else updated_at_ms end asc,
			updated_at asc,
			id asc`, provider)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return ReinsertThreads(tx, ids)
}

func reinsertThread(tx *sql.Tx, id string) error {
	rows, err := tx.Query(`select * from threads where id = ?`, id)
	if err != nil {
		return err
	}
	cols, err := rows.Columns()
	if err != nil {
		rows.Close()
		return err
	}
	if !rows.Next() {
		rows.Close()
		return sql.ErrNoRows
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	res, err := tx.Exec(`delete from threads where id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("expected to delete 1 row for %s, deleted %d", id, n)
	}
	holders := make([]string, len(cols))
	for i := range holders {
		holders[i] = "?"
	}
	_, err = tx.Exec(`insert into threads (`+strings.Join(cols, ",")+`) values (`+strings.Join(holders, ",")+`)`, vals...)
	return err
}

func (t Thread) UpdatedString() string {
	if t.UpdatedAt <= 0 {
		return ""
	}
	return time.Unix(t.UpdatedAt, 0).Format("2006-01-02 15:04")
}

func fileExists(path string) bool {
	_, err := osStat(path)
	return err == nil
}

func writable(path string) bool {
	info, err := osStat(path)
	return err == nil && info.IsDir()
}
