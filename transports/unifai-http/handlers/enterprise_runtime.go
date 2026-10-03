package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
)

func propagateAccessProfile(ctx context.Context, configStore configstore.ConfigStore, profile tables.TableAccessProfile) error {
	if configStore == nil || !profile.IsActive {
		return nil
	}
	spec := profile.Spec()
	vkIDs := specStringSlice(spec, "virtual_key_ids")
	if len(vkIDs) == 0 {
		return nil
	}
	providerConfigs := specMapSlice(spec, "provider_configs")
	mcpServers := specMapSlice(spec, "mcp_servers")
	rateLimitSpec := specMap(spec, "rate_limit")
	budgets := specMapSlice(spec, "budgets")

	if len(providerConfigs) > 0 || len(mcpServers) > 0 {
		if err := snapshotAccessProfileGrants(ctx, configStore, profile.ID, vkIDs, providerConfigs, mcpServers); err != nil {
			return fmt.Errorf("record pre-profile grants: %w", err)
		}
	}

	for _, vkID := range vkIDs {
		vk, err := configStore.GetVirtualKey(ctx, vkID)
		if err != nil || vk == nil {
			continue
		}

		if len(providerConfigs) > 0 {
			if err := applyAccessProfileProviders(ctx, configStore, vk.ID, providerConfigs); err != nil {
				return fmt.Errorf("update virtual key %s providers: %w", vkID, err)
			}
		}
		if len(mcpServers) > 0 {
			if err := applyAccessProfileMCP(ctx, configStore, vk.ID, mcpServers); err != nil {
				return fmt.Errorf("update virtual key %s mcp: %w", vkID, err)
			}
			_ = configStore.ReconcileOauthAfterVKChange(ctx, vk.ID)
			_ = configStore.ReconcileMCPHeadersAfterVKChange(ctx, vk.ID)
		}
		if rateLimitSpec != nil {
			if err := applyAccessProfileRateLimit(ctx, configStore, profile.ID, vk, rateLimitSpec); err != nil {
				return fmt.Errorf("update virtual key %s rate limit: %w", vkID, err)
			}
		}
		if len(budgets) > 0 {
			if err := applyAccessProfileBudgets(ctx, configStore, profile.ID, vk.ID, budgets); err != nil {
				return fmt.Errorf("update virtual key %s budgets: %w", vkID, err)
			}
			if vk.CalendarAligned != profile.CalendarAligned {
				vk.CalendarAligned = profile.CalendarAligned
				if err := configStore.UpdateVirtualKey(ctx, vk); err != nil {
					return fmt.Errorf("update virtual key %s calendar alignment: %w", vkID, err)
				}
			}
		}
	}
	return nil
}

func applyAccessProfileProviders(ctx context.Context, store configstore.ConfigStore, vkID string, items []map[string]any) error {
	desired := providerConfigsFromSpec(items, vkID)
	existing, err := store.GetVirtualKeyProviderConfigs(ctx, vkID)
	if err != nil {
		return err
	}
	byProvider := make(map[string]tables.TableVirtualKeyProviderConfig, len(existing))
	for _, pc := range existing {
		byProvider[pc.Provider] = pc
	}
	kept := make(map[string]bool, len(desired))
	for _, pc := range desired {
		kept[pc.Provider] = true
		if cur, ok := byProvider[pc.Provider]; ok {
			cur.AllowedModels = pc.AllowedModels
			cur.Weight = pc.Weight
			if err := store.UpdateVirtualKeyProviderConfig(ctx, &cur); err != nil {
				return err
			}
			continue
		}
		if err := store.CreateVirtualKeyProviderConfig(ctx, &pc); err != nil {
			return err
		}
	}
	for provider, cur := range byProvider {
		if kept[provider] {
			continue
		}
		// Access profiles only push desired providers; leave unrelated VK
		// providers alone so ad-hoc VK grants are not wiped.
		_ = provider
		_ = cur
	}
	return nil
}

func applyAccessProfileMCP(ctx context.Context, store configstore.ConfigStore, vkID string, items []map[string]any) error {
	desired, err := mcpConfigsFromSpec(ctx, store, items, vkID)
	if err != nil {
		return err
	}
	existing, err := store.GetVirtualKeyMCPConfigs(ctx, vkID)
	if err != nil {
		return err
	}
	byClient := make(map[uint]tables.TableVirtualKeyMCPConfig, len(existing))
	for _, mc := range existing {
		byClient[mc.MCPClientID] = mc
	}
	kept := make(map[uint]bool, len(desired))
	for _, mc := range desired {
		kept[mc.MCPClientID] = true
		if cur, ok := byClient[mc.MCPClientID]; ok {
			cur.ToolsToExecute = mc.ToolsToExecute
			if err := store.UpdateVirtualKeyMCPConfig(ctx, &cur); err != nil {
				return err
			}
			continue
		}
		if err := store.CreateVirtualKeyMCPConfig(ctx, &mc); err != nil {
			return err
		}
	}
	for clientID, cur := range byClient {
		if kept[clientID] {
			continue
		}
		// Same merge semantics as providers: add/update profile MCP grants
		// without deleting unrelated VK MCP rows.
		_ = cur
	}
	return nil
}

