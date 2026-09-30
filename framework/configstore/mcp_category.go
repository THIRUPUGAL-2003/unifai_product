package configstore

import (
	"strings"
	"unicode"
)

// mcpCategoryAliases maps lowercase/normalized spellings onto a single display
// label so the library filter sidebar never lists AI/ai or Communication/
// Communications as separate facets.
var mcpCategoryAliases = map[string]string{
	"ai":               "AI",
	"crm":              "CRM",
	"communication":    "Communication",
	"communications":   "Communication",
	"design":           "Design",
	"ecommerce":        "E-commerce",
	"e-commerce":       "E-commerce",
	"e commerce":       "E-commerce",
	"developer tools":  "Developer Tools",
	"developer tool":   "Developer Tools",
	"developertools":   "Developer Tools",
	"finance":          "Finance",
	"general":          "General",
	"observability":    "Observability",
	"productivity":     "Productivity",
	"search":           "Search",
	"automation":       "Automation",
	"security":         "Security",
	"database":         "Database",
	"databases":        "Database",
	"devops":           "DevOps",
	"marketing":        "Marketing",
	"sales":            "Sales",
	"support":          "Support",
	"analytics":        "Analytics",
	"collaboration":    "Collaboration",
	"storage":          "Storage",
	"infrastructure":   "Infrastructure",
}

// CanonicalMCPCategory returns a stable display category for library rows and
// filter facets. Empty input stays empty.
func CanonicalMCPCategory(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	key := normalizeMCPCategoryKey(trimmed)
	if canon, ok := mcpCategoryAliases[key]; ok {
		return canon
	}
	return titleMCPCategory(trimmed)
}

// MCPCategoryFilterValues expands selected facet labels into every known raw
// spelling that should match them (plus the selection itself), so dirty DB
// rows still appear when the user checks the canonical checkbox.
func MCPCategoryFilterValues(selected []string) []string {
	if len(selected) == 0 {
		return nil
	}
	wanted := make(map[string]struct{}, len(selected))
	for _, s := range selected {
		canon := CanonicalMCPCategory(s)
		if canon == "" {
			continue
		}
		wanted[canon] = struct{}{}
	}
	if len(wanted) == 0 {
		return nil
	}

	out := make([]string, 0, len(wanted)*4)
	seen := make(map[string]struct{}, len(wanted)*4)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	for canon := range wanted {
		add(canon)
		add(strings.ToLower(canon))
	}
	for alias, canon := range mcpCategoryAliases {
		if _, ok := wanted[canon]; !ok {
			continue
		}
		add(alias)
		add(canon)
		// Preserve common Title-Case of multi-word aliases.
		add(titleMCPCategory(alias))
	}
	for _, s := range selected {
		add(s)
		add(strings.ToLower(strings.TrimSpace(s)))
	}
	return out
}

// DedupeCanonicalMCPCategories returns sorted-unique canonical category labels.
func DedupeCanonicalMCPCategories(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		c := CanonicalMCPCategory(r)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func normalizeMCPCategoryKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func titleMCPCategory(s string) string {
	fields := strings.Fields(strings.TrimSpace(s))
	for i, f := range fields {
		runes := []rune(strings.ToLower(f))
		if len(runes) == 0 {
			continue
		}
		runes[0] = unicode.ToUpper(runes[0])
		fields[i] = string(runes)
	}
	return strings.Join(fields, " ")
}
