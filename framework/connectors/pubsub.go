package connectors

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/unifai/unifai/core/schemas"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	pubsubapi "google.golang.org/api/pubsub/v1"
)

func pubsubService(ctx context.Context, cfg Settings) (*pubsubapi.Service, string, string, error) {
	projectID := configValue(cfg.Config, "project_id")
	topic := configValue(cfg.Config, "topic")
	if projectID == "" {
		return nil, "", "", fmt.Errorf("project_id is required")
	}
	if topic == "" {
		return nil, "", "", fmt.Errorf("topic is required")
	}
	credsJSON := configValue(cfg.Config, "credentials_json")
	if credsJSON == "" {
		return nil, "", "", fmt.Errorf("credentials_json is required")
	}
	creds, err := google.CredentialsFromJSON(ctx, []byte(credsJSON), pubsubapi.PubsubScope)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid service account json: %w", err)
	}
	svc, err := pubsubapi.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, "", "", err
	}
	return svc, projectID, topic, nil
}

func exportPubSub(ctx context.Context, cfg Settings, trace *schemas.Trace) error {
	svc, projectID, topic, err := pubsubService(ctx, cfg)
	if err != nil {
		return err
	}
	body, err := encodeEvent(traceEvent(trace))
	if err != nil {
		return err
	}
	topicName := fmt.Sprintf("projects/%s/topics/%s", projectID, topic)
	_, err = svc.Projects.Topics.Publish(topicName, &pubsubapi.PublishRequest{
		Messages: []*pubsubapi.PubsubMessage{{
			Data: base64.StdEncoding.EncodeToString(body),
		}},
	}).Context(ctx).Do()
	return err
}

func testPubSub(ctx context.Context, cfg Settings) error {
	svc, projectID, topic, err := pubsubService(ctx, cfg)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	topicName := fmt.Sprintf("projects/%s/topics/%s", projectID, topic)
	_, err = svc.Projects.Topics.Get(topicName).Context(testCtx).Do()
	if err != nil {
		return fmt.Errorf("pubsub topic not reachable: %w", err)
	}
	payload, _ := sonic.Marshal(map[string]string{"message": "unifai connector test"})
	_, err = svc.Projects.Topics.Publish(topicName, &pubsubapi.PublishRequest{
		Messages: []*pubsubapi.PubsubMessage{{
			Data: base64.StdEncoding.EncodeToString(payload),
		}},
	}).Context(testCtx).Do()
	return err
}
