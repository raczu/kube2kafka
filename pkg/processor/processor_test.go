package processor_test

import (
	"bytes"
	"context"
	"encoding/json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raczu/kube2kafka/pkg/kube"
	"github.com/raczu/kube2kafka/pkg/kube/watcher"
	log "github.com/raczu/kube2kafka/pkg/logger"
	"github.com/raczu/kube2kafka/pkg/processor"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"reflect"
	"sync"
	"time"
)

var _ = Describe("Processor", func() {
	var (
		source *watcher.EventBuffer
		proc   *processor.Processor
		logger *zap.Logger
		event  *kube.EnhancedEvent
		ctx    context.Context
		cancel context.CancelFunc
		wg     sync.WaitGroup
		buffer *bytes.Buffer
	)

	BeforeEach(func() {
		source = watcher.NewEventBuffer(1)
		event = &kube.EnhancedEvent{
			Event: corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: corev1.NamespaceDefault,
					UID:       uuid.NewUUID(),
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Pod",
				},
				Reason:  "Created",
				Message: "Pod created successfully",
				Type:    "Normal",
				Source: corev1.EventSource{
					Component: "kubelet",
				},
			},
			ClusterName: "dev.kube2kafka.local",
		}

		buffer = &bytes.Buffer{}
		opts := log.Options{
			Output: buffer,
		}
		logger = log.New(log.UseOptions(&opts))
	})

	When("processing an event", func() {
		BeforeEach(func() {
			proc = processor.New(source, processor.WithLogger(logger))
			source.Write(event)
		})

		AfterEach(func() {
			wg.Wait()
		})

		Context("and there are some filters", func() {
			It("should filter out the event if no filter matches", func() {
				filters := []processor.Filter{
					{
						Kind: "ReplicaSet",
					},
				}
				proc = processor.New(
					source,
					processor.WithLogger(logger),
					processor.WithFilters(filters),
				)

				ctx, cancel = context.WithCancel(context.Background())
				defer cancel()

				wg.Add(1)
				go func() {
					defer wg.Done()
					proc.Process(ctx)
				}()

				Consistently(func() int {
					return proc.GetBuffer().Size()
				}, 1*time.Second, 100*time.Millisecond).Should(Equal(0))
			})

			It("should write the event to the output buffer if any filter matches", func() {
				filters := []processor.Filter{
					{
						Kind: "Pod",
					},
				}
				proc = processor.New(
					source,
					processor.WithLogger(logger),
					processor.WithFilters(filters),
				)

				ctx, cancel = context.WithCancel(context.Background())
				defer cancel()

				wg.Add(1)
				go func() {
					defer wg.Done()
					proc.Process(ctx)
				}()

				Eventually(func() int {
					return proc.GetBuffer().Size()
				}, 5*time.Second, 100*time.Millisecond).Should(Equal(1))
			})
		})

		Context("and there are some selectors", func() {
			It("should customize the event payload based on the selectors", func() {
				selectors := []processor.Selector{
					{
						Key:   "cluster",
						Value: "{{ .ClusterName }}",
					},
					{
						Key:   "kind",
						Value: "{{ .InvolvedObject.Kind }}",
					},
				}
				proc = processor.New(
					source,
					processor.WithLogger(logger),
					processor.WithSelectors(selectors),
				)

				ctx, cancel = context.WithCancel(context.Background())
				defer cancel()

				wg.Add(1)
				go func() {
					defer wg.Done()
					proc.Process(ctx)
				}()

				Eventually(func() int {
					return proc.GetBuffer().Size()
				}, 5*time.Second, 100*time.Millisecond).Should(Equal(1))

				msg, _ := proc.GetBuffer().Read()
				Expect(msg.Key).To(Equal([]byte(event.UID)))

				var payload map[string]string
				err := json.Unmarshal(msg.Value, &payload)
				Expect(err).NotTo(HaveOccurred())

				Expect(payload).To(HaveKeyWithValue("cluster", event.ClusterName))
				Expect(payload).To(HaveKeyWithValue("kind", event.InvolvedObject.Kind))
			})

			It("should log warning when any selection causes an error", func() {
				selectors := []processor.Selector{
					{
						Key:   "cluster",
						Value: "{{ .ClusterName }}",
					},
					{
						Key:   "dummy",
						Value: "{{ .NonExistingField }}",
					},
				}
				proc = processor.New(
					source,
					processor.WithLogger(logger),
					processor.WithSelectors(selectors),
				)

				ctx, cancel = context.WithCancel(context.Background())
				defer cancel()

				wg.Add(1)
				go func() {
					defer wg.Done()
					proc.Process(ctx)
				}()

				Eventually(func() int {
					return proc.GetBuffer().Size()
				}, 5*time.Second, 100*time.Millisecond).Should(Equal(1))

				Expect(
					buffer.String(),
				).To(ContainSubstring("error occurred while selecting fields from the event"))
			})

			It("should provide default payload when any selection causes an error", func() {
				selectors := []processor.Selector{
					{
						Key:   "cluster",
						Value: "{{ .ClusterName }}",
					},
					{
						Key:   "dummy",
						Value: "{{ .NonExistingField }}",
					},
				}
				proc = processor.New(
					source,
					processor.WithLogger(logger),
					processor.WithSelectors(selectors),
				)

				ctx, cancel = context.WithCancel(context.Background())
				defer cancel()

				wg.Add(1)
				go func() {
					defer wg.Done()
					proc.Process(ctx)
				}()

				Eventually(func() int {
					return proc.GetBuffer().Size()
				}, 5*time.Second, 100*time.Millisecond).Should(Equal(1))

				msg, _ := proc.GetBuffer().Read()
				Expect(msg.Key).To(Equal([]byte(event.UID)))

				var payload map[string]string
				err := json.Unmarshal(msg.Value, &payload)
				Expect(err).NotTo(HaveOccurred())

				isEqual := reflect.DeepEqual(payload, processor.RelevantFieldSelection(event))
				Expect(isEqual).To(BeTrue())
			})
		})

		Context("and selectors are not provided", func() {
			It("should write the event to the output buffer as is", func() {
				proc = processor.New(
					source,
					processor.WithLogger(logger),
				)

				ctx, cancel = context.WithCancel(context.Background())
				defer cancel()

				wg.Add(1)
				go func() {
					defer wg.Done()
					proc.Process(ctx)
				}()

				Eventually(func() int {
					return proc.GetBuffer().Size()
				}, 5*time.Second, 100*time.Millisecond).Should(Equal(1))

				msg, _ := proc.GetBuffer().Read()
				Expect(msg.Key).To(Equal([]byte(event.UID)))

				var processed kube.EnhancedEvent
				err := json.Unmarshal(msg.Value, &processed)
				Expect(err).NotTo(HaveOccurred())

				isEqual := reflect.DeepEqual(processed, *event)
				Expect(isEqual).To(BeTrue())
			})
		})
	})
})
