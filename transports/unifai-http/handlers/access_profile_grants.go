package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
)

const (
	grantKindProvider = "provider"
	grantKindMCP      = "mcp"
)

// accessProfileGrant records what a virtual key had for one provider / MCP server before an
// access profile first wrote to it, so removing the profile can put that state back.
type accessProfileGrant struct {
	ProfileID uint   `json:"profile_id"`
	VKID      string `json:"vk_id"`
	Kind      string `json:"kind"`
	Key       string `json:"key"` // provider name or MCP client ID
	Seq       int64  `json:"seq"` // first-apply order, used when several profiles share a row
	Existed   bool   `json:"existed"`

	PrevModels schemas.WhiteList `json:"prev_models,omitempty"`
	PrevWeight *float64          `json:"prev_weight,omitempty"`
	PrevTools  schemas.WhiteList `json:"prev_tools,omitempty"`
	ApplModels schemas.WhiteList `json:"applied_models,omitempty"`
	ApplWeight *float64          `json:"applied_weight,omitempty"`
	ApplTools  schemas.WhiteList `json:"applied_tools,omitempty"`
}

func (g accessProfileGrant) sameTarget(o accessProfileGrant) bool {
	return g.VKID == o.VKID && g.Kind == o.Kind && g.Key == o.Key
}

var accessProfileGrantsMu sync.Mutex

func loadAccessProfileGrants(ctx context.Context, ws configstore.WorkspaceStore) ([]accessProfileGrant, error) {
	row, err := ws.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingAccessProfileGrants)
	if errors.Is(err, configstore.ErrNotFound) || (err == nil && (row == nil || row.Data == "")) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var grants []accessProfileGrant
	if err := json.Unmarshal([]byte(row.Data), &grants); err != nil {
		return nil, err
	}
	return grants, nil
}

func saveAccessProfileGrants(ctx context.Context, ws configstore.WorkspaceStore, grants []accessProfileGrant) error {
	data, err := json.Marshal(grants)
	if err != nil {
		return err
	}
	return ws.UpsertWorkspaceSetting(ctx, configstore.WorkspaceSettingAccessProfileGrants, string(data))
}

func findGrant(grants []accessProfileGrant, profileID uint, vkID, kind, key string) int {
	for i, g := range grants {
		if g.ProfileID == profileID && g.VKID == vkID && g.Kind == kind && g.Key == key {
			return i
		}
	}
	return -1
}

// recordProviderGrants snapshots the VK's current provider rows before the profile writes them.
// The first snapshot per (profile, VK, provider) is kept; later applies only refresh applied values.
func recordProviderGrants(grants []accessProfileGrant, profileID uint, vkID string, existing, desired []tables.TableVirtualKeyProviderConfig) []accessProfileGrant {
	byProvider := make(map[string]tables.TableVirtualKeyProviderConfig, len(existing))
	for _, pc := range existing {
		byProvider[pc.Provider] = pc
	}
	for _, pc := range desired {
		idx := findGrant(grants, profileID, vkID, grantKindProvider, pc.Provider)
		if idx < 0 {
			g := accessProfileGrant{ProfileID: profileID, VKID: vkID, Kind: grantKindProvider, Key: pc.Provider, Seq: nextGrantSeq(grants)}
			if cur, ok := byProvider[pc.Provider]; ok {
				g.Existed = true
				g.PrevModels = slices.Clone(cur.AllowedModels)
				g.PrevWeight = cloneFloat(cur.Weight)
			}
			grants = append(grants, g)
			idx = len(grants) - 1
		}
		grants[idx].ApplModels = slices.Clone(pc.AllowedModels)
		grants[idx].ApplWeight = cloneFloat(pc.Weight)
	}
	return grants
}

