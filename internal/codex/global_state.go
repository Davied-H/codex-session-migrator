package codex

import (
	"encoding/json"
	"os"
)

const OrdinaryConversationGroup = "普通对话"

type GlobalIndex struct {
	Projectless            map[string]bool
	ThreadWorkspaceRoot    map[string]string
	ProjectRoots           []string
	HeartbeatPermissionsID map[string]any
}

func ReadGlobalIndex(path string) (GlobalIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GlobalIndex{
			Projectless:         map[string]bool{},
			ThreadWorkspaceRoot: map[string]string{},
			ProjectRoots:        nil,
		}, err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return GlobalIndex{}, err
	}
	idx := GlobalIndex{
		Projectless:            map[string]bool{},
		ThreadWorkspaceRoot:    map[string]string{},
		ProjectRoots:           nil,
		HeartbeatPermissionsID: map[string]any{},
	}
	if ids, ok := root["projectless-thread-ids"].([]any); ok {
		for _, raw := range ids {
			if id, ok := raw.(string); ok {
				idx.Projectless[id] = true
			}
		}
	}
	if hints, ok := root["thread-workspace-root-hints"].(map[string]any); ok {
		for id, raw := range hints {
			if root, ok := raw.(string); ok {
				idx.ThreadWorkspaceRoot[id] = root
			}
		}
	}
	addProjectRoots := func(key string) {
		if roots, ok := root[key].([]any); ok {
			for _, raw := range roots {
				if root, ok := raw.(string); ok {
					idx.addProjectRoot(root)
				}
			}
		}
	}
	addProjectRoots("project-order")
	addProjectRoots("electron-saved-workspace-roots")
	addProjectRoots("active-workspace-roots")
	if atom, ok := root["electron-persisted-atom-state"].(map[string]any); ok {
		if perms, ok := atom["heartbeat-thread-permissions-by-id"].(map[string]any); ok {
			idx.HeartbeatPermissionsID = perms
		}
	}
	return idx, nil
}

func (idx GlobalIndex) ProjectRoot(t Thread) string {
	if idx.Projectless[t.ID] {
		return OrdinaryConversationGroup
	}
	if root := idx.ThreadWorkspaceRoot[t.ID]; root != "" {
		return root
	}
	return OrdinaryConversationGroup
}

func (idx *GlobalIndex) addProjectRoot(root string) {
	if root == "" {
		return
	}
	for _, existing := range idx.ProjectRoots {
		if existing == root {
			return
		}
	}
	idx.ProjectRoots = append(idx.ProjectRoots, root)
}

func AddGlobalThread(path, oldID, newID, cwd string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	if stringArrayContains(root, "projectless-thread-ids", oldID) {
		addStringToArray(root, "projectless-thread-ids", newID)
	}
	copyMapValue(root, "thread-workspace-root-hints", oldID, newID, cwd)
	if atom, ok := root["electron-persisted-atom-state"].(map[string]any); ok {
		copyMapValue(atom, "heartbeat-thread-permissions-by-id", oldID, newID, nil)
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o600)
}

func RemoveGlobalThreads(path string, ids []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	idSet := map[string]bool{}
	for _, id := range ids {
		idSet[id] = true
	}
	removeStringsFromArray(root, "projectless-thread-ids", idSet)
	removeMapKeys(root, "thread-workspace-root-hints", idSet)
	if atom, ok := root["electron-persisted-atom-state"].(map[string]any); ok {
		removeMapKeys(atom, "heartbeat-thread-permissions-by-id", idSet)
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o600)
}

func addStringToArray(root map[string]any, key, value string) {
	raw, _ := root[key].([]any)
	for _, v := range raw {
		if s, ok := v.(string); ok && s == value {
			return
		}
	}
	root[key] = append(raw, value)
}

func stringArrayContains(root map[string]any, key, value string) bool {
	raw, _ := root[key].([]any)
	for _, v := range raw {
		if s, ok := v.(string); ok && s == value {
			return true
		}
	}
	return false
}

func copyMapValue(root map[string]any, key, oldID, newID string, fallback any) {
	raw, _ := root[key].(map[string]any)
	if raw == nil {
		raw = map[string]any{}
		root[key] = raw
	}
	if _, exists := raw[newID]; exists {
		return
	}
	if v, exists := raw[oldID]; exists {
		raw[newID] = v
		return
	}
	if fallback != nil {
		raw[newID] = fallback
	}
}

func removeStringsFromArray(root map[string]any, key string, values map[string]bool) {
	raw, _ := root[key].([]any)
	if raw == nil {
		return
	}
	next := make([]any, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if ok && values[s] {
			continue
		}
		next = append(next, v)
	}
	root[key] = next
}

func removeMapKeys(root map[string]any, key string, values map[string]bool) {
	raw, _ := root[key].(map[string]any)
	for id := range values {
		delete(raw, id)
	}
}
