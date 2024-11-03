package exporter

import (
	"context"
	"crypto/tls"
	"fmt"
	log "github.com/raczu/kube2kafka/pkg/logger"
	"github.com/raczu/kube2kafka/pkg/processor"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"go.uber.org/zap"
	"net"
	"time"
)

const (
	DefaultMaxAttempts = 3
	DefaultBatchSize   = 16
)

type Option func(*Exporter)

type Exporter struct {
	source *processor.KafkaMessageBuffer
	writer *kafka.Writer
	logger *zap.Logger
}

func New(
	source *processor.KafkaMessageBuffer,
	topic string,
	brokers []string,
	opts ...Option,
) *Exporter {
	transport := &kafka.Transport{
		Dial: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
	}

	e := &Exporter{
		source: source,
		writer: &kafka.Writer{
			Addr:        kafka.TCP(brokers...),
			Topic:       topic,
			Balancer:    &kafka.LeastBytes{},
			MaxAttempts: DefaultMaxAttempts,
			BatchSize:   DefaultBatchSize,
			Transport:   transport,
		},
		logger: log.New().Named("exporter"),
	}

	for _, opt := range opts {
		opt(e)
	}

	sublogger := e.logger.Named("writer").Sugar()
	e.writer.Logger = kafka.LoggerFunc(sublogger.Debugf)
	e.writer.ErrorLogger = kafka.LoggerFunc(sublogger.Errorf)
	return e
}

// write writes the given messages to the Kafka topic. It returns the number of messages
// successfully written, boolean indicating if the error is fatal and the error itself.
func (e *Exporter) write(ctx context.Context, messages []kafka.Message) (int, bool, error) {
	written := len(messages)

	switch err := e.writer.WriteMessages(ctx, messages...).(type) {
	case nil:
		return written, false, nil
	case kafka.WriteErrors:
		for _, werr := range err {
			if werr != nil {
				written--
				// If the error is fatal, return immediately as
				// the writer will not be able to send anything.
				if isFatalError(werr) {
					return 0, true, werr
				}
			}
		}
		return written, false, err
	default:
		return 0, isFatalError(err), err
	}
}

func (e *Exporter) export(ctx context.Context) error {
	if e.source.Size() == 0 {
		return nil
	}

	messages := tryReadUpTo(e.source, e.writer.BatchSize)
	written, fatal, err := e.write(ctx, messages)
	if err != nil {
		if fatal {
			return fmt.Errorf("encountered fatal error: %w", err)
		}
		e.logger.Error("failed to write messages to kafka", zap.Error(err))
	}
	e.logger.Info(
		"successfully wrote given number of messages to kafka",
		zap.Int("written", written),
	)
	return nil
}

// Export reads messages from the source buffer and writes them to the Kafka topic.
// It returns an error in case of encountering a fatal error caused by the misconfiguration
// of the Kafka cluster or the exporter itself preventing further operation.
func (e *Exporter) Export(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if err := e.writer.Close(); err != nil {
				return fmt.Errorf("failed to close kafka writer in exporter: %w", err)
			}
			return nil
		case <-ticker.C:
			if err := e.export(ctx); err != nil {
				return err
			}
		}
	}
}

// UseTLS configures the exporter to use TLS for communication with the Kafka brokers.
func UseTLS(config *tls.Config) Option {
	return func(e *Exporter) {
		transport := e.writer.Transport.(*kafka.Transport)
		transport.TLS = config
	}
}

// UseSASL configures the exporter to use SASL for authentication with the Kafka brokers.
func UseSASL(mechanism sasl.Mechanism) Option {
	return func(e *Exporter) {
		transport := e.writer.Transport.(*kafka.Transport)
		transport.SASL = mechanism
	}
}

// WithCompression configures the exporter to use the specified compression codec.
func WithCompression(codec kafka.Compression) Option {
	return func(e *Exporter) {
		e.writer.Compression = codec
	}
}

// WithLogger sets the logger for the exporter.
func WithLogger(logger *zap.Logger) Option {
	return func(e *Exporter) {
		e.logger = logger
	}
}
