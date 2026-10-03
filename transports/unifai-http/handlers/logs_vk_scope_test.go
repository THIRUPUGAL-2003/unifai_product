package handlers

import (
	"slices"
	"strings"
	"testing"
)

func TestScopedVKFilter(t *testing.T) {
	allowed := map[string]bool{"vk-a": true, "vk-b": true}

	got := strings.Split(scopedVKFilter("", allowed), ",")
	slices.Sort(got)
	if !slices.Equal(got, []string{"vk-a", "vk-b"}) {
		t.Fatalf("no filter should default to the caller's keys, got %v", got)
	}
	if got := scopedVKFilter("vk-a,vk-other", allowed); got != "vk-a" {
		t.Fatalf("requested keys outside the caller's set must be dropped, got %q", got)
	}
	if got := scopedVKFilter("vk-other", allowed); got != noVirtualKeyAccess {
		t.Fatalf("only foreign keys requested must match nothing, got %q", got)
	}
	if got := scopedVKFilter("", map[string]bool{}); got != noVirtualKeyAccess {
		t.Fatalf("a caller with no keys must match nothing, got %q", got)
	}
}
