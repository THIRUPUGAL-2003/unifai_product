package connectors

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/unifai/unifai/core/schemas"
)

func kafkaWriter(cfg Settings) (*kafka.Writer, error) {
	brokers := strings.Split(configValue(cfg.Config, "brokers"), ",")
	clean := make([]string, 0, len(brokers))
	for _, b := range brokers {
		b = strings.TrimSpace(b)
		if b != "" {
			clean = append(clean, b)
		}
	}
	if len(clean) == 0 {
		return nil, fmt.Errorf("kafka brokers is required")
	}
	topic := configValue(cfg.Config, "topic")
	if topic == "" {
		return nil, fmt.Errorf("kafka topic is required")
	}
	transport := &kafka.Transport{}
	user := configValue(cfg.Config, "username")
	pass := configValue(cfg.Config, "password")
	if user != "" || pass != "" {
		transport.SASL = plain.Mechanism{Username: user, Password: pass}
	}
	return &kafka.Writer{
		Addr:         kafka.TCP(clean...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		Transport:    transport,
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}, nil
}

func exportKafka(ctx context.Context, cfg Settings, trace *schemas.Trace) error {
	writer, err := kafkaWriter(cfg)
	if err != nil {
		return err
	}
	defer writer.Close()
	body, err := encodeEvent(traceEvent(trace))
	if err != nil {
		return err
	}
	return writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(trace.RequestID),
		Value: body,
		Time:  time.Now().UTC(),
	})
}

func testKafka(ctx context.Context, cfg Settings) error {
	writer, err := kafkaWriter(cfg)
	if err != nil {
		return err
	}
	defer writer.Close()
	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return writer.WriteMessages(testCtx, kafka.Message{
		Key:   []byte("unifai-connector-test"),
		Value: []byte(`{"message":"unifai connector test"}`),
		Time:  time.Now().UTC(),
	})
}