func mcpConfigsFromSpec(ctx context.Context, store configstore.ConfigStore, items []map[string]any, vkID string) ([]tables.TableVirtualKeyMCPConfig, error) {
	out := make([]tables.TableVirtualKeyMCPConfig, 0, len(items))
	for _, item := range items {
		clientID, err := resolveMCPClientID(ctx, store, item)
		if err != nil || clientID == 0 {
			continue
		}
		tools := schemas.WhiteList{"*"}
		if names := stringSliceFromAny(item["tools_to_execute"]); len(names) > 0 {
			tools = schemas.WhiteList(names)
		}
		out = append(out, tables.TableVirtualKeyMCPConfig{
			VirtualKeyID:   vkID,
			MCPClientID:    clientID,
			ToolsToExecute: tools,
		})
	}
	return out, nil
}

func resolveMCPClientID(ctx context.Context, store configstore.ConfigStore, item map[string]any) (uint, error) {
	if id := uintFromAny(item["mcp_client_id"]); id > 0 {
		return id, nil
	}
	name := firstString(item, "mcp_client_name", "name", "client_name")
	if name == "" {
		return 0, nil
	}
	client, err := store.GetMCPClientByName(ctx, name)
	if err != nil || client == nil {
		return 0, err
	}
	return client.ID, nil
}

func applyAccessProfileRateLimit(ctx context.Context, store configstore.ConfigStore, profileID uint, vk *tables.TableVirtualKey, spec map[string]any) error {
	rl := rateLimitFromSpec(spec, vk.RateLimitID)
	if vk.RateLimitID != nil && *vk.RateLimitID != "" {
		existing, err := store.GetRateLimit(ctx, *vk.RateLimitID)
		if err == nil && existing != nil {
			if err := recordLimitSnap(ctx, store, rateLimitSnapFrom(profileID, vk.ID, existing)); err != nil {
				return err
			}
			// Merge only what the profile specifies: live counters, reset times and limits
			// the profile does not model must survive every propagation.
			if rl.RequestMaxLimit != nil {
				existing.RequestMaxLimit = rl.RequestMaxLimit
			}
			if rl.RequestResetDuration != nil {
				existing.RequestResetDuration = rl.RequestResetDuration
			}
			if rl.TokenMaxLimit != nil {
				existing.TokenMaxLimit = rl.TokenMaxLimit
			}
			if rl.TokenResetDuration != nil {
				existing.TokenResetDuration = rl.TokenResetDuration
			}
			existing.UpdatedAt = time.Now().UTC()
			return store.UpdateRateLimit(ctx, existing)
		}
		if err != nil && !errors.Is(err, configstore.ErrNotFound) {
			return err
		}
		// The VK points at a rate limit that no longer exists: recreate it under the same ID.
	}
	now := time.Now().UTC()
	rl.TokenLastReset = now
	rl.RequestLastReset = now
	if err := store.CreateRateLimit(ctx, rl); err != nil {
		return err
	}
	vk.RateLimitID = &rl.ID
	return store.UpdateVirtualKey(ctx, vk)
}

func applyAccessProfileBudgets(ctx context.Context, store configstore.ConfigStore, profileID uint, vkID string, items []map[string]any) error {
	desired := budgetsFromSpec(items, vkID)
	vk, err := store.GetVirtualKey(ctx, vkID)
	if err != nil || vk == nil {
		return err
	}
	existingByDuration := make(map[string]tables.TableBudget, len(vk.Budgets))
	for _, b := range vk.Budgets {
		existingByDuration[b.ResetDuration] = b
	}
	for _, b := range desired {
		if cur, ok := existingByDuration[b.ResetDuration]; ok {
			if err := recordLimitSnap(ctx, store, accessProfileLimitSnap{
				ProfileID: profileID, VKID: vkID, Kind: limitKindBudget, Key: b.ResetDuration, PrevMaxLimit: cur.MaxLimit,
			}); err != nil {
				return err
			}
			cur.MaxLimit = b.MaxLimit
			if err := store.UpdateBudget(ctx, &cur); err != nil {
				return err
			}
			continue
		}
		if err := store.CreateBudget(ctx, &b); err != nil {
			return err
		}
	}
	return nil
}

