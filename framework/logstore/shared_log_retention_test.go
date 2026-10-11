package logstore

import "testing"

func TestUnifySharedLogAutoDelete(t *testing.T) {
	ctrl := BrowserControlSettings{
		PromptLogAutoDelete: true,
		PromptLogRetention:  "30d",
		SearchLogAutoDelete: false,
		SearchLogRetention:  "7d",
	}
	if !unifySharedLogAutoDelete(&ctrl) {
		t.Fatal("expected the two settings to be merged")
	}
	if !ctrl.PromptLogAutoDelete || !ctrl.SearchLogAutoDelete {
		t.Fatal("both logs should use the same on switch")
	}
	if ctrl.PromptLogRetention != "30d" || ctrl.SearchLogRetention != "30d" {
		t.Fatalf("retention = %s %s", ctrl.PromptLogRetention, ctrl.SearchLogRetention)
	}
	if unifySharedLogAutoDelete(&ctrl) {
		t.Fatal("a second pass should leave matching settings unchanged")
	}
}
