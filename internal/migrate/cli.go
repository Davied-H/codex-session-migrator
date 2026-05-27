package migrate

import (
	"fmt"
	"strings"

	"codex-session-migrator/internal/codex"
)

type CLIOptions struct {
	FromProvider string
	Target       string
	Mode         string
	IDsCSV       string
	DryRun       bool
}

func RunCLI(paths codex.Paths, opts CLIOptions) (string, error) {
	ids := splitCSV(opts.IDsCSV)
	if len(ids) == 0 && opts.FromProvider != "" {
		db, err := codex.OpenDB(paths)
		if err != nil {
			return "", err
		}
		defer db.Close()
		threads, err := codex.ListThreads(db, opts.FromProvider, "", false, false, 0)
		if err != nil {
			return "", err
		}
		for _, t := range threads {
			ids = append(ids, t.ID)
		}
	}
	res, err := Run(paths, Options{
		IDs: ids, Target: opts.Target, Mode: Mode(opts.Mode), DryRun: opts.DryRun, RequireFrom: opts.FromProvider,
	})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if opts.DryRun {
		b.WriteString("dry-run writes:\n")
	} else {
		b.WriteString("migration completed:\n")
	}
	for _, line := range res.Lines {
		fmt.Fprintln(&b, "- "+line)
	}
	return b.String(), nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
