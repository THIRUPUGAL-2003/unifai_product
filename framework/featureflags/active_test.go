package featureflags

import "testing"

func TestActiveEnabledUsesDefaultBeforeBoot(t *testing.T) {
	SetActive(nil)
	t.Cleanup(func() { SetActive(nil) })

	if !ActiveEnabled(FlagBrowserPromptGuard) {
		t.Fatal("prompt guard should default on before the store exists")
	}
	if ActiveEnabled("not.a.real.flag") {
		t.Fatal("unknown flags should be off")
	}
}

func TestActiveEnabledFollowsToggle(t *testing.T) {
	store, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	SetActive(store)
	t.Cleanup(func() { SetActive(nil) })

	if !ActiveEnabled(FlagCacheResponses) {
		t.Fatal("cache flag should start at its default")
	}
	if _, err := store.Set(t.Context(), FlagCacheResponses, false); err != nil {
		t.Fatal(err)
	}
	if ActiveEnabled(FlagCacheResponses) {
		t.Fatal("toggle should apply immediately")
	}
}
