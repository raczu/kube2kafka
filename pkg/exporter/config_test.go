package exporter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raczu/kube2kafka/pkg/exporter"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"
	"github.com/segmentio/kafka-go/sasl/plain"
)

var _ = Describe("MapCodecString", func() {
	When("mapping a known codec string", func() {
		It("should return the corresponding kafka.Compression", func() {
			codecs := []string{"none", "gzip", "snappy", "lz4", "zstd"}
			expected := []kafka.Compression{
				compress.None,
				kafka.Gzip,
				kafka.Snappy,
				kafka.Lz4,
				kafka.Zstd,
			}

			for i, codec := range codecs {
				compression, err := exporter.MapCodecString(codec)
				Expect(err).NotTo(HaveOccurred())
				Expect(compression).To(Equal(expected[i]))
			}
		})
	})

	When("mapping an unknown codec string", func() {
		It("should return an error", func() {
			compression, err := exporter.MapCodecString("unknown")
			Expect(err).To(HaveOccurred())
			Expect(compression).To(Equal(kafka.Compression(0)))
		})
	})
})

var _ = Describe("MapMechanismString", func() {
	When("mapping a known mechanism string", func() {
		It("should return plain mechanism factory", func() {
			factory, err := exporter.MapMechanismString("plain")
			Expect(err).NotTo(HaveOccurred())

			mechanism, err := factory("username", "password")
			Expect(err).NotTo(HaveOccurred())
			Expect(mechanism).To(BeAssignableToTypeOf(plain.Mechanism{}))
		})

		It("should return scram sha256 mechanism factory", func() {
			factory, err := exporter.MapMechanismString("sha256")
			Expect(err).NotTo(HaveOccurred())

			mechanism, err := factory("username", "password")
			Expect(err).NotTo(HaveOccurred())
			Expect(mechanism.Name()).To(Equal("SCRAM-SHA-256"))
		})

		It("should return scram sha512 mechanism factory", func() {
			factory, err := exporter.MapMechanismString("sha512")
			Expect(err).NotTo(HaveOccurred())

			mechanism, err := factory("username", "password")
			Expect(err).NotTo(HaveOccurred())
			Expect(mechanism.Name()).To(Equal("SCRAM-SHA-512"))
		})
	})

	When("mapping an unknown mechanism string", func() {
		It("should return an error", func() {
			mechanism, err := exporter.MapMechanismString("unknown")
			Expect(err).To(HaveOccurred())
			Expect(mechanism).To(BeNil())
		})
	})
})
