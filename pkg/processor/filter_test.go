package processor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raczu/kube2kafka/pkg/kube"
	"github.com/raczu/kube2kafka/pkg/processor"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Filter", func() {
	var (
		filter processor.Filter
		event  *kube.EnhancedEvent
	)

	When("validating a filter", func() {
		It("should validate only non-empty fields", func() {
			filter = processor.Filter{
				Kind: "Pod",
			}
			err := filter.Validate()

			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error if any of the fields is not a valid regular expression", func() {
			filter = processor.Filter{
				Kind:      "[abc",
				Namespace: "def*+",
			}
			err := filter.Validate()

			Expect(err).To(HaveOccurred())
		})

		It("should return error if expression is too complex", func() {
			filter = processor.Filter{
				Kind: "[a-z]{1,1500}",
			}
			err := filter.Validate()

			Expect(err).To(HaveOccurred())
		})

		It("should not return an error if all fields are valid regular expressions", func() {
			filter = processor.Filter{
				Kind:      "^Pod$",
				Namespace: "^(default)?$",
				Reason:    "(?i)created",
				Message:   ".*",
				Type:      "Normal|Warning",
				Component: "^kubelet$",
			}
			err := filter.Validate()

			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("matching an event", func() {
		BeforeEach(func() {
			event = &kube.EnhancedEvent{
				Event: corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: corev1.NamespaceDefault,
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
			}
		})

		It("should not compare empty fields", func() {
			filter = processor.Filter{}
			matched := filter.MatchEvent(event)

			Expect(matched).To(BeTrue())
		})

		Context("and filter fields do not use regular expression syntax", func() {
			It("should match if event field contain filter value", func() {
				filter = processor.Filter{
					Message: "created",
				}
				matched := filter.MatchEvent(event)

				Expect(matched).To(BeTrue())
			})

			It("should not match if event field does not contain filter value", func() {
				filter = processor.Filter{
					Message: "deleted",
				}
				matched := filter.MatchEvent(event)

				Expect(matched).To(BeFalse())
			})

			It("should respect case sensitivity", func() {
				filter = processor.Filter{
					Reason: "created",
				}
				matched := filter.MatchEvent(event)

				Expect(matched).To(BeFalse())
			})

			It("should match if all fields match", func() {
				filter = processor.Filter{
					Kind:      "Pod",
					Namespace: "default",
					Reason:    "Created",
					Message:   "Pod created successfully",
					Type:      "Normal",
					Component: "kubelet",
				}
				matched := filter.MatchEvent(event)

				Expect(matched).To(BeTrue())
			})

			It("should not match if any field does not match", func() {
				filter = processor.Filter{
					Kind:      "Pod",
					Namespace: "default",
					Reason:    "Created",
					Message:   "Pod created successfully but with errors",
					Type:      "Normal",
					Component: "kubelet",
				}
				matched := filter.MatchEvent(event)

				Expect(matched).To(BeFalse())
			})
		})

		Context("and filter fields uses regular expression syntax", func() {
			It("should match if defined exact match", func() {
				filter = processor.Filter{
					Kind: "^Pod$",
				}
				matched := filter.MatchEvent(event)

				Expect(matched).To(BeTrue())
			})

			It("should match if some fields partially match", func() {
				filter = processor.Filter{
					Kind:    "^[pP]",
					Reason:  "(?i)created",
					Message: "^[pP]",
				}
				matched := filter.MatchEvent(event)

				Expect(matched).To(BeTrue())
			})

			It("should match if all fields match", func() {
				filter = processor.Filter{
					Kind:      "^Pod$",
					Namespace: "^(default)?$",
					Reason:    "(?i)created",
					Message:   ".*",
					Type:      "Normal|Warning",
					Component: "^kubelet$",
				}
				matched := filter.MatchEvent(event)

				Expect(matched).To(BeTrue())
			})

			It("should not match if any field does not match", func() {
				filter = processor.Filter{
					Kind:      "^Pod$",
					Namespace: "^(default)?$",
					Reason:    "(?i)deleted",
					Message:   ".*",
					Type:      "Normal|Warning",
					Component: "^kubelet$",
				}
				matched := filter.MatchEvent(event)

				Expect(matched).To(BeFalse())
			})
		})
	})
})
