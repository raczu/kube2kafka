package exporter_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"
	"github.com/segmentio/kafka-go/sasl/plain"

	"github.com/raczu/kube2kafka/internal/kafkatest"
	"github.com/raczu/kube2kafka/pkg/exporter"
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

var _ = Describe("TLSData", func() {
	var (
		certs *kafkatest.Certs
	)

	BeforeEach(func() {
		var err error
		certs, err = kafkatest.GenerateCerts()
		Expect(err).NotTo(HaveOccurred())
	})

	When("building a TLS config", func() {
		It("should verify the server with system roots by default", func() {
			config, err := (&exporter.TLSData{}).Build()
			Expect(err).NotTo(HaveOccurred())
			Expect(config.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
			Expect(config.RootCAs).To(BeNil())
			Expect(config.Certificates).To(BeEmpty())
			Expect(config.InsecureSkipVerify).To(BeFalse())
		})

		It("should trust the given CA", func() {
			config, err := (&exporter.TLSData{CA: certs.CA}).Build()
			Expect(err).NotTo(HaveOccurred())

			block, _ := pem.Decode(certs.CA)
			ca, err := x509.ParseCertificate(block.Bytes)
			Expect(err).NotTo(HaveOccurred())
			expected := x509.NewCertPool()
			expected.AddCert(ca)
			Expect(config.RootCAs.Equal(expected)).To(BeTrue())
		})

		It("should return an error if the CA is not valid PEM", func() {
			_, err := (&exporter.TLSData{CA: []byte("not a certificate")}).Build()
			Expect(err).To(MatchError(ContainSubstring("failed to parse ca certificate")))
		})

		It("should present the given client certificate", func() {
			data := &exporter.TLSData{Cert: certs.ClientCert, Key: certs.ClientKey}
			config, err := data.Build()
			Expect(err).NotTo(HaveOccurred())
			Expect(config.Certificates).To(HaveLen(1))

			leaf, err := x509.ParseCertificate(config.Certificates[0].Certificate[0])
			Expect(err).NotTo(HaveOccurred())
			Expect(leaf.Subject.CommonName).To(Equal(kafkatest.ClientCommonName))
		})

		It("should return an error if the client key does not match the certificate", func() {
			data := &exporter.TLSData{Cert: certs.ClientCert, Key: certs.ServerKey}
			_, err := data.Build()
			Expect(err).To(MatchError(ContainSubstring(
				"failed to parse client certificate and its private key",
			)))
		})

		It("should skip server verification if requested", func() {
			config, err := (&exporter.TLSData{SkipVerify: true}).Build()
			Expect(err).NotTo(HaveOccurred())
			Expect(config.InsecureSkipVerify).To(BeTrue())
		})
	})
})
