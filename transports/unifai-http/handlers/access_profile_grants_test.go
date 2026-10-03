package handlers

import (
	"context"
	"slices"
	"testing"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"gorm.io/gorm"
)

type grantsTestStore struct {
	configstore.ConfigStore
	configstore.WorkspaceStore
	settings map[string]string
	pcs      []tables.TableVirtualKeyProviderConfig
	nextID   uint
}

func newGrantsTestStore() *grantsTestStore {
	return &grantsTestStore{settings: map[string]string{}, nextID: 1}
}

func (s *grantsTestStore) GetWorkspaceSetting(_ context.Context, key string) (*tables.TableWorkspaceSetting, error) {
	data, ok := s.settings[key]
	if !ok {
		return nil, configstore.ErrNotFound
	}
	return &tables.TableWorkspaceSetting{Key: key, Data: data}, nil
}

func (s *grantsTestStore) UpsertWorkspaceSetting(_ context.Context, key, data string) error {
	s.settings[key] = data
	return nil
}

func (s *grantsTestStore) GetVirtualKeyProviderConfigs(_ context.Context, vkID string) ([]tables.TableVirtualKeyProviderConfig, error) {
	var out []tables.TableVirtualKeyProviderConfig
	for _, pc := range s.pcs {
		if pc.VirtualKeyID == vkID {
			out = append(out, pc)
		}
	}
	return out, nil
}

func (s *grantsTestStore) CreateVirtualKeyProviderConfig(_ context.Context, pc *tables.TableVirtualKeyProviderConfig, _ ...*gorm.DB) error {
	pc.ID = s.nextID
	s.nextID++
	s.pcs = append(s.pcs, *pc)
	return nil
}

func (s *grantsTestStore) UpdateVirtualKeyProviderConfig(_ context.Context, pc *tables.TableVirtualKeyProviderConfig, _ ...*gorm.DB) error {
	for i := range s.pcs {
		if s.pcs[i].ID == pc.ID {
			s.pcs[i] = *pc
		}
	}
	return nil
}

func (s *grantsTestStore) DeleteVirtualKeyProviderConfig(_ context.Context, id uint, _ ...*gorm.DB) error {
	s.pcs = slices.DeleteFunc(s.pcs, func(pc tables.TableVirtualKeyProviderConfig) bool { return pc.ID == id })
	return nil
}

func (s *grantsTestStore) provider(vkID, name string) *tables.TableVirtualKeyProviderConfig {
	for i := range s.pcs {
		if s.pcs[i].VirtualKeyID == vkID && s.pcs[i].Provider == name {
			return &s.pcs[i]
		}
	}
	return nil
}

func grantTestProfile(id uint, vkID string, providers ...map[string]any) tables.TableAccessProfile {
	items := make([]any, 0, len(providers))
	for _, p := range providers {
		items = append(items, p)
	}
	return tables.TableAccessProfile{ID: id, IsActive: true, ParsedSpec: map[string]any{
		"virtual_key_ids":  []any{vkID},
		"provider_configs": items,
	}}
}

