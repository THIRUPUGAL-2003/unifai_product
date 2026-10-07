package prompts

import (
	"context"
	"testing"
	"time"

	"github.com/gateway/gateway/core/schemas"
	configstoreTables "github.com/gateway/gateway/framework/configstore/tables"
)

type fakeDeployments struct {
	rows []configstoreTables.TablePromptDeployment
}

func (f *fakeDeployments) ListPromptDeployments(_ context.Context, promptID string) ([]configstoreTables.TablePromptDeployment, error) {
	out := make([]configstoreTables.TablePromptDeployment, 0, len(f.rows))
	for _, r := range f.rows {
		if promptID == "" || r.PromptID == promptID {
			out = append(out, r)
		}
	}
	return out, nil
}

func newResolverCtx(values map[schemas.GatewayContextKey]any) *schemas.GatewayContext {
	ctx := schemas.NewGatewayContext(context.Background(), time.Time{})
	for k, v := range values {
		ctx.SetValue(k, v)
	}
	return ctx
}

func TestDeploymentAwareResolver(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	now := time.Now()
	store := &fakeDeployments{rows: []configstoreTables.TablePromptDeployment{
		{ID: 1, PromptID: "p1", Environment: "production", VersionNumber: 1, Enabled: true, UpdatedAt: old},
		{ID: 2, PromptID: "p1", Environment: "Production", VersionNumber: 3, Enabled: true, UpdatedAt: now},
		{ID: 3, PromptID: "p2", Environment: "production", VersionNumber: 7, Enabled: true, UpdatedAt: old},
		{ID: 4, PromptID: "p1", Environment: "staging", VersionNumber: 2, Enabled: false, UpdatedAt: now},
	}}
	r := &deploymentAwareResolver{headers: &headerResolver{}, deployments: store}

	cases := []struct {
		name        string
		values      map[schemas.GatewayContextKey]any
		wantPrompt  string
		wantVersion int
	}{
		{"no headers resolves nothing", nil, "", 0},
		{"explicit version wins", map[schemas.GatewayContextKey]any{PromptIDKey: "p1", PromptVersionKey: "1", PromptEnvironmentKey: "production"}, "p1", 1},
		{"prompt plus env uses newest deployment of that prompt", map[schemas.GatewayContextKey]any{PromptIDKey: "p2", PromptEnvironmentKey: "production"}, "p2", 7},
		{"env only uses newest enabled deployment", map[schemas.GatewayContextKey]any{PromptEnvironmentKey: "production"}, "p1", 3},
		{"disabled deployment ignored", map[schemas.GatewayContextKey]any{PromptIDKey: "p1", PromptEnvironmentKey: "staging"}, "p1", 0},
		{"dimension env alone never injects", map[schemas.GatewayContextKey]any{schemas.GatewayContextKeyDimensions: map[string]string{"environment": "production"}}, "", 0},
		{"dimension env selects version for named prompt", map[schemas.GatewayContextKey]any{PromptIDKey: "p1", schemas.GatewayContextKeyDimensions: map[string]string{"environment": "production"}}, "p1", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotPrompt, gotVersion, err := r.Resolve(newResolverCtx(tc.values), nil)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if gotPrompt != tc.wantPrompt || gotVersion != tc.wantVersion {
				t.Fatalf("got (%q, %d), want (%q, %d)", gotPrompt, gotVersion, tc.wantPrompt, tc.wantVersion)
			}
		})
	}
}

func TestBuildMergedParamsMapDropsPlaygroundKeys(t *testing.T) {
	merged, err := buildMergedParamsMap(configstoreTables.ModelParams{
		"temperature": 0.2,
		"api_key_id":  "sk-uf-abc",
		"skill_id":    "skill-1",
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := merged["api_key_id"]; ok {
		t.Fatal("api_key_id must not reach the provider")
	}
	if _, ok := merged["skill_id"]; ok {
		t.Fatal("skill_id must not reach the provider")
	}
	if merged["temperature"] != 0.2 {
		t.Fatalf("temperature lost: %v", merged["temperature"])
	}
}
