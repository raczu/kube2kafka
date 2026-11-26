package exporter_test

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"github.com/raczu/kube2kafka/internal/kafkatest"
	"github.com/raczu/kube2kafka/pkg/exporter"
	"github.com/raczu/kube2kafka/pkg/processor"
)

// exportInBackground runs exp on its own context until the spec ends, then asserts that
// it stopped without error, so a fatal export error fails the spec with its real cause.
func exportInBackground(exp *exporter.Exporter) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- exp.Export(ctx) }()

	DeferCleanup(func(cctx SpecContext) {
		cancel()
		select {
		case err := <-done:
			Expect(err).NotTo(HaveOccurred())
		case <-cctx.Done():
		}
	}, NodeTimeout(10*time.Second))
}

var _ = Describe("Exporter", Label("integration"), func() {
	var (
		source  *processor.KafkaMessageBuffer
		brokers []string
		topic   string
		logger  *zap.Logger
	)

	BeforeEach(func() {
		source = processor.NewKafkaMessageBuffer(8)
		logger = zap.NewNop()
		brokers = []string{os.Getenv(kafkatest.BrokerEnv)}
		topic = uuid.New().String()
	})

	When("exporting messages", func() {
		Context("and the topic does not exist", func() {
			It("should exit immediately with an error", func(ctx SpecContext) {
				source.Write(&kafka.Message{Key: []byte("key"), Value: []byte("value")})
				exp := exporter.New(source, topic, brokers, exporter.WithLogger(logger))

				err := exp.Export(ctx)
				Expect(err).To(MatchError(ContainSubstring(kafka.UnknownTopicOrPartition.Error())))
			}, SpecTimeout(30*time.Second))
		})

		Context("and the topic exists", func() {
			var (
				reader *kafka.Reader
			)

			BeforeEach(func(ctx SpecContext) {
				Expect(kafkatest.CreateTopic(ctx, brokers, topic)).To(Succeed())
				DeferCleanup(func(ctx SpecContext) {
					// No need to wait for the deletion: topic names are unique UUIDs.
					Expect(kafkatest.DeleteTopic(ctx, brokers, topic)).To(Succeed())
				}, NodeTimeout(30*time.Second))

				reader = kafkatest.NewReader(brokers, topic)
				DeferCleanup(reader.Close)
			}, NodeTimeout(30*time.Second))

			It("should export a message successfully", func(ctx SpecContext) {
				message := &kafka.Message{Key: []byte("key"), Value: []byte("value")}
				source.Write(message)
				exportInBackground(
					exporter.New(source, topic, brokers, exporter.WithLogger(logger)),
				)

				var received kafka.Message
				Eventually(func(g Gomega) {
					var err error
					received, err = kafkatest.ReadMessage(ctx, reader, time.Second)
					g.Expect(err).NotTo(HaveOccurred())
				}).WithContext(ctx).Should(Succeed())

				Expect(received.Key).To(Equal(message.Key))
				Expect(received.Value).To(Equal(message.Value))
			}, SpecTimeout(30*time.Second))

			It("should export multiple messages at once successfully", func(ctx SpecContext) {
				messages := []*kafka.Message{
					{Key: []byte("key-a"), Value: []byte("value-a")},
					{Key: []byte("key-b"), Value: []byte("value-b")},
					{Key: []byte("key-c"), Value: []byte("value-c")},
				}
				for _, msg := range messages {
					source.Write(msg)
				}
				exportInBackground(
					exporter.New(source, topic, brokers, exporter.WithLogger(logger)),
				)

				var received []kafka.Message
				Eventually(func(g Gomega) {
					msg, err := kafkatest.ReadMessage(ctx, reader, time.Second)
					g.Expect(err).NotTo(HaveOccurred())
					received = append(received, msg)
					g.Expect(received).To(HaveLen(len(messages)))
				}).WithContext(ctx).Should(Succeed())

				for i, msg := range messages {
					Expect(received[i].Key).To(Equal(msg.Key))
					Expect(received[i].Value).To(Equal(msg.Value))
				}
			}, SpecTimeout(30*time.Second))
		})
	})
})
