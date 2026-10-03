package rbac

import "testing"

func TestPlaygroundSessionWritesNeedOnlyView(t *testing.T) {
	cases := []struct {
		method, path string
		wantOp       string
	}{
		{"POST", "/api/prompt-repo/prompts/p1/sessions", "View"},
		{"PUT", "/api/prompt-repo/sessions/12", "View"},
		{"DELETE", "/api/prompt-repo/sessions/12", "View"},
		{"PUT", "/api/prompt-repo/sessions/12/rename", "View"},
		{"POST", "/api/prompt-repo/sessions/12/commit", "Create"},
		{"POST", "/api/prompt-repo/prompts", "Create"},
		{"PUT", "/api/prompt-repo/prompts/p1", "Update"},
		{"POST", "/api/prompt-repo/prompts//sessions", "Create"},
	}
	for _, tc := range cases {
		req := PathRequirementFor(tc.method, tc.path)
		if req == nil {
			t.Fatalf("%s %s: expected a requirement", tc.method, tc.path)
		}
		if req.Resource != "PromptRepository" || req.Operation != tc.wantOp {
			t.Errorf("%s %s: got %s/%s, want PromptRepository/%s", tc.method, tc.path, req.Resource, req.Operation, tc.wantOp)
		}
	}
}
