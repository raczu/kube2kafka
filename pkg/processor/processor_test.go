package processor_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/raczu/kube2kafka/pkg/kube"
	"github.com/raczu/kube2kafka/pkg/kube/watcher"
	"github.com/raczu/kube2kafka/pkg/processor"
)

// processInBackground runs proc on its own context until the spec ends.
func processInBackground(proc *processor.Processor) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		proc.Process(ctx)
	}()

	DeferCleanup(func(cctx SpecContext) {
		cancel()
		select {
		case <-done:
		case <-cctx.Done():
		}
	}, NodeTimeout(cleanupTimeout))
}

var _ = Describe("Processor", func() {
	var (
		source *watcher.EventBuffer
		proc   *processor.Processor
		logger *zap.Logger
		event  *kube.EnhancedEvent
		logs   *observer.ObservedLogs
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

		var core zapcore.Core
		core, logs = observer.New(zap.DebugLevel)
		logger = zap.New(core)
	})

	outputSize := func() int {
		return proc.GetBuffer().Size()
	}

	When("processing an event", func() {
		BeforeEach(func() {
			source.Write(event)
		})

		Context("and there are some filters", func() {
			It("should filter out the event if no filter matches", func(ctx SpecContext) {
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

				processInBackground(proc)

				// Once the event left the source, the processor has made its decision.
				Eventually(source.Size).WithContext(ctx).Should(Equal(0))
				Consistently(outputSize, quietPeriod).Should(Equal(0))
			}, SpecTimeout(specTimeout))

			It(
				"should write the event to the output buffer if any filter matches",
				func(ctx SpecContext) {
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

					processInBackground(proc)

					Eventually(outputSize).WithContext(ctx).Should(Equal(1))
				},
				SpecTimeout(specTimeout),
			)
		})

		Context("and there are some selectors", func() {
			It("should customize the event payload based on the selectors", func(ctx SpecContext) {
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

				processInBackground(proc)

				Eventually(outputSize).WithContext(ctx).Should(Equal(1))

				msg, _ := proc.GetBuffer().Read()
				Expect(msg.Key).To(Equal([]byte(event.UID)))

				var payload map[string]string
				err := json.Unmarshal(msg.Value, &payload)
				Expect(err).NotTo(HaveOccurred())

				Expect(payload).To(HaveKeyWithValue("cluster", event.ClusterName))
				Expect(payload).To(HaveKeyWithValue("kind", event.InvolvedObject.Kind))
			}, SpecTimeout(specTimeout))

			It("should log warning when any selection causes an error", func(ctx SpecContext) {
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

				processInBackground(proc)

				Eventually(outputSize).WithContext(ctx).Should(Equal(1))

				warnings := logs.FilterLevelExact(zap.WarnLevel).
					FilterMessageSnippet("error occurred while selecting fields from the event")
				Expect(warnings.Len()).To(Equal(1))
			}, SpecTimeout(specTimeout))

			It(
				"should provide default payload when any selection causes an error",
				func(ctx SpecContext) {
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

					processInBackground(proc)

					Eventually(outputSize).WithContext(ctx).Should(Equal(1))

					msg, _ := proc.GetBuffer().Read()
					Expect(msg.Key).To(Equal([]byte(event.UID)))

					var payload map[string]string
					err := json.Unmarshal(msg.Value, &payload)
					Expect(err).NotTo(HaveOccurred())

					Expect(payload).To(Equal(processor.RelevantFieldSelection(event)))
				},
				SpecTimeout(specTimeout),
			)
		})

		Context("and selectors are not provided", func() {
			It("should write the event to the output buffer as is", func(ctx SpecContext) {
				proc = processor.New(
					source,
					processor.WithLogger(logger),
				)

				processInBackground(proc)

				Eventually(outputSize).WithContext(ctx).Should(Equal(1))

				msg, _ := proc.GetBuffer().Read()
				Expect(msg.Key).To(Equal([]byte(event.UID)))

				var processed kube.EnhancedEvent
				err := json.Unmarshal(msg.Value, &processed)
				Expect(err).NotTo(HaveOccurred())

				Expect(processed).To(Equal(*event))
			}, SpecTimeout(specTimeout))
		})
	})
})
