package watcher_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/raczu/kube2kafka/pkg/kube"
	kubewatcher "github.com/raczu/kube2kafka/pkg/kube/watcher"
)

const (
	specTimeout = 10 * time.Second
	// quietPeriod is how long a spec checks that nothing is written to the buffer.
	quietPeriod = 500 * time.Millisecond
)

// watchUntilSynced runs w on its own context until the spec ends and waits for the
// initial cache sync, so the cleanup can assert that Watch stopped without error.
func watchUntilSynced(ctx context.Context, w *kubewatcher.Watcher, logs *observer.ObservedLogs) {
	wctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Watch(wctx) }()

	DeferCleanup(func(cctx SpecContext) {
		cancel()
		select {
		case err := <-done:
			Expect(err).NotTo(HaveOccurred())
		case <-cctx.Done():
		}
	}, NodeTimeout(specTimeout))

	Eventually(func() int {
		return logs.FilterMessage("initial watcher cache synced").Len()
	}).WithContext(ctx).Should(Equal(1))
}

var _ = Describe("Watcher", func() {
	var (
		clientset *fake.Clientset
		logs      *observer.ObservedLogs
		cluster   *kube.Cluster
		watcher   *kubewatcher.Watcher
	)

	BeforeEach(func() {
		clientset = fake.NewClientset()
		kubewatcher.CreateKubeClient = func(config *rest.Config) kubernetes.Interface {
			return clientset
		}

		var core zapcore.Core
		core, logs = observer.New(zap.DebugLevel)

		cluster = &kube.Cluster{
			Name:            "test.cluster.local",
			TargetNamespace: corev1.NamespaceAll,
		}

		watcher = kubewatcher.New(&rest.Config{}, *cluster, kubewatcher.WithLogger(zap.New(core)))
	})

	bufferSize := func() int {
		return watcher.GetBuffer().Size()
	}

	When("syncing initial watcher cache", func() {
		It("should return error if the context is canceled before the sync is done", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := watcher.Watch(ctx)
			Expect(err).To(MatchError(ContainSubstring("initial cache sync for watcher failed")))
		})

		Context("and there are no events", func() {
			It("should not write anything to the buffer", func(ctx SpecContext) {
				watchUntilSynced(ctx, watcher, logs)

				Consistently(bufferSize, quietPeriod).Should(Equal(0))
			}, SpecTimeout(specTimeout))
		})

		Context("and there are events", func() {
			var (
				event *corev1.Event
			)

			BeforeEach(func() {
				now := metav1.Now()
				event = &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-event",
						Namespace: cluster.TargetNamespace,
					},
					InvolvedObject: corev1.ObjectReference{
						Name: "test-pod",
					},
					Reason:         "test-reason",
					Type:           corev1.EventTypeNormal,
					FirstTimestamp: now,
					LastTimestamp:  now,
				}
			})

			It("should write event to the buffer", func(ctx SpecContext) {
				_, err := clientset.CoreV1().
					Events(cluster.TargetNamespace).
					Create(ctx, event, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				watchUntilSynced(ctx, watcher, logs)

				Eventually(bufferSize).WithContext(ctx).Should(Equal(1))
			}, SpecTimeout(specTimeout))

			It("should not write event older than the max age", func(ctx SpecContext) {
				event.LastTimestamp = metav1.NewTime(
					time.Now().Add(-2 * kubewatcher.DefaultMaxEventAge),
				)
				_, err := clientset.CoreV1().
					Events(cluster.TargetNamespace).
					Create(ctx, event, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				watchUntilSynced(ctx, watcher, logs)

				Consistently(bufferSize, quietPeriod).Should(Equal(0))
			}, SpecTimeout(specTimeout))
		})
	})

	When("watching and observed a new event", func() {
		var (
			event *corev1.Event
		)

		BeforeEach(func(ctx SpecContext) {
			// Warmup the cache.
			dummy := &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy",
					Namespace: cluster.TargetNamespace,
				},
				LastTimestamp: metav1.Now(),
			}

			_, err := clientset.CoreV1().
				Events(cluster.TargetNamespace).
				Create(ctx, dummy, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			watchUntilSynced(ctx, watcher, logs)

			// Wait for the cache to sync.
			Eventually(bufferSize).WithContext(ctx).Should(Equal(1))

			// Drain the buffer to avoid false positives.
			_, ok := watcher.GetBuffer().Read()
			Expect(ok).To(BeTrue())

			now := metav1.Now()
			event = &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-event",
					Namespace:       cluster.TargetNamespace,
					ResourceVersion: "1",
				},
				InvolvedObject: corev1.ObjectReference{
					Name: "test-pod",
				},
				Reason:         "test-reason",
				Type:           corev1.EventTypeNormal,
				FirstTimestamp: now,
				LastTimestamp:  now,
			}
		}, NodeTimeout(specTimeout))

		It("should write the event to the buffer", func(ctx SpecContext) {
			_, err := clientset.CoreV1().
				Events(cluster.TargetNamespace).
				Create(ctx, event, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(bufferSize).WithContext(ctx).Should(Equal(1))
		}, SpecTimeout(specTimeout))

		Context("but this is an update of an existing event", func() {
			BeforeEach(func(ctx SpecContext) {
				_, err := clientset.CoreV1().
					Events(cluster.TargetNamespace).
					Create(ctx, event, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				Eventually(bufferSize).WithContext(ctx).Should(Equal(1))
			}, NodeTimeout(specTimeout))

			It("should write the updated event to the buffer", func(ctx SpecContext) {
				event.LastTimestamp = metav1.Now()
				event.ResourceVersion = "2"

				_, err := clientset.CoreV1().
					Events(cluster.TargetNamespace).
					Update(ctx, event, metav1.UpdateOptions{})
				Expect(err).NotTo(HaveOccurred())

				Eventually(bufferSize).WithContext(ctx).Should(Equal(2))
			}, SpecTimeout(specTimeout))

			It("should not write the event if the resource version is the same",
				func(ctx SpecContext) {
					event.LastTimestamp = metav1.Now()

					_, err := clientset.CoreV1().
						Events(cluster.TargetNamespace).
						Update(ctx, event, metav1.UpdateOptions{})
					Expect(err).NotTo(HaveOccurred())

					Consistently(bufferSize, quietPeriod).Should(Equal(1))
				}, SpecTimeout(specTimeout))
		})
	})
})
