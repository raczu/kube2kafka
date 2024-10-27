package kube_test

import (
	"bytes"
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raczu/kube2kafka/pkg/kube"
	log "github.com/raczu/kube2kafka/pkg/logger"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"time"
)

var _ = Describe("Watcher", func() {
	var (
		clientset *fake.Clientset
		logger    *zap.Logger
		cluster   *kube.Cluster
		watcher   *kube.Watcher
	)

	BeforeEach(func() {
		clientset = fake.NewSimpleClientset()
		kube.CreateKubeClient = func(config *rest.Config) kubernetes.Interface {
			return clientset
		}

		opts := log.Options{
			Output: &bytes.Buffer{},
		}
		logger = log.New(log.UseOptions(&opts))

		cluster = &kube.Cluster{
			Name:            "test.cluster.local",
			TargetNamespace: corev1.NamespaceAll,
		}

		watcher = kube.NewWatcher(&rest.Config{}, *cluster, kube.WithLogger(logger))
	})

	When("syncing initial watcher cache", func() {
		It("should return error if the context is canceled before the sync is done", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
			defer cancel()

			err := watcher.Watch(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("initial cache sync for watcher failed"))
		})

		Context("and there are no events", func() {
			It("should not write anything to the buffer", func() {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				go func() {
					_ = watcher.Watch(ctx)
				}()
				Expect(watcher.GetBuffer().Size()).To(Equal(0))
			})
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

			AfterEach(func() {
				// Wait for goroutines to finish.
				time.Sleep(1 * time.Second)
			})

			It("should write event to the buffer", func() {
				_, err := clientset.CoreV1().
					Events(cluster.TargetNamespace).
					Create(context.TODO(), event, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				go func() {
					_ = watcher.Watch(ctx)
				}()

				time.Sleep(500 * time.Millisecond)
				Expect(watcher.GetBuffer().Size()).To(Equal(1))
			})

			It("should not write event older than the max age", func() {
				event.LastTimestamp = metav1.NewTime(time.Now().Add(-2 * kube.DefaultMaxEventAge))
				_, err := clientset.CoreV1().
					Events(cluster.TargetNamespace).
					Create(context.TODO(), event, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				go func() {
					_ = watcher.Watch(ctx)
				}()

				time.Sleep(500 * time.Millisecond)
				Expect(watcher.GetBuffer().Size()).To(Equal(0))
			})
		})
	})

	When("watching and observed a new event", func() {
		var (
			ctx    context.Context
			cancel context.CancelFunc
			event  *corev1.Event
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			go func() {
				_ = watcher.Watch(ctx)
			}()

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
		})

		AfterEach(func() {
			cancel()
			// Wait for goroutines to finish.
			time.Sleep(1 * time.Second)
		})

		It("should write the event to the buffer", func() {
			_, err := clientset.CoreV1().
				Events(cluster.TargetNamespace).
				Create(context.TODO(), event, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(500 * time.Millisecond)
			Expect(watcher.GetBuffer().Size()).To(Equal(1))
		})

		Context("but this is an update of an existing event", func() {
			BeforeEach(func() {
				_, err := clientset.CoreV1().
					Events(cluster.TargetNamespace).
					Create(context.TODO(), event, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)
			})

			It("should write the updated event to the buffer", func() {
				event.LastTimestamp = metav1.Now()
				event.ResourceVersion = "2"

				_, err := clientset.CoreV1().
					Events(cluster.TargetNamespace).
					Update(context.TODO(), event, metav1.UpdateOptions{})
				Expect(err).NotTo(HaveOccurred())

				time.Sleep(500 * time.Millisecond)
				Expect(watcher.GetBuffer().Size()).To(Equal(2))
			})

			It("should not write the event if the resource version is the same", func() {
				event.LastTimestamp = metav1.Now()

				_, err := clientset.CoreV1().
					Events(cluster.TargetNamespace).
					Update(context.TODO(), event, metav1.UpdateOptions{})
				Expect(err).NotTo(HaveOccurred())

				time.Sleep(500 * time.Millisecond)
				Expect(watcher.GetBuffer().Size()).To(Equal(1))
			})
		})
	})

})
