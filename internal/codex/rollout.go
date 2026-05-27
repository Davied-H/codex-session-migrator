package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ConversationInfo struct {
	Path          string
	LineCount     int
	UserMessages  int
	AgentMessages int
	ToolEvents    int
	Items         []ConversationItem
	Truncated     bool
}

type ConversationItem struct {
	Role string
	Text string
}

func UpdateRolloutProvider(path, target string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := bytes.SplitAfter(data, []byte("\n"))
	if len(lines) == 0 || len(bytes.TrimSpace(lines[0])) == 0 {
		return fmt.Errorf("empty rollout: %s", path)
	}
	var first map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(lines[0]), &first); err != nil {
		return fmt.Errorf("parse rollout meta: %w", err)
	}
	payload, ok := first["payload"].(map[string]any)
	if !ok {
		payload = map[string]any{}
		first["payload"] = payload
	}
	payload["model_provider"] = target
	updated, err := json.Marshal(first)
	if err != nil {
		return err
	}
	lines[0] = append(updated, '\n')
	return os.WriteFile(path, bytes.Join(lines, nil), 0o600)
}

func CloneRollout(oldPath, oldID, newID, target string) (string, error) {
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return "", err
	}
	newPath := strings.Replace(oldPath, oldID, newID, 1)
	if newPath == oldPath {
		ext := filepath.Ext(oldPath)
		newPath = strings.TrimSuffix(oldPath, ext) + "-" + newID + ext
	}
	replaced := bytes.ReplaceAll(data, []byte(oldID), []byte(newID))
	if err := os.MkdirAll(filepath.Dir(newPath), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(newPath, replaced, 0o600); err != nil {
		return "", err
	}
	if err := UpdateRolloutProvider(newPath, target); err != nil {
		_ = os.Remove(newPath)
		return "", err
	}
	return newPath, nil
}

func ReadRolloutProvider(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return "", sc.Err()
	}
	var first struct {
		Payload struct {
			ModelProvider string `json:"model_provider"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(sc.Bytes(), &first); err != nil {
		return "", err
	}
	return first.Payload.ModelProvider, nil
}

func ReadConversationInfo(path string, maxItems int) (ConversationInfo, error) {
	info := ConversationInfo{Path: path}
	f, err := os.Open(path)
	if err != nil {
		return info, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		info.LineCount++
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		item, ok := conversationItem(event)
		if !ok {
			continue
		}
		switch item.Role {
		case "user":
			info.UserMessages++
		case "assistant":
			info.AgentMessages++
		default:
			info.ToolEvents++
		}
		if maxItems <= 0 || len(info.Items) < maxItems {
			info.Items = append(info.Items, item)
		} else {
			info.Truncated = true
		}
	}
	return info, sc.Err()
}

func WriteConversationMarkdown(outputDir string, t Thread, maxItems int) (string, error) {
	info, err := ReadConversationInfo(t.RolloutPath, maxItems)
	if err != nil {
		return "", err
	}
	title := DisplayThreadTitle(t)
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return "", err
	}
	name := markdownFilename(t, title)
	path := filepath.Join(outputDir, name)
	if err := os.WriteFile(path, []byte(conversationMarkdown(t, title, info)), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func conversationMarkdown(t Thread, title string, info ConversationInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- ID: `%s`\n", t.ID)
	fmt.Fprintf(&b, "- Provider: `%s`\n", t.ModelProvider)
	fmt.Fprintf(&b, "- Updated: `%s`\n", t.UpdatedString())
	fmt.Fprintf(&b, "- Archived: `%t`\n", t.Archived)
	fmt.Fprintf(&b, "- CWD: `%s`\n", t.CWD)
	fmt.Fprintf(&b, "- Rollout: `%s`\n", t.RolloutPath)
	fmt.Fprintf(&b, "- Lines: `%d`\n", info.LineCount)
	fmt.Fprintf(&b, "- Messages: user `%d`, assistant `%d`, tools/events `%d`\n\n", info.UserMessages, info.AgentMessages, info.ToolEvents)
	b.WriteString("## Conversation\n\n")
	if len(info.Items) == 0 {
		b.WriteString("_rollout 中没有解析到可展示的消息。_\n")
		return b.String()
	}
	displayed := 0
	for _, item := range info.Items {
		if item.Role != "user" && item.Role != "assistant" {
			continue
		}
		role := item.Role
		fmt.Fprintf(&b, "### %s\n\n", role)
		fence := markdownFence(item.Text)
		fmt.Fprintf(&b, "%stext\n%s\n%s\n\n", fence, strings.TrimSpace(item.Text), fence)
		displayed++
	}
	if displayed == 0 {
		b.WriteString("_rollout 中没有解析到可展示的用户或助手消息。_\n")
	}
	if info.Truncated {
		fmt.Fprintf(&b, "_已截断，仅显示前 %d 条可解析消息。_\n", len(info.Items))
	}
	return b.String()
}

func markdownFilename(t Thread, title string) string {
	date := "unknown"
	if t.UpdatedAt > 0 {
		date = strings.ReplaceAll(t.UpdatedString(), ":", "")
		date = strings.ReplaceAll(date, " ", "-")
	}
	id := t.ID
	if len(id) > 8 {
		id = id[:8]
	}
	slug := slugFilename(title)
	if slug == "" {
		slug = "session"
	}
	return fmt.Sprintf("%s-%s-%s.md", date, slug, id)
}

func slugFilename(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
		if b.Len() >= 48 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func markdownFence(text string) string {
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	return fence
}

func conversationItem(event map[string]any) (ConversationItem, bool) {
	payload, _ := event["payload"].(map[string]any)
	item, _ := payload["item"].(map[string]any)
	if item == nil {
		item = payload
	}
	if item == nil {
		return ConversationItem{}, false
	}
	typ, _ := item["type"].(string)
	role, _ := item["role"].(string)
	text := strings.TrimSpace(extractText(item))
	if text == "" {
		return ConversationItem{}, false
	}
	if role == "" {
		role = roleFromType(typ)
	}
	if role == "" {
		role = "event"
	}
	return ConversationItem{Role: role, Text: text}, true
}

func roleFromType(typ string) string {
	switch typ {
	case "message", "reasoning":
		return "assistant"
	case "function_call", "function_call_output", "tool_call", "tool_result":
		return "tool"
	default:
		return ""
	}
}

func extractText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		var parts []string
		for _, e := range x {
			if s := strings.TrimSpace(extractText(e)); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"text", "content", "output_text", "input_text", "summary", "name"} {
			if s := strings.TrimSpace(extractText(x[key])); s != "" {
				return s
			}
		}
	}
	return ""
}
