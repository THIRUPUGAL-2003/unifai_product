package prompts

import (
	"context"
	"strings"
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

type fakeSkillAndPromptStore struct {
	prompts  []configstoreTables.TablePrompt
	versions []configstoreTables.TablePromptVersion
	skills   map[string]*configstoreTables.TableSkill
}

func (f *fakeSkillAndPromptStore) GetPrompts(_ context.Context, _ *string) ([]configstoreTables.TablePrompt, error) {
	return f.prompts, nil
}

func (f *fakeSkillAndPromptStore) GetAllPromptVersions(_ context.Context) ([]configstoreTables.TablePromptVersion, error) {
	return f.versions, nil
}

func (f *fakeSkillAndPromptStore) GetSkillLean(_ context.Context, id string) (*configstoreTables.TableSkill, error) {
	return f.skills[id], nil
}

func (f *fakeSkillAndPromptStore) GetSkillByName(_ context.Context, name string) (*configstoreTables.TableSkill, error) {
	for _, s := range f.skills {
		if strings.EqualFold(s.Name, name) {
			return s, nil
		}
	}
	return f.skills[name], nil
}

func TestSkillInjection(t *testing.T) {
	skill1Body := "# Skill 1: Code Reviewer Guidelines"
	skill2Body := "# Skill 2: Python Expert Persona"

	store := &fakeSkillAndPromptStore{
		prompts: []configstoreTables.TablePrompt{
			{ID: "p1", Name: "Prompt One"},
			{ID: "p-allowed", Name: "Prompt With Allowed Skills"},
		},
		versions: []configstoreTables.TablePromptVersion{
			{
				PromptID:      "p1",
				VersionNumber: 1,
				ModelParams: configstoreTables.ModelParams{
					"skill_id": "skill-1",
				},
				Messages: []configstoreTables.TablePromptVersionMessage{
					{
						Message: []byte(`{"role":"system","content":"Template system prompt"}`),
					},
				},
			},
			{
				PromptID:      "p-allowed",
				VersionNumber: 1,
				ModelParams: configstoreTables.ModelParams{
					"prompt_allowed_skills": []any{"skill-1", "skill-2"},
				},
			},
		},
		skills: map[string]*configstoreTables.TableSkill{
			"skill-1": {ID: "skill-1", Name: "Code Reviewer", SkillMDBody: skill1Body},
			"skill-2": {ID: "skill-2", Name: "Python Expert", SkillMDBody: skill2Body},
		},
	}

	p, err := Init(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	userMsg := "Help me write code"
	mkChatReq := func() *schemas.GatewayRequest {
		return &schemas.GatewayRequest{
			ChatRequest: &schemas.GatewayChatRequest{
				Input: []schemas.ChatMessage{
					{
						Role:    schemas.ChatMessageRoleUser,
						Content: &schemas.ChatMessageContent{ContentStr: &userMsg},
					},
				},
			},
		}
	}

	// Case 1: Standalone execution with explicit x-uf-skill-id header (no prompt id)
	t.Run("explicit_skill_header_alone", func(t *testing.T) {
		req := mkChatReq()
		ctx := newResolverCtx(map[schemas.GatewayContextKey]any{
			SkillIDKey: "skill-2",
		})
		resReq, _, err := p.PreLLMHook(ctx, req)
		if err != nil {
			t.Fatalf("PreLLMHook: %v", err)
		}
		if len(resReq.ChatRequest.Input) != 2 {
			t.Fatalf("expected 2 messages (skill system + user), got %d", len(resReq.ChatRequest.Input))
		}
		if resReq.ChatRequest.Input[0].Role != schemas.ChatMessageRoleSystem ||
			*resReq.ChatRequest.Input[0].Content.ContentStr != skill2Body {
			t.Fatalf("expected skill-2 body at index 0, got %v", resReq.ChatRequest.Input[0])
		}
	})

	// Case 2: Prompt execution without skill header - should fallback to version's configured skill_id!
	t.Run("prompt_fallback_to_version_skill_id", func(t *testing.T) {
		req := mkChatReq()
		ctx := newResolverCtx(map[schemas.GatewayContextKey]any{
			PromptIDKey:      "p1",
			PromptVersionKey: "1",
		})
		resReq, _, err := p.PreLLMHook(ctx, req)
		if err != nil {
			t.Fatalf("PreLLMHook: %v", err)
		}
		// Expect 3 messages: [Skill System, Template System, User Message]
		if len(resReq.ChatRequest.Input) != 3 {
			t.Fatalf("expected 3 messages, got %d: %+v", len(resReq.ChatRequest.Input), resReq.ChatRequest.Input)
		}
		if *resReq.ChatRequest.Input[0].Content.ContentStr != skill1Body {
			t.Errorf("expected skill-1 body at top, got %q", *resReq.ChatRequest.Input[0].Content.ContentStr)
		}
		if *resReq.ChatRequest.Input[1].Content.ContentStr != "Template system prompt" {
			t.Errorf("expected template system prompt at index 1, got %q", *resReq.ChatRequest.Input[1].Content.ContentStr)
		}
		if *resReq.ChatRequest.Input[2].Content.ContentStr != userMsg {
			t.Errorf("expected user message at index 2, got %q", *resReq.ChatRequest.Input[2].Content.ContentStr)
		}
	})

	// Case 3: Explicit header overrides prompt version's configured skill
	t.Run("explicit_header_overrides_version_skill", func(t *testing.T) {
		req := mkChatReq()
		ctx := newResolverCtx(map[schemas.GatewayContextKey]any{
			PromptIDKey:      "p1",
			PromptVersionKey: "1",
			SkillIDKey:       "skill-2",
		})
		resReq, _, err := p.PreLLMHook(ctx, req)
		if err != nil {
			t.Fatalf("PreLLMHook: %v", err)
		}
		if len(resReq.ChatRequest.Input) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(resReq.ChatRequest.Input))
		}
		if *resReq.ChatRequest.Input[0].Content.ContentStr != skill2Body {
			t.Errorf("expected skill-2 override at top, got %q", *resReq.ChatRequest.Input[0].Content.ContentStr)
		}
	})

	// Case 4: Prompt with prompt_allowed_skills list
	t.Run("prompt_with_prompt_allowed_skills_list", func(t *testing.T) {
		req := mkChatReq()
		ctx := newResolverCtx(map[schemas.GatewayContextKey]any{
			PromptIDKey:      "p-allowed",
			PromptVersionKey: "1",
		})
		resReq, _, err := p.PreLLMHook(ctx, req)
		if err != nil {
			t.Fatalf("PreLLMHook: %v", err)
		}
		// Expect 3 messages: [Skill 1 System, Skill 2 System, User Message]
		if len(resReq.ChatRequest.Input) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(resReq.ChatRequest.Input))
		}
		if *resReq.ChatRequest.Input[0].Content.ContentStr != skill1Body {
			t.Errorf("expected skill 1 at index 0, got %q", *resReq.ChatRequest.Input[0].Content.ContentStr)
		}
		if *resReq.ChatRequest.Input[1].Content.ContentStr != skill2Body {
			t.Errorf("expected skill 2 at index 1, got %q", *resReq.ChatRequest.Input[1].Content.ContentStr)
		}
	})

	// Case 5: External client (Cursor, Codex, or OpenAI SDK) passes skill_id in extra_params
	t.Run("external_client_extra_params", func(t *testing.T) {
		req := mkChatReq()
		req.ChatRequest.Params = &schemas.ChatParameters{
			ExtraParams: map[string]interface{}{
				"skill_id": "skill-2",
			},
		}
		ctx := newResolverCtx(nil)
		resReq, _, err := p.PreLLMHook(ctx, req)
		if err != nil {
			t.Fatalf("PreLLMHook: %v", err)
		}
		if len(resReq.ChatRequest.Input) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(resReq.ChatRequest.Input))
		}
		if *resReq.ChatRequest.Input[0].Content.ContentStr != skill2Body {
			t.Errorf("expected skill-2 at index 0, got %q", *resReq.ChatRequest.Input[0].Content.ContentStr)
		}
		// Ensure skill_id was removed from ExtraParams so provider does not reject it
		if _, exists := req.ChatRequest.Params.ExtraParams["skill_id"]; exists {
			t.Errorf("skill_id must be stripped from extra_params before reaching model provider")
		}
	})

	// Case 6: Skill lookup by kebab-case name
	t.Run("skill_lookup_by_name", func(t *testing.T) {
		req := mkChatReq()
		ctx := newResolverCtx(map[schemas.GatewayContextKey]any{
			SkillIDKey: "Python Expert",
		})
		resReq, _, err := p.PreLLMHook(ctx, req)
		if err != nil {
			t.Fatalf("PreLLMHook: %v", err)
		}
		if len(resReq.ChatRequest.Input) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(resReq.ChatRequest.Input))
		}
		if *resReq.ChatRequest.Input[0].Content.ContentStr != skill2Body {
			t.Errorf("expected skill-2 by name, got %q", *resReq.ChatRequest.Input[0].Content.ContentStr)
		}
	})
}
