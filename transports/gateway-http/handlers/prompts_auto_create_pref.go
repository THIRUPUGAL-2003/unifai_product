package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/gateway/gateway/framework/configstore"
)

// Serializes read-modify-write of the single opt-out row.
var promptAutoCreateOptOutMu sync.Mutex

func loadPromptAutoCreateOptOut(ctx context.Context, ws configstore.WorkspaceStore) (map[string]bool, error) {
	out := map[string]bool{}
	row, err := ws.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingPromptAutoCreateOptOut)
	if errors.Is(err, configstore.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if row == nil || strings.TrimSpace(row.Data) == "" {
		return out, nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(row.Data), &ids); err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// setPromptAutoCreatePreference records the admin's "auto create prompt" choice so later
// team assignments respect it instead of always creating a personal prompt.
func setPromptAutoCreatePreference(ctx context.Context, store configstore.ConfigStore, userID string, enabled bool) error {
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil || userID == "" {
		return errors.New("workspace settings are not available")
	}
	promptAutoCreateOptOutMu.Lock()
	defer promptAutoCreateOptOutMu.Unlock()
	optOut, err := loadPromptAutoCreateOptOut(ctx, ws)
	if err != nil {
		return err
	}
	if optOut[userID] == !enabled {
		return nil
	}
	if enabled {
		delete(optOut, userID)
	} else {
		optOut[userID] = true
	}
	ids := make([]string, 0, len(optOut))
	for id := range optOut {
		ids = append(ids, id)
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return ws.UpsertWorkspaceSetting(ctx, configstore.WorkspaceSettingPromptAutoCreateOptOut, string(raw))
}

// promptAutoCreateDisabled reports whether the user was created with prompt auto-creation off.
func promptAutoCreateDisabled(ctx context.Context, store configstore.ConfigStore, userID string) bool {
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil || userID == "" {
		return false
	}
	optOut, err := loadPromptAutoCreateOptOut(ctx, ws)
	if err != nil {
		return false
	}
	return optOut[userID]
}
