package exporter

import (
	"crypto/tls"
	"errors"

	"github.com/segmentio/kafka-go"

	"github.com/raczu/kube2kafka/pkg/processor"
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
//
// A rejected broker certificate is fatal because certificates are read only at startup,
// so retrying can never recover. A TLS alert from the broker (e.g. it rejects our client
// certificate) is not covered: crypto/tls reports received alerts with an unexported
// type, so they cannot be matched without relying on error text.
func isFatalError(err error) bool {
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return true
	}
	return errors.Is(err, kafka.UnknownTopicOrPartition) ||
		errors.Is(err, kafka.UnsupportedSASLMechanism) ||
		errors.Is(err, kafka.IllegalSASLState) ||
		errors.Is(err, kafka.SASLAuthenticationFailed) ||
		errors.Is(err, kafka.TopicAuthorizationFailed)
}
