package manager_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	k2kconfig "github.com/raczu/kube2kafka/internal/config"
	"github.com/raczu/kube2kafka/internal/kafkatest"
	"github.com/raczu/kube2kafka/internal/manager"
	"github.com/raczu/kube2kafka/pkg/kube"
	kubewatcher "github.com/raczu/kube2kafka/pkg/kube/watcher"
)

// startInBackground runs mgr on its own context until the spec ends, then asserts that
// it stopped without error.
func startInBackground(mgr *manager.Manager) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()

	DeferCleanup(func(cctx SpecContext) {
		cancel()
		select {
		case err := <-done:
			Expect(err).NotTo(HaveOccurred())
		case <-cctx.Done():
		}
	}, NodeTimeout(cleanupTimeout))
}

var _ = Describe("Manager", func() {
	var (
		config    *k2kconfig.Config
		clientset *fake.Clientset
		mgr       *manager.Manager
		logger    *zap.Logger
		logs      *observer.ObservedLogs
	)

	BeforeEach(func() {
		// Monkey patch the kube client creation to return the fake clientset.
		clientset = fake.NewClientset()
		kubewatcher.CreateKubeClient = func(config *rest.Config) kubernetes.Interface {
			return clientset
		}

		config = &k2kconfig.Config{
			ClusterName:     "tests.kube2kafka.local",
			TargetNamespace: corev1.NamespaceDefault,
			BufferSize:      5,
			MaxEventAge:     1 * time.Hour,
			Kafka: &k2kconfig.KafkaConfig{
				Brokers:        []string{os.Getenv(kafkatest.BrokerEnv)},
				Topic:          uuid.New().String(),
				RawCompression: "none",
			},
		}

		var core zapcore.Core
		core, logs = observer.New(zap.DebugLevel)
		logger = zap.New(core)
	})

	When("setting up the manager", func() {
		It("should return error if TLS config has issues", func() {
			// An empty file is not a valid CA certificate.
			cafile := filepath.Join(GinkgoT().TempDir(), "ca.crt")
			Expect(os.WriteFile(cafile, nil, 0o600)).To(Succeed())

			config.Kafka.RawTLS = &k2kconfig.RawTLSData{
				CA: cafile,
			}

			mgr = manager.New(config, &rest.Config{}, logger)
			err := mgr.Setup()
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

		BeforeEach(func(ctx SpecContext) {
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

			// The fake clientset populates server-side fields (e.g. managedFields) on create,
			// so keep the stored object as it is the one observed by the watcher.
			var err error
			event, err = clientset.CoreV1().
				Events(config.TargetNamespace).
				Create(ctx, event, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		}, NodeTimeout(specTimeout))

		It("should return error if manager was not set up", func() {
			mgr = manager.New(config, &rest.Config{}, logger)

			err := mgr.Start(context.Background())
			Expect(err).To(MatchError(manager.ErrManagerNotSetup))
		})

		Context("and one of the components fails", func() {
			BeforeEach(func() {
				mgr = manager.New(config, &rest.Config{}, logger)
				err := mgr.Setup()
				Expect(err).NotTo(HaveOccurred())
			})

			It("should stop immediately and return error if watcher fails", func() {
				// A canceled context makes the initial cache sync fail deterministically.
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				err := mgr.Start(ctx)
				Expect(err).To(
					MatchError(ContainSubstring("initial cache sync for watcher failed")),
				)
			})

			It("should stop immediately and return error if exporter fails",
				Label("integration"), func(ctx SpecContext) {
					// The topic name is a fresh UUID, so the topic does not exist.
					err := mgr.Start(ctx)
					Expect(err).To(
						MatchError(ContainSubstring(kafka.UnknownTopicOrPartition.Error())),
					)
				}, SpecTimeout(specTimeout))
		})

		Context("and all components work properly", Label("integration"), func() {
			var (
				reader *kafka.Reader
			)

			BeforeEach(func(ctx SpecContext) {
				Expect(kafkatest.CreateTopic(ctx, config.Kafka.Brokers, config.Kafka.Topic)).
					To(Succeed())
				DeferCleanup(func(ctx SpecContext) {
					// No need to wait for the deletion: topic names are unique UUIDs.
					Expect(kafkatest.DeleteTopic(ctx, config.Kafka.Brokers, config.Kafka.Topic)).
						To(Succeed())
				}, NodeTimeout(specTimeout))

				reader = kafkatest.NewReader(config.Kafka.Brokers, config.Kafka.Topic)
				DeferCleanup(reader.Close)
			}, NodeTimeout(specTimeout))

			It("should export the observed event", func(ctx SpecContext) {
				mgr = manager.New(config, &rest.Config{}, logger)
				Expect(mgr.Setup()).To(Succeed())

				// The manager runs on its own context so that reading never stops it.
				startInBackground(mgr)

				// Until the watcher has synced, stopping the manager returns a sync error.
				Eventually(func() int {
					return logs.FilterMessage("initial watcher cache synced").Len()
				}).WithContext(ctx).Should(Equal(1))

				var message kafka.Message
				Eventually(func(g Gomega) {
					var err error
					message, err = kafkatest.ReadMessage(ctx, reader, readTimeout)
					g.Expect(err).NotTo(HaveOccurred())
				}).WithContext(ctx).Should(Succeed())

				expected, err := json.Marshal(&kube.EnhancedEvent{
					ClusterName: config.ClusterName,
					Event:       *event,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(message.Value).To(MatchJSON(expected))
			}, SpecTimeout(specTimeout))
		})
	})
})