func applyGrantTestProfile(t *testing.T, ctx context.Context, store *grantsTestStore, p tables.TableAccessProfile) {
	t.Helper()
	spec := p.Spec()
	items := specMapSlice(spec, "provider_configs")
	vkIDs := specStringSlice(spec, "virtual_key_ids")
	if err := snapshotAccessProfileGrants(ctx, store, p.ID, vkIDs, items, nil); err != nil {
		t.Fatal(err)
	}
	for _, vkID := range vkIDs {
		if err := applyAccessProfileProviders(ctx, store, vkID, items); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAccessProfileProviderRollbackRestoresPriorState(t *testing.T) {
	ctx := context.Background()
	store := newGrantsTestStore()
	_ = store.CreateVirtualKeyProviderConfig(ctx, &tables.TableVirtualKeyProviderConfig{
		VirtualKeyID: "vk1", Provider: "openai", AllowedModels: schemas.WhiteList{"gpt-4o-mini"},
	})

	profile := grantTestProfile(1, "vk1",
		map[string]any{"provider": "openai", "allowed_models": []any{"gpt-4o"}},
		map[string]any{"provider": "anthropic", "all_models_allowed": true},
	)
	applyGrantTestProfile(t, ctx, store, profile)
	if store.provider("vk1", "anthropic") == nil {
		t.Fatal("profile should have added anthropic")
	}

	// Drop only anthropic from the profile: anthropic goes, openai keeps the profile's models.
	updated := grantTestProfile(1, "vk1", map[string]any{"provider": "openai", "allowed_models": []any{"gpt-4o"}})
	if _, err := rollbackAccessProfileGrants(ctx, store, profile, &updated); err != nil {
		t.Fatal(err)
	}
	if store.provider("vk1", "anthropic") != nil {
		t.Fatal("anthropic added by the profile must be removed when the profile drops it")
	}
	if got := store.provider("vk1", "openai").AllowedModels; !slices.Equal(got, schemas.WhiteList{"gpt-4o"}) {
		t.Fatalf("openai still granted by the profile, got %v", got)
	}

	// Delete the profile: openai goes back to what the VK had before.
	if _, err := rollbackAccessProfileGrants(ctx, store, updated, nil); err != nil {
		t.Fatal(err)
	}
	if got := store.provider("vk1", "openai"); got == nil || !slices.Equal(got.AllowedModels, schemas.WhiteList{"gpt-4o-mini"}) {
		t.Fatalf("openai must be restored to its pre-profile models, got %+v", got)
	}
}

func TestAccessProfileProviderRollbackHandsOffBetweenProfiles(t *testing.T) {
	ctx := context.Background()
	store := newGrantsTestStore()
	p1 := grantTestProfile(1, "vk1", map[string]any{"provider": "anthropic", "all_models_allowed": true})
	p2 := grantTestProfile(2, "vk1", map[string]any{"provider": "anthropic", "all_models_allowed": true})
	applyGrantTestProfile(t, ctx, store, p1)
	applyGrantTestProfile(t, ctx, store, p2)

	if _, err := rollbackAccessProfileGrants(ctx, store, p1, nil); err != nil {
		t.Fatal(err)
	}
	if store.provider("vk1", "anthropic") == nil {
		t.Fatal("anthropic is still granted by profile 2 and must stay")
	}
	if _, err := rollbackAccessProfileGrants(ctx, store, p2, nil); err != nil {
		t.Fatal(err)
	}
	if store.provider("vk1", "anthropic") != nil {
		t.Fatal("after both profiles are gone the VK must lose anthropic (it never had it)")
	}
}

func TestAccessProfileProviderRollbackKeepsManualEdits(t *testing.T) {
	ctx := context.Background()
	store := newGrantsTestStore()
	_ = store.CreateVirtualKeyProviderConfig(ctx, &tables.TableVirtualKeyProviderConfig{
		VirtualKeyID: "vk1", Provider: "openai", AllowedModels: schemas.WhiteList{"gpt-4o-mini"},
	})
	p := grantTestProfile(1, "vk1", map[string]any{"provider": "openai", "allowed_models": []any{"gpt-4o"}})
	applyGrantTestProfile(t, ctx, store, p)
	store.provider("vk1", "openai").AllowedModels = schemas.WhiteList{"o3"}

	if _, err := rollbackAccessProfileGrants(ctx, store, p, nil); err != nil {
		t.Fatal(err)
	}
	if got := store.provider("vk1", "openai").AllowedModels; !slices.Equal(got, schemas.WhiteList{"o3"}) {
		t.Fatalf("admin edit made after the profile must be kept, got %v", got)
	}
	if profileHasGrantSnapshots(ctx, store, 1) {
		t.Fatal("snapshots must be cleared once the profile is rolled back")
	}
}
