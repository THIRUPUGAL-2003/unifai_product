package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/raksha/raksha/framework/configstore"
	"github.com/raksha/raksha/framework/configstore/tables"
)

const (
	limitKindBudget    = "budget"
	limitKindRateLimit = "rate_limit"
)

// accessProfileLimitSnap records a VK budget (per reset duration) or rate limit as it was
// before an access profile first overwrote it, so removing the profile restores it
// instead of deleting a limit the key already had.
type accessProfileLimitSnap struct {
	ProfileID uint   `json:"profile_id"`
	VKID      string `json:"vk_id"`
	Kind      string `json:"kind"`
	Key       string `json:"key"` // budget reset duration; "" for the rate limit

	PrevMaxLimit float64 `json:"prev_max_limit,omitempty"`

	PrevRequestMaxLimit      *int64  `json:"prev_request_max_limit,omitempty"`
	PrevRequestResetDuration *string `json:"prev_request_reset_duration,omitempty"`
	PrevTokenMaxLimit        *int64  `json:"prev_token_max_limit,omitempty"`
	PrevTokenResetDuration   *string `json:"prev_token_reset_duration,omitempty"`
}

var accessProfileLimitsMu sync.Mutex

func loadAccessProfileLimitSnaps(ctx context.Context, ws configstore.WorkspaceStore) ([]accessProfileLimitSnap, error) {
	row, err := ws.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingAccessProfileLimits)
	if errors.Is(err, configstore.ErrNotFound) || (err == nil && (row == nil || row.Data == "")) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var snaps []accessProfileLimitSnap
	if err := json.Unmarshal([]byte(row.Data), &snaps); err != nil {
		return nil, err
	}
	return snaps, nil
}

func saveAccessProfileLimitSnaps(ctx context.Context, ws configstore.WorkspaceStore, snaps []accessProfileLimitSnap) error {
	data, err := json.Marshal(snaps)
	if err != nil {
		return err
	}
	return ws.UpsertWorkspaceSetting(ctx, configstore.WorkspaceSettingAccessProfileLimits, string(data))
}

func findLimitSnap(snaps []accessProfileLimitSnap, profileID uint, vkID, kind, key string) int {
	for i, s := range snaps {
		if s.ProfileID == profileID && s.VKID == vkID && s.Kind == kind && s.Key == key {
			return i
		}
	}
	return -1
}

// recordLimitSnap stores the first pre-profile value for a target; later applies keep it.
func recordLimitSnap(ctx context.Context, store configstore.ConfigStore, snap accessProfileLimitSnap) error {
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil || snap.ProfileID == 0 {
		return nil
	}
	accessProfileLimitsMu.Lock()
	defer accessProfileLimitsMu.Unlock()
	snaps, err := loadAccessProfileLimitSnaps(ctx, ws)
	if err != nil {
		return err
	}
	if findLimitSnap(snaps, snap.ProfileID, snap.VKID, snap.Kind, snap.Key) >= 0 {
		return nil
	}
	return saveAccessProfileLimitSnaps(ctx, ws, append(snaps, snap))
}

// takeLimitSnap returns and removes the snapshot for a target, if any.
func takeLimitSnap(ctx context.Context, store configstore.ConfigStore, profileID uint, vkID, kind, key string) (*accessProfileLimitSnap, error) {
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil {
		return nil, nil
	}
	accessProfileLimitsMu.Lock()
	defer accessProfileLimitsMu.Unlock()
	snaps, err := loadAccessProfileLimitSnaps(ctx, ws)
	if err != nil {
		return nil, err
	}
	idx := findLimitSnap(snaps, profileID, vkID, kind, key)
	if idx < 0 {
		return nil, nil
	}
	snap := snaps[idx]
	snaps = append(snaps[:idx], snaps[idx+1:]...)
	if err := saveAccessProfileLimitSnaps(ctx, ws, snaps); err != nil {
		return nil, err
	}
	return &snap, nil
}

func rateLimitSnapFrom(profileID uint, vkID string, rl *tables.TableRateLimit) accessProfileLimitSnap {
	return accessProfileLimitSnap{
		ProfileID: profileID, VKID: vkID, Kind: limitKindRateLimit,
		PrevRequestMaxLimit: rl.RequestMaxLimit, PrevRequestResetDuration: rl.RequestResetDuration,
		PrevTokenMaxLimit: rl.TokenMaxLimit, PrevTokenResetDuration: rl.TokenResetDuration,
	}
}