func recordMCPGrants(grants []accessProfileGrant, profileID uint, vkID string, existing, desired []tables.TableVirtualKeyMCPConfig) []accessProfileGrant {
	byClient := make(map[uint]tables.TableVirtualKeyMCPConfig, len(existing))
	for _, mc := range existing {
		byClient[mc.MCPClientID] = mc
	}
	for _, mc := range desired {
		key := strconv.FormatUint(uint64(mc.MCPClientID), 10)
		idx := findGrant(grants, profileID, vkID, grantKindMCP, key)
		if idx < 0 {
			g := accessProfileGrant{ProfileID: profileID, VKID: vkID, Kind: grantKindMCP, Key: key, Seq: nextGrantSeq(grants)}
			if cur, ok := byClient[mc.MCPClientID]; ok {
				g.Existed = true
				g.PrevTools = slices.Clone(cur.ToolsToExecute)
			}
			grants = append(grants, g)
			idx = len(grants) - 1
		}
		grants[idx].ApplTools = slices.Clone(mc.ToolsToExecute)
	}
	return grants
}

// nextGrantSeq is strictly increasing even when the clock does not advance between applies.
func nextGrantSeq(grants []accessProfileGrant) int64 {
	seq := time.Now().UnixNano()
	for _, g := range grants {
		if g.Seq >= seq {
			seq = g.Seq + 1
		}
	}
	return seq
}

func cloneFloat(f *float64) *float64 {
	if f == nil {
		return nil
	}
	v := *f
	return &v
}

