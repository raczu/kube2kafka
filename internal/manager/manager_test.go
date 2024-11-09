package manager_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k2kconfig "github.com/raczu/kube2kafka/internal/config"
	"github.com/raczu/kube2kafka/internal/manager"
	"github.com/raczu/kube2kafka/pkg/kube"
	kubewatcher "github.com/raczu/kube2kafka/pkg/kube/watcher"
	log "github.com/raczu/kube2kafka/pkg/logger"
	"github.com/raczu/kube2kafka/pkg/processor"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"os"
	"reflect"
	"time"
)

var _ = Describe("Manager", func() {
	var (
		config    *k2kconfig.Config
		clientset *fake.Clientset
		mgr       *manager.Manager
		logger    *zap.Logger
	)

	BeforeEach(func() {
		// Monkey patch the kube client creation to return the fake clientset.
		clientset = fake.NewSimpleClientset()
		kubewatcher.CreateKubeClient = func(config *rest.Config) kubernetes.Interface {
			return clientset
		}

		config = &k2kconfig.Config{
			ClusterName:     "tests.kube2kafka.local",
			TargetNamespace: corev1.NamespaceDefault,
			BufferSize:      5,
			MaxEventAge:     1 * time.Hour,
			Kafka: &k2kconfig.KafkaConfig{
				Brokers:        []string{os.Getenv("KAFKA_TEST_BROKER")},
				Topic:          "placeholder",
				RawCompression: "none",
			},
		}

		opts := log.Options{
			Output: &bytes.Buffer{},
		}
		logger = log.New(log.UseOptions(&opts))
	})

	When("setting up the manager", func() {
		It("should return error if TLS config has issues", func() {
			cafile, err := os.CreateTemp("", "kube2kafka-ca.crt")
			defer os.Remove(cafile.Name())
			Expect(err).NotTo(HaveOccurred())

			config.Kafka.RawTLS = &k2kconfig.RawTLSData{
				CA: cafile.Name(),
			}

			mgr = manager.New(config, &rest.Config{}, logger)
			err = mgr.Setup()
			Expect(err).To(HaveOccurred())
		})

		It("should not return error if TLS config is nil", func() {
			mgr = manager.New(config, &rest.Config{}, logger)
			err := mgr.Setup()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error if SASL config has issues", func() {
			config.Kafka.RawSASL = &k2kconfig.RawSASLData{
				Username:  "user\x00name",
				Password:  "pwd",
				Mechanism: "sha256",
			}

			mgr = manager.New(config, &rest.Config{}, logger)
			err := mgr.Setup()
			Expect(err).To(HaveOccurred())
		})

		It("should not return error if SASL config is nil", func() {
			mgr = manager.New(config, &rest.Config{}, logger)
			err := mgr.Setup()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("starting the manager", func() {
		var (
			event *corev1.Event
		)

		BeforeEach(func() {
			now := metav1.Now()
			event = &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: corev1.NamespaceDefault,
					Name:      "test",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Node",
				},
				FirstTimestamp: now,
				LastTimestamp:  now,
				Reason:         "NodeAllocatableEnforced",
				Message:        "Updated Node Allocatable limit across pods",
				Type:           "Normal",
				Source: corev1.EventSource{
					Component: "kubelet",
				},
			}

			_, err := clientset.CoreV1().
				Events(config.TargetNamespace).
				Create(context.TODO(), event, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error if manager was not set up", func() {
			mgr = manager.New(config, &rest.Config{}, logger)

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			err := mgr.Start(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(manager.ErrManagerNotSetup))
		})

		Context("and one of the components fails", func() {
			BeforeEach(func() {
				mgr = manager.New(config, &rest.Config{}, logger)
				err := mgr.Setup()
				Expect(err).NotTo(HaveOccurred())
			})

			It("should stop immediately and return error if watcher fails", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
				defer cancel()

				err := mgr.Start(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("initial cache sync for watcher failed"))
			})

			It("should stop immediately and return error if exporter fails", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer cancel()

				err := mgr.Start(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(kafka.UnknownTopicOrPartition.Error()))
			})
		})

		Context("and all components works properly", func() {
			var (
				admin *kafka.Client
			)

			BeforeEach(func() {
				config.Kafka.Topic = uuid.New().String()
				admin = &kafka.Client{
					Addr: kafka.TCP(config.Kafka.Brokers...),
				}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, err := admin.CreateTopics(ctx, &kafka.CreateTopicsRequest{
					Topics: []kafka.TopicConfig{
						{
							Topic:             config.Kafka.Topic,
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
						Topics: []string{config.Kafka.Topic},
					})
					return err
				}, 5*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, err := admin.DeleteTopics(ctx, &kafka.DeleteTopicsRequest{
					Topics: []string{config.Kafka.Topic},
				})
				Expect(err).NotTo(HaveOccurred())
				// There is no need to wait for the topic to be deleted as we use
				// UUIDs for topic names, there should be no conflicts.
			})

			When("some filters are defined", func() {
				var (
					expected *corev1.Event
				)

				BeforeEach(func() {
					now := metav1.Now()
					expected = &corev1.Event{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: corev1.NamespaceDefault,
							Name:      "expected",
						},
						FirstTimestamp: now,
						LastTimestamp:  now,
						InvolvedObject: corev1.ObjectReference{
							Kind: "Pod",
						},
						Reason:  "Created",
						Message: "Pod created successfully",
						Type:    "Normal",
						Source: corev1.EventSource{
							Component: "kubelet",
						},
					}

					_, err := clientset.CoreV1().
						Events(config.TargetNamespace).
						Create(context.TODO(), expected, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())

					config.Filters = []processor.Filter{
						{
							Reason: "(?i)^created",
						},
					}

					mgr = manager.New(config, &rest.Config{}, logger)
					err = mgr.Setup()
					Expect(err).NotTo(HaveOccurred())
				})

				It("should export only events matching defined filters", func() {
					r := kafka.NewReader(kafka.ReaderConfig{
						Brokers: config.Kafka.Brokers,
						Topic:   config.Kafka.Topic,
					})

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					done := make(chan struct{})
					var message kafka.Message
					var rerr error

					go func() {
						defer close(done)
						message, rerr = r.ReadMessage(ctx)
						cancel()
					}()

					err := mgr.Start(ctx)
					Expect(err).NotTo(HaveOccurred())

					<-done
					Expect(rerr).NotTo(HaveOccurred())

					var received kube.EnhancedEvent
					err = json.Unmarshal(message.Value, &received)
					Expect(err).NotTo(HaveOccurred())

					Expect(received.ClusterName).To(Equal(config.ClusterName))
					Expect(
						received.Event.InvolvedObject.Kind,
					).To(Equal(expected.InvolvedObject.Kind))
					Expect(received.Event.Reason).To(Equal(expected.Reason))
					Expect(received.Event.Message).To(Equal(expected.Message))
				})
			})

			When("customizing payload", func() {
				It("should customize payload based on defined selectors", func() {
					config.Selectors = []processor.Selector{
						{
							Key:   "cluster",
							Value: "{{ .ClusterName }}",
						},
						{
							Key:   "kind",
							Value: "{{ .InvolvedObject.Kind }}",
						},
						{
							Key:   "reason",
							Value: "{{ .Reason }}",
						},
					}

					mgr = manager.New(config, &rest.Config{}, logger)
					err := mgr.Setup()
					Expect(err).NotTo(HaveOccurred())

					r := kafka.NewReader(kafka.ReaderConfig{
						Brokers: config.Kafka.Brokers,
						Topic:   config.Kafka.Topic,
					})

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					done := make(chan struct{})
					var message kafka.Message
					var rerr error

					go func() {
						defer close(done)
						message, rerr = r.ReadMessage(ctx)
						cancel()
					}()

					err = mgr.Start(ctx)
					Expect(err).NotTo(HaveOccurred())

					<-done
					Expect(rerr).NotTo(HaveOccurred())

					var received map[string]string
					err = json.Unmarshal(message.Value, &received)
					Expect(err).NotTo(HaveOccurred())

					Expect(len(received)).To(Equal(len(config.Selectors)))
					Expect(received).To(HaveKeyWithValue("cluster", config.ClusterName))
					Expect(received).To(HaveKeyWithValue("kind", event.InvolvedObject.Kind))
					Expect(received).To(HaveKeyWithValue("reason", event.Reason))
				})

				It("should export event as it is if no selectors are defined", func() {
					mgr = manager.New(config, &rest.Config{}, logger)
					err := mgr.Setup()
					Expect(err).NotTo(HaveOccurred())

					r := kafka.NewReader(kafka.ReaderConfig{
						Brokers: config.Kafka.Brokers,
						Topic:   config.Kafka.Topic,
					})

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					done := make(chan struct{})
					var message kafka.Message
					var rerr error

					go func() {
						defer close(done)
						message, rerr = r.ReadMessage(ctx)
						cancel()
					}()

					err = mgr.Start(ctx)
					Expect(err).NotTo(HaveOccurred())

					<-done
					Expect(rerr).NotTo(HaveOccurred())

					var expected []byte
					expected, err = json.Marshal(
						&kube.EnhancedEvent{
							ClusterName: config.ClusterName,
							Event:       *event,
						})
					Expect(err).NotTo(HaveOccurred())

					isEqual := reflect.DeepEqual(message.Value, expected)
					Expect(isEqual).To(BeTrue())
				})

				It("should export default payload when any selection causes an error", func() {
					config.Selectors = []processor.Selector{
						{
							Key:   "cluster",
							Value: "{{ .NonExistingField }}",
						},
					}

					mgr = manager.New(config, &rest.Config{}, logger)
					err := mgr.Setup()
					Expect(err).NotTo(HaveOccurred())

					r := kafka.NewReader(kafka.ReaderConfig{
						Brokers: config.Kafka.Brokers,
						Topic:   config.Kafka.Topic,
					})

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					done := make(chan struct{})
					var message kafka.Message
					var rerr error

					go func() {
						defer close(done)
						message, rerr = r.ReadMessage(ctx)
						cancel()
					}()

					err = mgr.Start(ctx)
					Expect(err).NotTo(HaveOccurred())

					<-done
					Expect(rerr).NotTo(HaveOccurred())

					var received map[string]string
					err = json.Unmarshal(message.Value, &received)
					Expect(err).NotTo(HaveOccurred())

					expected := processor.RelevantFieldSelection(
						&kube.EnhancedEvent{
							ClusterName: config.ClusterName,
							Event:       *event,
						},
					)
					isEqual := reflect.DeepEqual(received, expected)
					Expect(isEqual).To(BeTrue())
				})
			})
		})
	})
})
