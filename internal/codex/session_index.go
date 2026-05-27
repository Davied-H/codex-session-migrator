package codex

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

func ReadSessionNames(path string) (map[string]string, error) {
	names := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return names, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var entry struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.ID != "" && strings.TrimSpace(entry.ThreadName) != "" {
			names[entry.ID] = entry.ThreadName
		}
	}
	return names, sc.Err()
}