// rollbackAccessProfileMCP removes MCP grants that this profile previously applied.
func rollbackAccessProfileMCP(ctx context.Context, store configstore.ConfigStore, profile tables.TableAccessProfile) error {
	spec := profile.Spec()
	vkIDs := specStringSlice(spec, "virtual_key_ids")
	servers := specMapSlice(spec, "mcp_servers")
	if len(vkIDs) == 0 || len(servers) == 0 {
		return nil
	}
	clientIDs := map[uint]bool{}
	for _, item := range servers {
		id, err := resolveMCPClientID(ctx, store, item)
		if err != nil || id == 0 {
			continue
		}
		clientIDs[id] = true
	}
	if len(clientIDs) == 0 {
		return nil
	}
	for _, vkID := range vkIDs {
		existing, err := store.GetVirtualKeyMCPConfigs(ctx, vkID)
		if err != nil {
			continue
		}
		for _, mc := range existing {
			if clientIDs[mc.MCPClientID] {
				_ = store.DeleteVirtualKeyMCPConfig(ctx, mc.ID)
			}
		}
	}
	return nil
}

// rollbackAccessProfileLimits removes budgets and rate limits the previous profile version
// pushed onto its virtual keys and the new version no longer gives (after == nil means the
// profile was deleted). Rows are only removed while they still carry exactly the profile's
// values, so limits an admin changed on the VK afterwards are kept. Returns touched VK IDs.
func rollbackAccessProfileLimits(ctx context.Context, store configstore.ConfigStore, before tables.TableAccessProfile, after *tables.TableAccessProfile) ([]string, error) {
	if store == nil || !before.IsActive {
		return nil, nil
	}
	beforeSpec := before.Spec()
	beforeBudgets := budgetsFromSpec(specMapSlice(beforeSpec, "budgets"), "")
	beforeRL := specMap(beforeSpec, "rate_limit")
	if len(beforeBudgets) == 0 && beforeRL == nil {
		return nil, nil
	}
	stillOnVK := map[string]bool{}
	afterDurations := map[string]bool{}
	afterHasRL := false
	if after != nil && after.IsActive {
		afterSpec := after.Spec()
		for _, id := range specStringSlice(afterSpec, "virtual_key_ids") {
			stillOnVK[id] = true
		}
		for _, b := range budgetsFromSpec(specMapSlice(afterSpec, "budgets"), "") {
			afterDurations[b.ResetDuration] = true
		}
		afterHasRL = specMap(afterSpec, "rate_limit") != nil
	}

	var touched []string
	for _, vkID := range specStringSlice(beforeSpec, "virtual_key_ids") {
		kept := stillOnVK[vkID]
		vk, err := store.GetVirtualKey(ctx, vkID)
		if err != nil || vk == nil {
			continue
		}
		changed := false
		for _, want := range beforeBudgets {
			if kept && afterDurations[want.ResetDuration] {
				continue
			}
			snap, err := takeLimitSnap(ctx, store, before.ID, vkID, limitKindBudget, want.ResetDuration)
			if err != nil {
				return touched, err
			}
			for _, b := range vk.Budgets {
				if b.ResetDuration == want.ResetDuration && b.MaxLimit == want.MaxLimit {
					if snap != nil {
						// The key had this budget before the profile: put its limit back.
						b.MaxLimit = snap.PrevMaxLimit
						if err := store.UpdateBudget(ctx, &b); err != nil {
							return touched, err
						}
					} else if err := store.DeleteBudget(ctx, b.ID); err != nil && !errors.Is(err, configstore.ErrNotFound) {
						return touched, err
					}
					changed = true
				}
			}
		}
		if beforeRL != nil && !(kept && afterHasRL) && vk.RateLimitID != nil && *vk.RateLimitID != "" {
			snap, err := takeLimitSnap(ctx, store, before.ID, vkID, limitKindRateLimit, "")
			if err != nil {
				return touched, err
			}
			if rl, err := store.GetRateLimit(ctx, *vk.RateLimitID); err == nil && rl != nil && rateLimitMatchesSpec(rl, beforeRL) && snap != nil {
				rl.RequestMaxLimit = snap.PrevRequestMaxLimit
				rl.RequestResetDuration = snap.PrevRequestResetDuration
				rl.TokenMaxLimit = snap.PrevTokenMaxLimit
				rl.TokenResetDuration = snap.PrevTokenResetDuration
				rl.UpdatedAt = time.Now().UTC()
				if err := store.UpdateRateLimit(ctx, rl); err != nil {
					return touched, err
				}
				changed = true
			} else if err == nil && rl != nil && rateLimitMatchesSpec(rl, beforeRL) {
				rlID := *vk.RateLimitID
				vk.RateLimitID = nil
				vk.RateLimit = nil
				if err := store.UpdateVirtualKey(ctx, vk); err != nil {
					return touched, err
				}
				if err := store.DeleteRateLimit(ctx, rlID); err != nil && !errors.Is(err, configstore.ErrNotFound) {
					return touched, err
				}
				changed = true
			}
		}
		if changed {
			touched = append(touched, vkID)
		}
	}
	return touched, nil
}

