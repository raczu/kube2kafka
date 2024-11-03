package exporter_test

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raczu/kube2kafka/pkg/exporter"
	log "github.com/raczu/kube2kafka/pkg/logger"
	"github.com/raczu/kube2kafka/pkg/processor"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"os"
	"time"
)

var _ = Describe("Exporter", func() {
	var (
		source  *processor.KafkaMessageBuffer
		brokers []string
		topic   string
		exp     *exporter.Exporter
		logger  *zap.Logger
	)

	BeforeEach(func() {
		source = processor.NewKafkaMessageBuffer(8)
		opts := log.Options{
			Output: &bytes.Buffer{},
		}
		logger = log.New(log.UseOptions(&opts))
		brokers = []string{os.Getenv("KAFKA_TEST_BROKER")}
	})

	When("exporting messages", func() {
		Context("and the topic does not exist", func() {
			BeforeEach(func() {
				source.Write(&kafka.Message{Key: []byte("key"), Value: []byte("value")})
			})

			It("should exit immediately with an error", func() {
				topic = uuid.New().String()
				exp = exporter.New(source, topic, brokers, exporter.WithLogger(logger))

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				err := exp.Export(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(kafka.UnknownTopicOrPartition.Error()))
			})
		})

		Context("and the topic exists", func() {
			var (
				admin *kafka.Client
			)

			BeforeEach(func() {
				topic = uuid.New().String()
				exp = exporter.New(source, topic, brokers, exporter.WithLogger(logger))

				admin = &kafka.Client{
					Addr: kafka.TCP(brokers...),
				}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, err := admin.CreateTopics(ctx, &kafka.CreateTopicsRequest{
					Topics: []kafka.TopicConfig{
						{
							Topic:             topic,
							NumPartitions:     1,
							ReplicationFactor: 1,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())

				// Wait for the topic to be created.
				Eventually(func() error {
					ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
					defer cancel()
					_, err := admin.Metadata(ctx, &kafka.MetadataRequest{
						Topics: []string{topic},
					})
					return err
				}, 5*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, err := admin.DeleteTopics(ctx, &kafka.DeleteTopicsRequest{
					Topics: []string{topic},
				})
				Expect(err).NotTo(HaveOccurred())
				// There is no need to wait for the topic to be deleted as we use
				// UUIDs for topic names, there should be no conflicts.
			})

			It("should export a message successfully", func() {
				message := &kafka.Message{Key: []byte("key"), Value: []byte("value")}
				source.Write(message)

				r := kafka.NewReader(kafka.ReaderConfig{
					Brokers: brokers,
					Topic:   topic,
				})

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				done := make(chan struct{})
				var received kafka.Message
				var rerr error

				go func() {
					defer close(done)
					received, rerr = r.ReadMessage(ctx)
					cancel()
				}()

				err := exp.Export(ctx)
				Expect(err).NotTo(HaveOccurred())

				<-done
				Expect(rerr).NotTo(HaveOccurred())
				Expect(received.Key).To(Equal(message.Key))
				Expect(received.Value).To(Equal(message.Value))
			})

			It("should export multiple messages at once successfully", func() {
				messages := []*kafka.Message{
					{Key: []byte("key-a"), Value: []byte("value-a")},
					{Key: []byte("key-b"), Value: []byte("value-b")},
					{Key: []byte("key-c"), Value: []byte("value-c")},
				}
				for _, msg := range messages {
					source.Write(msg)
				}

				r := kafka.NewReader(kafka.ReaderConfig{
					Brokers: brokers,
					Topic:   topic,
				})

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				done := make(chan struct{})
				var received []kafka.Message
				var rerr error

				go func() {
					defer close(done)
					for i := 0; i < len(messages); i++ {
						msg, err := r.ReadMessage(ctx)
						if err != nil {
							rerr = err
							return
						}
						received = append(received, msg)
					}
					cancel()
				}()

				err := exp.Export(ctx)
				Expect(err).NotTo(HaveOccurred())

				<-done
				Expect(rerr).NotTo(HaveOccurred())
				Expect(received).To(HaveLen(len(messages)))
				for i, msg := range messages {
					Expect(received[i].Key).To(Equal(msg.Key))
					Expect(received[i].Value).To(Equal(msg.Value))
				}

				for i := 1; i < len(received); i++ {
					rtime := received[i].Time
					Expect(rtime).To(BeTemporally("~", received[0].Time, time.Millisecond))
				}
			})
		})
	})
})
