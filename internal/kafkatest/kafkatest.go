// Package kafkatest provides helpers for Kafka-related tests: a real test broker and
// test TLS certificates.
package kafkatest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"time"

	"github.com/segmentio/kafka-go"
)

// BrokerEnv is the environment variable holding the address of the test broker.
const BrokerEnv = "KAFKA_TEST_BROKER"

const pollInterval = 100 * time.Millisecond

// Broker returns the test broker address from BrokerEnv after checking that it accepts
// TCP connections, so suites fail once with an actionable message instead of every spec
// failing on connection errors.
func Broker(ctx context.Context) (string, error) {
	broker := os.Getenv(BrokerEnv)
	if broker == "" {
		return "", fmt.Errorf(
			`%s is not set; start a broker with "make kafka" or run "make test-unit"`,
			BrokerEnv,
		)
	}

	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", broker)
	if err != nil {
		return "", fmt.Errorf(
			"kafka broker %q from %s is unreachable: %w; "+
				`start one with "make kafka" or run "make test-unit"`,
			broker, BrokerEnv, err,
		)
	}
	return broker, conn.Close()
}

// CreateTopic creates a topic with one partition and replication factor 1 and waits
// until its partition has a leader, so producers do not hit UnknownTopicOrPartition.
func CreateTopic(ctx context.Context, brokers []string, topic string) error {
	client := &kafka.Client{Addr: kafka.TCP(brokers...)}

	resp, err := client.CreateTopics(ctx, &kafka.CreateTopicsRequest{
		Topics: []kafka.TopicConfig{{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}},
	})
	if err != nil {
		return fmt.Errorf("create topic %q: %w", topic, err)
	}
	if err = resp.Errors[topic]; err != nil {
		return fmt.Errorf("create topic %q: %w", topic, err)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var last error
	for {
		if last = topicReady(ctx, client, topic); last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf(
				"wait for topic %q to become ready: %w (last observed: %w)",
				topic, ctx.Err(), last,
			)
		case <-ticker.C:
		}
	}
}

// topicReady returns nil when the topic metadata reports no error and every partition
// has a leader. kafka-go reports these conditions in the response, not as an error.
func topicReady(ctx context.Context, client *kafka.Client, topic string) error {
	resp, err := client.Metadata(ctx, &kafka.MetadataRequest{Topics: []string{topic}})
	if err != nil {
		return err
	}
	i := slices.IndexFunc(resp.Topics, func(t kafka.Topic) bool { return t.Name == topic })
	if i < 0 {
		return errors.New("topic missing from metadata response")
	}
	t := resp.Topics[i]
	if t.Error != nil {
		return t.Error
	}
	if len(t.Partitions) == 0 {
		return errors.New("topic has no partitions")
	}
	for _, p := range t.Partitions {
		if p.Error != nil {
			return fmt.Errorf("partition %d: %w", p.ID, p.Error)
		}
		if p.Leader.Host == "" {
			return fmt.Errorf("partition %d has no leader", p.ID)
		}
	}
	return nil
}

// DeleteTopic deletes the topic.
func DeleteTopic(ctx context.Context, brokers []string, topic string) error {
	client := &kafka.Client{Addr: kafka.TCP(brokers...)}

	resp, err := client.DeleteTopics(ctx, &kafka.DeleteTopicsRequest{Topics: []string{topic}})
	if err != nil {
		return fmt.Errorf("delete topic %q: %w", topic, err)
	}
	if err = resp.Errors[topic]; err != nil {
		return fmt.Errorf("delete topic %q: %w", topic, err)
	}
	return nil
}

// NewReader returns a reader for the topic's single partition. MaxWait is lowered from
// the 10 s default because the first fetch on an empty topic blocks for the whole
// MaxWait even when a message is produced meanwhile.
func NewReader(brokers []string, topic string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		MaxWait: 500 * time.Millisecond,
	})
}

// ReadMessage reads the next message from r, giving up after timeout so callers can
// poll it from Eventually without one slow read consuming the whole spec budget.
func ReadMessage(
	ctx context.Context,
	r *kafka.Reader,
	timeout time.Duration,
) (kafka.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return r.ReadMessage(ctx)
}
