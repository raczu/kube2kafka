package e2e_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("kube2kafka", Label("e2e"), Ordered, func() {
	It("should skip events that do not match the filter", func(ctx SpecContext) {
		skipped := newID()
		createEvent(ctx, config.TargetNamespace, "E2EMiss", skipped, time.Now())
		sentinel := newID()
		createEvent(ctx, config.TargetNamespace, matchReason, sentinel, time.Now())

		expectSkipped(ctx, sentinel, skipped)
	}, SpecTimeout(specTimeout))

	It("should export exactly the configured selectors", func(ctx SpecContext) {
		id := newID()
		createEvent(ctx, config.TargetNamespace, matchReason, id, time.Now())

		got, _ := readUntil(ctx, id)
		Expect(got).To(Equal(payload{
			"cluster":   config.ClusterName,
			"kind":      "Pod",
			"name":      id,
			"namespace": config.TargetNamespace,
			"reason":    matchReason,
			"message":   id,
			"count":     "1",
		}))
	}, SpecTimeout(specTimeout))

	It("should export an event again when it is updated", func(ctx SpecContext) {
		id := newID()
		event := createEvent(ctx, config.TargetNamespace, matchReason, id, time.Now())
		got, _ := readUntil(ctx, id)
		Expect(got).To(HaveKeyWithValue("count", "1"))

		event.Count = 2
		event.LastTimestamp = metav1.Now()
		_, err := clientset.CoreV1().Events(config.TargetNamespace).
			Update(ctx, event, metav1.UpdateOptions{})
		Expect(err).NotTo(HaveOccurred())

		got, _ = readUntil(ctx, id)
		Expect(got).To(HaveKeyWithValue("count", "2"))
	}, SpecTimeout(specTimeout))

	It("should skip events outside the target namespace", func(ctx SpecContext) {
		skipped := newID()
		createEvent(ctx, otherNamespace, matchReason, skipped, time.Now())
		sentinel := newID()
		createEvent(ctx, config.TargetNamespace, matchReason, sentinel, time.Now())

		expectSkipped(ctx, sentinel, skipped)
	}, SpecTimeout(specTimeout))

	// maxEventAge only applies to events listed at startup, so the old event is created
	// while kube2kafka is stopped.
	It("should skip events older than maxEventAge at startup", func(ctx SpecContext) {
		scaleKube2kafka(ctx, 0)
		skipped := newID()
		createEvent(ctx, config.TargetNamespace, matchReason, skipped,
			time.Now().Add(-2*config.MaxEventAge))
		scaleKube2kafka(ctx, 1)
		waitForSyncedPod(ctx)

		sentinel := newID()
		createEvent(ctx, config.TargetNamespace, matchReason, sentinel, time.Now())

		expectSkipped(ctx, sentinel, skipped)
	}, SpecTimeout(rolloutTimeout))
})
