package handlers

import (
	"slices"
	"testing"

	"github.com/gateway/gateway/framework/configstore/tables"
)

func TestFolderCycleBreakPoints(t *testing.T) {
	p := func(s string) *string { return &s }
	folders := []tables.TableFolder{
		{ID: "root"},
		{ID: "child", ParentID: p("root")},
		{ID: "self", ParentID: p("self")},
		{ID: "a", ParentID: p("b")},
		{ID: "b", ParentID: p("a")},
		{ID: "hangs-off-loop", ParentID: p("a")},
	}
	got := folderCycleBreakPoints(folders)
	slices.Sort(got)
	if len(got) != 2 || got[1] != "self" || (got[0] != "a" && got[0] != "b") {
		t.Fatalf("expected one break in the a/b loop and one for the self-parent, got %v", got)
	}
	if got := folderCycleBreakPoints(folders[:2]); len(got) != 0 {
		t.Fatalf("a healthy tree has no break points, got %v", got)
	}
}
