package codex

import "testing"

func TestDesktopRunningFromCommLinesDetectsMainApp(t *testing.T) {
	lines := []string{
		"/Applications/Codex.app/Contents/MacOS/Codex",
	}

	if !desktopRunningFromCommLines(lines) {
		t.Fatal("expected Codex main app process to be detected")
	}
}

func TestDesktopRunningFromCommLinesIgnoresHelperProcesses(t *testing.T) {
	lines := []string{
		"/Applications/Codex.app/Contents/Frameworks/Codex Helper.app/Contents/MacOS/Codex Helper",
		"/Applications/Codex.app/Contents/Frameworks/Codex Helper (Renderer).app/Contents/MacOS/Codex Helper (Renderer)",
		"/Applications/Codex.app/Contents/Frameworks/Codex Helper (GPU).app/Contents/MacOS/Codex Helper (GPU)",
		"/Applications/Codex.app/Contents/Frameworks/Squirrel.framework/Versions/A/Resources/crashpad_handler",
		"codex-session-migrator",
	}

	if desktopRunningFromCommLines(lines) {
		t.Fatal("expected helper and tool processes to be ignored")
	}
}