func floatPtrEqual(a, b *float64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// snapshotAccessProfileGrants persists each VK's current provider / MCP rows before the profile
// overwrites them. It must run before the rows are modified.
func snapshotAccessProfileGrants(ctx context.Context, store configstore.ConfigStore, profileID uint, vkIDs []string, providerItems, mcpItems []map[string]any) error {
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil || profileID == 0 {
		return nil
	}
	accessProfileGrantsMu.Lock()
	defer accessProfileGrantsMu.Unlock()

	grants, err := loadAccessProfileGrants(ctx, ws)
	if err != nil {
		return err
	}
	for _, vkID := range vkIDs {
		if len(providerItems) > 0 {
			existing, err := store.GetVirtualKeyProviderConfigs(ctx, vkID)
			if err != nil {
				continue
			}
			grants = recordProviderGrants(grants, profileID, vkID, existing, providerConfigsFromSpec(providerItems, vkID))
		}
		if len(mcpItems) > 0 {
			existing, err := store.GetVirtualKeyMCPConfigs(ctx, vkID)
			if err != nil {
				continue
			}
			desired, err := mcpConfigsFromSpec(ctx, store, mcpItems, vkID)
			if err != nil {
				return err
			}
			grants = recordMCPGrants(grants, profileID, vkID, existing, desired)
		}
	}
	return saveAccessProfileGrants(ctx, ws, grants)
}

// stillGrantedBy reports which (VK, kind, key) targets the profile version still writes.
func stillGrantedBy(ctx context.Context, store configstore.ConfigStore, after *tables.TableAccessProfile) map[string]bool {
	out := map[string]bool{}
	if after == nil || !after.IsActive {
		return out
	}
	spec := after.Spec()
	providers := providerConfigsFromSpec(specMapSlice(spec, "provider_configs"), "")
	var clientIDs []uint
	for _, item := range specMapSlice(spec, "mcp_servers") {
		if id, err := resolveMCPClientID(ctx, store, item); err == nil && id > 0 {
			clientIDs = append(clientIDs, id)
		}
	}
	for _, vkID := range specStringSlice(spec, "virtual_key_ids") {
		for _, pc := range providers {
			out[vkID+"|"+grantKindProvider+"|"+pc.Provider] = true
		}
		for _, id := range clientIDs {
			out[vkID+"|"+grantKindMCP+"|"+strconv.FormatUint(uint64(id), 10)] = true
		}
	}
	return out
}

// rollbackAccessProfileGrants undoes provider and MCP grants that `before` applied and `after`
// no longer gives (after == nil: profile deleted). When another profile wrote the same row
// later, that profile inherits the original snapshot instead of the row being reverted now.
// Returns the touched VK IDs.
func rollbackAccessProfileGrants(ctx context.Context, store configstore.ConfigStore, before tables.TableAccessProfile, after *tables.TableAccessProfile) ([]string, error) {
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil {
		return nil, nil
	}
	accessProfileGrantsMu.Lock()
	defer accessProfileGrantsMu.Unlock()

	grants, err := loadAccessProfileGrants(ctx, ws)
	if err != nil {
		return nil, err
	}
	kept := stillGrantedBy(ctx, store, after)
	touchedSet := map[string]bool{}
	remaining := make([]accessProfileGrant, 0, len(grants))
	var undo []accessProfileGrant
	for _, g := range grants {
		if g.ProfileID == before.ID && !kept[g.VKID+"|"+g.Kind+"|"+g.Key] {
			undo = append(undo, g)
			continue
		}
		remaining = append(remaining, g)
	}
	if len(undo) == 0 {
		return nil, nil
	}

	for _, g := range undo {
		next := -1
		for i, o := range remaining {
			if o.sameTarget(g) && o.Seq > g.Seq && (next < 0 || o.Seq < remaining[next].Seq) {
				next = i
			}
		}
		if next >= 0 {
			remaining[next].Existed = g.Existed
			remaining[next].PrevModels = g.PrevModels
			remaining[next].PrevWeight = g.PrevWeight
			remaining[next].PrevTools = g.PrevTools
			continue
		}
		changed, err := restoreGrant(ctx, store, g)
		if err != nil {
			return nil, err
		}
		if changed {
			touchedSet[g.VKID] = true
		}
	}

	if err := saveAccessProfileGrants(ctx, ws, remaining); err != nil {
		return nil, err
	}
	touched := make([]string, 0, len(touchedSet))
	for id := range touchedSet {
		touched = append(touched, id)
	}
	return touched, nil
}

// restoreGrant puts one row back to its pre-profile state. Rows the profile created are
// removed; rows it modified are restored only while they still carry the profile's values,
// so later manual edits on the VK win.
func restoreGrant(ctx context.Context, store configstore.ConfigStore, g accessProfileGrant) (bool, error) {
	switch g.Kind {
	case grantKindProvider:
		rows, err := store.GetVirtualKeyProviderConfigs(ctx, g.VKID)
		if err != nil {
			return false, nil
		}
		for _, row := range rows {
			if row.Provider != g.Key {
				continue
			}
			if !g.Existed {
				if err := store.DeleteVirtualKeyProviderConfig(ctx, row.ID); err != nil && !errors.Is(err, configstore.ErrNotFound) {
					return false, err
				}
				return true, nil
			}
			if !slices.Equal(row.AllowedModels, g.ApplModels) || !floatPtrEqual(row.Weight, g.ApplWeight) {
				return false, nil
			}
			row.AllowedModels = slices.Clone(g.PrevModels)
			row.Weight = cloneFloat(g.PrevWeight)
			if err := store.UpdateVirtualKeyProviderConfig(ctx, &row); err != nil {
				return false, err
			}
			return true, nil
		}
	case grantKindMCP:
		clientID, err := strconv.ParseUint(g.Key, 10, 64)
		if err != nil {
			return false, nil
		}
		rows, err := store.GetVirtualKeyMCPConfigs(ctx, g.VKID)
		if err != nil {
			return false, nil
		}
		for _, row := range rows {
			if uint64(row.MCPClientID) != clientID {
				continue
			}
			if !g.Existed {
				if err := store.DeleteVirtualKeyMCPConfig(ctx, row.ID); err != nil && !errors.Is(err, configstore.ErrNotFound) {
					return false, err
				}
				return true, nil
			}
			if !slices.Equal(row.ToolsToExecute, g.ApplTools) {
				return false, nil
			}
			row.ToolsToExecute = slices.Clone(g.PrevTools)
			if err := store.UpdateVirtualKeyMCPConfig(ctx, &row); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// profileHasGrantSnapshots reports whether grants were recorded for the profile; profiles
// applied before snapshots existed fall back to the legacy MCP-only rollback.
func profileHasGrantSnapshots(ctx context.Context, store configstore.ConfigStore, profileID uint) bool {
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil {
		return false
	}
	accessProfileGrantsMu.Lock()
	defer accessProfileGrantsMu.Unlock()
	grants, err := loadAccessProfileGrants(ctx, ws)
	if err != nil {
		return false
	}
	for _, g := range grants {
		if g.ProfileID == profileID {
			return true
		}
	}
	return false
}
