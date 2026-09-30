package mcptoolgroups

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
)

// Group is the runtime view of an MCP tool group.
type Group struct {
	ID            uint
	Name          string
	Enabled       bool
	Tools         []string
	VirtualKeyIDs []string
	TeamIDs       []string
	CustomerIDs   []string
	UserIDs       []string
	ProviderNames []string
}

// Runtime holds enabled MCP tool groups.
type Runtime struct {
	mu     sync.RWMutex
	groups []Group
}

// Default is the process-wide MCP tool group runtime.
var Default = &Runtime{}

// ReloadFromStore refreshes tool groups from the workspace DB.
func ReloadFromStore(ctx context.Context, store configstore.WorkspaceStore) error {
	if store == nil {
		Default.mu.Lock()
		Default.groups = nil
		Default.mu.Unlock()
		return nil
	}
	rows, err := store.ListMCPToolGroups(ctx)
	if err != nil {
		return err
	}
	groups := make([]Group, 0, len(rows))
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		groups = append(groups, groupFromRow(row))
	}
	Default.mu.Lock()
	Default.groups = groups
	Default.mu.Unlock()
	return nil
}

func groupFromRow(row tables.TableMCPToolGroup) Group {
	spec := row.ParsedSpec
	if spec == nil {
		spec = map[string]any{}
	}
	return Group{
		ID: row.ID, Name: row.Name, Enabled: row.Enabled,
		Tools:         toolNames(spec),
		VirtualKeyIDs: stringSlice(spec, "virtual_key_ids"),
		TeamIDs:       stringSlice(spec, "team_ids"),
		CustomerIDs:   stringSlice(spec, "customer_ids"),
		UserIDs:       stringSlice(spec, "user_ids"),
		ProviderNames: stringSlice(spec, "provider_names"),
	}
}

func toolNames(spec map[string]any) []string {
	raw, ok := spec["tools"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		// Some stores round-trip as []map[string]any via JSON.
		if typed, ok := raw.([]map[string]any); ok {
			items = make([]any, len(typed))
			for i, m := range typed {
				items[i] = m
			}
		} else {
			return nil
		}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
			continue
		}
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// Flat runtime form: { "name": "clientName-toolName" } (also tool_name / id).
		foundFlat := false
		for _, key := range []string{"name", "tool_name", "id"} {
			if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
				// Skip bare tool_name when this is the UI bundle shape (has mcp_client_*).
				if key == "tool_name" && (m["mcp_client_id"] != nil || m["mcp_client_name"] != nil || m["tool_names"] != nil) {
					continue
				}
				out = append(out, strings.TrimSpace(v))
				foundFlat = true
				break
			}
		}
		if foundFlat {
			continue
		}
		// UI / Catalog form: { mcp_client_name|mcp_client_id, tool_names: [] }.
		// Runtime tool ids are "clientName-toolName" (see governance MCP checks).
		prefix := strings.TrimSpace(anyString(m["mcp_client_name"]))
		if prefix == "" {
			prefix = strings.TrimSpace(anyString(m["mcp_client_id"]))
		}
		if prefix == "" {
			continue
		}
		names := stringSliceFromValue(m["tool_names"])
		if len(names) == 0 {
			if single := strings.TrimSpace(anyString(m["tool_name"])); single != "" {
				names = []string{single}
			}
		}
		if len(names) == 0 {
			out = append(out, prefix+"-*")
			continue
		}
		for _, n := range names {
			n = strings.TrimSpace(n)
			if n == "" || n == "*" {
				out = append(out, prefix+"-*")
				continue
			}
			out = append(out, prefix+"-"+n)
		}
	}
	return out
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func stringSliceFromValue(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func stringSlice(spec map[string]any, key string) []string {
	raw, ok := spec[key]
	if !ok {
		return nil
	}
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
		if b, err := json.Marshal(raw); err == nil {
			var out []string
			_ = json.Unmarshal(b, &out)
			return out
		}
	}
	return nil
}

// RequestContext carries identifiers for tool group matching.
type RequestContext struct {
	VirtualKeyID string
	UserID       string
	TeamID       string
	CustomerID   string
	ProviderName string
}

// IsToolAllowed returns whether toolName is permitted by any matching enabled group.
// When no groups are configured, all tools are allowed.
func (r *Runtime) IsToolAllowed(toolName string, req RequestContext) bool {
	r.mu.RLock()
	groups := append([]Group(nil), r.groups...)
	r.mu.RUnlock()
	if len(groups) == 0 {
		return true
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return true
	}
	matched := false
	for _, group := range groups {
		if !group.matches(req) {
			continue
		}
		matched = true
		if len(group.Tools) == 0 {
			return true
		}
		for _, allowed := range group.Tools {
			if toolMatches(allowed, toolName) {
				return true
			}
		}
	}
	if !matched {
		return true
	}
	return false
}

// FilterTools narrows an include-tools stamp using matching tool groups so
// chat injection matches PreMCP execute enforcement. When no matching group
// applies (or a matching group has an empty tool list = allow-all), tools
// are returned unchanged.
func (r *Runtime) FilterTools(tools []string, req RequestContext) []string {
	if len(tools) == 0 {
		return tools
	}
	r.mu.RLock()
	groups := append([]Group(nil), r.groups...)
	r.mu.RUnlock()
	if len(groups) == 0 {
		return tools
	}

	matched := make([]Group, 0)
	allowsAll := false
	for _, group := range groups {
		if !group.matches(req) {
			continue
		}
		matched = append(matched, group)
		if len(group.Tools) == 0 {
			allowsAll = true
		}
	}
	if len(matched) == 0 || allowsAll {
		return tools
	}

	out := make([]string, 0, len(tools))
	seen := make(map[string]struct{}, len(tools))
	add := func(tool string) {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			return
		}
		if _, ok := seen[tool]; ok {
			return
		}
		seen[tool] = struct{}{}
		out = append(out, tool)
	}

	for _, tool := range tools {
		clientPrefix, isWildcard := strings.CutSuffix(tool, "-*")
		if isWildcard {
			for _, group := range matched {
				for _, pattern := range group.Tools {
					if pattern == clientPrefix+"-*" || strings.HasPrefix(pattern, clientPrefix+"-") {
						add(pattern)
					}
				}
			}
			continue
		}
		if r.IsToolAllowed(tool, req) {
			add(tool)
		}
	}
	return out
}

func (g Group) matches(req RequestContext) bool {
	if len(g.VirtualKeyIDs) > 0 && !contains(g.VirtualKeyIDs, req.VirtualKeyID) {
		return false
	}
	if len(g.UserIDs) > 0 && !contains(g.UserIDs, req.UserID) {
		return false
	}
	if len(g.TeamIDs) > 0 && !contains(g.TeamIDs, req.TeamID) {
		return false
	}
	if len(g.CustomerIDs) > 0 && !contains(g.CustomerIDs, req.CustomerID) {
		return false
	}
	if len(g.ProviderNames) > 0 && !contains(g.ProviderNames, req.ProviderName) {
		return false
	}
	return true
}

func contains(list []string, value string) bool {
	if value == "" {
		return false
	}
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func toolMatches(pattern, toolName string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "-*") {
		prefix := strings.TrimSuffix(pattern, "-*")
		return strings.HasPrefix(toolName, prefix+"-")
	}
	return pattern == toolName
}
