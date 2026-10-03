package connectors

import "testing"

func TestBigqueryRowSanitizesColumnsAndNestedValues(t *testing.T) {
	row, err := bigqueryRow(map[string]any{
		"gen_ai.provider.name": "openai",
		"http.status_code":     200,
		"1st":                  true,
		"dimensions":           map[string]string{"team": "a"},
	})
	if err != nil {
		t.Fatalf("bigqueryRow: %v", err)
	}
	if row["gen_ai_provider_name"] != "openai" || row["http_status_code"] != 200 || row["_1st"] != true {
		t.Fatalf("columns not sanitized: %#v", row)
	}
	if row["dimensions"] != `{"team":"a"}` {
		t.Fatalf("nested value must be a JSON string, got %#v", row["dimensions"])
	}
}
