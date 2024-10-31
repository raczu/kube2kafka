package processor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raczu/kube2kafka/pkg/kube"
	"github.com/raczu/kube2kafka/pkg/processor"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Selector", func() {
	var (
		selector processor.Selector
	)

	When("validating a selector", func() {
		It("should return an error if key is empty", func() {
			selector = processor.Selector{
				Key:   "",
				Value: "value",
			}
			err := selector.Validate()

			Expect(err).To(HaveOccurred())
		})

		It("should return an error if value is empty", func() {
			selector = processor.Selector{
				Key:   "key",
				Value: "",
			}
			err := selector.Validate()

			Expect(err).To(HaveOccurred())
		})

		It("should return an error if value is not a valid template string", func() {
			selector = processor.Selector{
				Key:   "key",
				Value: "{{",
			}
			err := selector.Validate()

			Expect(err).To(HaveOccurred())
		})

		It("should not return an error if is properly defined", func() {
			selector = processor.Selector{
				Key:   "key",
				Value: "{{ .Field }}",
			}
			err := selector.Validate()

			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("PayloadCustomizer", func() {
	var (
		selectors        []processor.Selector
		customizer       processor.PayloadCustomizer
		event            *kube.EnhancedEvent
		customizationErr error
	)

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
			ClusterName: "dev.kube2kafka.local",
		}
	})

	When("customizing the payload", func() {
		Context("and selector could get wanted fields", func() {
			It("should customize payload based on selectors", func() {
				selectors = []processor.Selector{
					{
						Key:   "cluster",
						Value: "{{ .ClusterName }}",
					},
					{
						Key:   "kind",
						Value: "{{ .InvolvedObject.Kind }}",
					},
				}
				customizer = processor.PayloadCustomizer{
					Selectors: selectors,
				}

				payload := customizer.Customize(event)
				Expect(payload).To(HaveKeyWithValue("cluster", event.ClusterName))
				Expect(payload).To(HaveKeyWithValue("kind", event.InvolvedObject.Kind))
			})
		})

		Context("and selectors could not get wanted fields", func() {
			BeforeEach(func() {
				fallback := func(event *kube.EnhancedEvent, err error) map[string]string {
					customizationErr = err
					return map[string]string{"dummy": "foo"}
				}
				customizer = processor.PayloadCustomizer{
					OnSelectionError: fallback,
				}
			})

			AfterEach(func() {
				customizationErr = nil
			})

			It("should use fallback function when any selection causes an error", func() {
				selectors = []processor.Selector{
					{
						Key:   "cluster",
						Value: "{{ .ClusterName }}",
					},
					{
						Key:   "dummy",
						Value: "{{ .NonExistingField }}",
					},
				}
				customizer.Selectors = selectors

				_ = customizer.Customize(event)
				Expect(customizationErr).NotTo(BeNil())
			})

			It("should use fallback function when accessing non-existing field", func() {
				selectors = []processor.Selector{
					{
						Key:   "dummy",
						Value: "{{ .NonExistingField }}",
					},
				}
				customizer.Selectors = selectors

				_ = customizer.Customize(event)
				Expect(customizationErr).NotTo(BeNil())
				Expect(
					customizationErr.Error(),
				).To(ContainSubstring("can't evaluate field NonExistingField"))
			})

			It("should use fallback function when evaluating nil pointer", func() {
				selectors = []processor.Selector{
					{
						Key:   "kind",
						Value: "{{ .Related.Kind }}",
					},
				}
				customizer.Selectors = selectors

				_ = customizer.Customize(event)
				Expect(customizationErr).NotTo(BeNil())
				Expect(customizationErr.Error()).To(ContainSubstring("nil pointer evaluating"))
			})
		})
	})
})
