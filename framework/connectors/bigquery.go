package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/unifai/unifai/core/schemas"
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
	rowJSON, err := encodeEvent(traceEvent(trace))
	if err != nil {
		return err
	}
	var row map[string]bigquery.JsonValue
	if err := json.Unmarshal(rowJSON, &row); err != nil {
		return err
	}
	req := &bigquery.TableDataInsertAllRequest{
		Rows: []*bigquery.TableDataInsertAllRequestRows{
			{
				InsertId: trace.TraceID,
				Json:     row,
			},
		},
	}
	_, err = svc.Tabledata.InsertAll(projectID, dataset, table, req).Context(ctx).Do()
	return err
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
