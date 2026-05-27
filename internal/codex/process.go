package codex

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func DesktopRunning() bool {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("ps", "-axo", "comm").Output()
		if err != nil {
			return false
		}
		return desktopRunningFromCommLines(strings.Split(string(out), "\n"))
	}
	out, err := exec.Command("ps", "-e", "-o", "comm=").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "codex")
}

func desktopRunningFromCommLines(lines []string) bool {
	for _, line := range lines {
		comm := strings.TrimSpace(line)
		if isCodexDesktopMainProcess(comm) {
			return true
		}
	}
	return false
}

func isCodexDesktopMainProcess(comm string) bool {
	if filepath.Base(comm) == "Codex" {
		return true
	}

	normalized := filepath.ToSlash(comm)
	return strings.HasSuffix(normalized, "/Codex.app/Contents/MacOS/Codex")
}