func rateLimitMatchesSpec(rl *tables.TableRateLimit, spec map[string]any) bool {
	want := rateLimitFromSpec(spec, nil)
	if want.RequestMaxLimit == nil && want.TokenMaxLimit == nil {
		return false
	}
	eqInt := func(a, b *int64) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
	eqStr := func(a, b *string) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
	// Only compare fields the profile set; unset ones were never written by it.
	if want.RequestMaxLimit != nil && !eqInt(rl.RequestMaxLimit, want.RequestMaxLimit) {
		return false
	}
	if want.RequestResetDuration != nil && !eqStr(rl.RequestResetDuration, want.RequestResetDuration) {
		return false
	}
	if want.TokenMaxLimit != nil && !eqInt(rl.TokenMaxLimit, want.TokenMaxLimit) {
		return false
	}
	if want.TokenResetDuration != nil && !eqStr(rl.TokenResetDuration, want.TokenResetDuration) {
		return false
	}
	return true
}

func providerConfigsFromSpec(items []map[string]any, vkID string) []tables.TableVirtualKeyProviderConfig {
	out := make([]tables.TableVirtualKeyProviderConfig, 0, len(items))
	for _, item := range items {
		provider := firstString(item, "provider", "provider_name")
		if provider == "" {
			continue
		}
		cfg := tables.TableVirtualKeyProviderConfig{
			VirtualKeyID: vkID,
			Provider:     provider,
			AllowAllKeys: true, // listed provider without explicit key pin → use all provider keys
		}
		if models := stringSliceFromAny(item["allowed_models"]); len(models) > 0 {
			cfg.AllowedModels = schemas.WhiteList(models)
		} else if all, ok := item["all_models_allowed"].(bool); ok && all {
			cfg.AllowedModels = schemas.WhiteList{"*"}
		}
		if keyIDs := stringSliceFromAny(item["key_ids"]); len(keyIDs) > 0 && !(len(keyIDs) == 1 && keyIDs[0] == "*") {
			cfg.AllowAllKeys = false
		}
		if weight, ok := item["weight"].(float64); ok {
			cfg.Weight = &weight
		}
		out = append(out, cfg)
	}
	return out
}

func rateLimitFromSpec(spec map[string]any, existingID *string) *tables.TableRateLimit {
	id := uuid.NewString()
	if existingID != nil && *existingID != "" {
		id = *existingID
	}
	rl := &tables.TableRateLimit{ID: id}
	if v, ok := spec["request_max_limit"].(float64); ok {
		n := int64(v)
		rl.RequestMaxLimit = &n
	}
	if v, ok := spec["request_reset_duration"].(string); ok {
		rl.RequestResetDuration = &v
	}
	if v, ok := spec["token_max_limit"].(float64); ok {
		n := int64(v)
		rl.TokenMaxLimit = &n
	}
	if v, ok := spec["token_reset_duration"].(string); ok {
		rl.TokenResetDuration = &v
	}
	return rl
}

func budgetsFromSpec(items []map[string]any, vkID string) []tables.TableBudget {
	out := make([]tables.TableBudget, 0, len(items))
	for _, item := range items {
		b := tables.TableBudget{
			ID:           uuid.NewString(),
			VirtualKeyID: &vkID,
			LastReset:    time.Now().UTC(),
		}
		if v, ok := item["max_limit"].(float64); ok {
			b.MaxLimit = v
		}
		if v, ok := item["reset_duration"].(string); ok {
			b.ResetDuration = strings.TrimSpace(v)
		}
		// A budget without a limit or period cannot be enforced or reset.
		if b.MaxLimit <= 0 || b.ResetDuration == "" {
			continue
		}
		out = append(out, b)
	}
	return out
}

func stringSliceFromAny(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func uintFromAny(raw any) uint {
	switch typed := raw.(type) {
	case float64:
		if typed > 0 {
			return uint(typed)
		}
	case int:
		if typed > 0 {
			return uint(typed)
		}
	case int64:
		if typed > 0 {
			return uint(typed)
		}
	case uint:
		return typed
	case string:
		if n, err := strconv.ParseUint(typed, 10, 64); err == nil {
			return uint(n)
		}
	}
	return 0
}

func firstString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := item[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
