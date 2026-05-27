package main

import (
	"flag"
	"fmt"
	"os"

	"codex-session-migrator/internal/codex"
	"codex-session-migrator/internal/migrate"
	"codex-session-migrator/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	var (
		codexHome = flag.String("codex-home", codex.DefaultHome(), "Codex home directory")
		from      = flag.String("from", "", "source provider for non-interactive migration")
		to        = flag.String("to", "", "target provider")
		mode      = flag.String("mode", "retag", "migration mode: retag or clone")
		ids       = flag.String("ids", "", "comma-separated thread ids")
		dryRun    = flag.Bool("dry-run", false, "print planned writes without applying")
		rollback  = flag.String("rollback", "", "restore a snapshot directory")
	)
	flag.Parse()

	paths := codex.NewPaths(*codexHome)
	if *rollback != "" {
		if err := migrate.Rollback(paths, *rollback); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("rollback completed:", *rollback)
		return
	}

	if *ids != "" || *from != "" || *to != "" {
		report, err := migrate.RunCLI(paths, migrate.CLIOptions{
			FromProvider: *from,
			Target:       *to,
			Mode:         *mode,
			IDsCSV:       *ids,
			DryRun:       *dryRun,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(report)
		return
	}

	p := tea.NewProgram(tui.New(paths), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
