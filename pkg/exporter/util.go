package exporter

import (
	"errors"
	"github.com/raczu/kube2kafka/pkg/processor"
	"github.com/segmentio/kafka-go"
)

// tryReadUpTo tries to read n messages from the buffer. If there are fewer than n messages
// in the buffer, it returns all available messages. To ensure that some messages are read,
// the caller should at first check if there are any messages in the buffer.
func tryReadUpTo(buffer *processor.KafkaMessageBuffer, n int) []kafka.Message {
	var messages []kafka.Message
	for i := 0; i < n; i++ {
		message, ok := buffer.Read()
		if !ok {
			break
		}
		messages = append(messages, *message)
	}
	return messages
}

// isFatalError checks if the provided error is any of the Kafka fatal errors.
// Fatal errors are those that should stop the exporter. They are related to the
// misconfiguration of the Kafka cluster or the exporter itself.
func isFatalError(err error) bool {
	return errors.Is(err, kafka.UnknownTopicOrPartition) ||
		errors.Is(err, kafka.UnsupportedSASLMechanism) ||
		errors.Is(err, kafka.IllegalSASLState) ||
		errors.Is(err, kafka.SASLAuthenticationFailed) ||
		errors.Is(err, kafka.TopicAuthorizationFailed)
}
