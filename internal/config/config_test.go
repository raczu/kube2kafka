package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"

	"github.com/raczu/kube2kafka/internal/config"
)

var _ = Describe("Config", func() {
	When("setting defaults", func() {
		It("should default compression to none", func() {
			cfg := &config.Config{Kafka: &config.KafkaConfig{}}
			cfg.SetDefaults()
			Expect(cfg.Kafka.RawCompression).To(Equal("none"))
		})

		It("should default the SASL mechanism to plain", func() {
			cfg := &config.Config{Kafka: &config.KafkaConfig{RawSASL: &config.RawSASLData{}}}
			cfg.SetDefaults()
			Expect(cfg.Kafka.RawSASL.Mechanism).To(Equal("plain"))
		})

		It("should keep explicit values", func() {
			cfg := &config.Config{Kafka: &config.KafkaConfig{
				RawCompression: "gzip",
				RawSASL:        &config.RawSASLData{Mechanism: "sha512"},
			}}
			cfg.SetDefaults()
			Expect(cfg.Kafka.RawCompression).To(Equal("gzip"))
			Expect(cfg.Kafka.RawSASL.Mechanism).To(Equal("sha512"))
		})

		It("should not enable SASL or TLS", func() {
			cfg := &config.Config{Kafka: &config.KafkaConfig{}}
			cfg.SetDefaults()
			Expect(cfg.Kafka.RawSASL).To(BeNil())
			Expect(cfg.Kafka.RawTLS).To(BeNil())
		})

		It("should not fail without a kafka config", func() {
			cfg := &config.Config{}
			Expect(cfg.SetDefaults).NotTo(Panic())
		})

		It("should make the minimal documented config usable", func() {
			cfg := &config.Config{
				ClusterName: "c",
				Kafka: &config.KafkaConfig{
					Brokers: []string{"localhost:9092"},
					Topic:   "t",
					RawSASL: &config.RawSASLData{Username: "u", Password: "p"},
				},
			}
			cfg.SetDefaults()
			Expect(cfg.Validate()).To(Succeed())

			compression, err := cfg.Kafka.GetCompression()
			Expect(err).NotTo(HaveOccurred())
			Expect(compression).To(Equal(kafka.Compression(compress.None)))

			mechanism, err := cfg.Kafka.GetSASL()
			Expect(err).NotTo(HaveOccurred())
			Expect(mechanism.Name()).To(Equal("PLAIN"))
		})
	})
})
