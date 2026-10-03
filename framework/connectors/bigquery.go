package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/raksha/raksha/core/schemas"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

func bigqueryService(ctx context.Context, cfg Settings) (*bigquery.Service, string, string, string, error) {
	projectID := configValue(cfg.Config, "project_id")
	dataset := configValue(cfg.Config, "dataset")
	table := configValue(cfg.Config, "table")
	if projectID == "" {
		return nil, "", "", "", fmt.Errorf("project_id is required")
	}
	if dataset == "" {
		return nil, "", "", "", fmt.Errorf("dataset is required")
	}
	if table == "" {
		table = "llm_logs"
	}
	credsJSON := configValue(cfg.Config, "credentials_json")
	if credsJSON == "" {
		return nil, "", "", "", fmt.Errorf("credentials_json is required")
	}
	creds, err := google.CredentialsFromJSON(ctx, []byte(credsJSON), bigquery.BigqueryScope)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("invalid service account json: %w", err)
	}
	svc, err := bigquery.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, "", "", "", err
	}
	return svc, projectID, dataset, table, nil
}

func exportBigQuery(ctx context.Context, cfg Settings, trace *schemas.Trace) error {
	svc, projectID, dataset, table, err := bigqueryService(ctx, cfg)
	if err != nil {
		return err
	}
	row, err := bigqueryRow(traceEvent(trace))
	if err != nil {
		return err
	}
	req := &bigquery.TableDataInsertAllRequest{
		// Columns the admin's table does not define are dropped instead of failing the row.
		IgnoreUnknownValues: true,
		Rows: []*bigquery.TableDataInsertAllRequestRows{
			{
				InsertId: trace.TraceID,
				Json:     row,
			},
		},
	}
	resp, err := svc.Tabledata.InsertAll(projectID, dataset, table, req).Context(ctx).Do()
	if err != nil {
		return err
	}
	if resp != nil && len(resp.InsertErrors) > 0 {
		for _, ie := range resp.InsertErrors {
			for _, e := range ie.Errors {
				if e != nil {
					return fmt.Errorf("bigquery rejected row: %s (%s)", e.Message, e.Location)
				}
			}
		}
		return fmt.Errorf("bigquery rejected row")
	}
	return nil
}

// bigqueryRow maps a trace event to BigQuery column names: letters, digits, and
// underscores only ("gen_ai.provider.name" → "gen_ai_provider_name"). Nested values
// (dimensions, attributes) are stored as JSON strings so a STRING column accepts them.
func bigqueryRow(event map[string]any) (map[string]bigquery.JsonValue, error) {
	row := make(map[string]bigquery.JsonValue, len(event))
	for k, v := range event {
		col := bigqueryColumnName(k)
		if col == "" {
			continue
		}
		switch v.(type) {
		case nil, string, bool, int, int32, int64, float32, float64:
			row[col] = v
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			row[col] = string(b)
		}
	}
	return row, nil
}

func bigqueryColumnName(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	col := b.String()
	if col != "" && col[0] >= '0' && col[0] <= '9' {
		col = "_" + col
	}
	return col
}

func testBigQuery(ctx context.Context, cfg Settings) error {
	svc, projectID, dataset, table, err := bigqueryService(ctx, cfg)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, err = svc.Tables.Get(projectID, dataset, table).Context(testCtx).Do()
	if err != nil {
		return fmt.Errorf("bigquery table %s.%s.%s not reachable: %w", projectID, dataset, table, err)
	}
	return nil
}
