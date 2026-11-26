package config_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/raczu/kube2kafka/internal/config"
	"github.com/raczu/kube2kafka/internal/kafkatest"
)

var _ = Describe("RawTLSData", func() {
	var (
		data *config.RawTLSData
	)

	BeforeEach(func() {
		certs, err := kafkatest.GenerateCerts()
		Expect(err).NotTo(HaveOccurred())

		dir := GinkgoT().TempDir()
		data = &config.RawTLSData{
			CA:   filepath.Join(dir, "ca.crt"),
			Cert: filepath.Join(dir, "client.crt"),
			Key:  filepath.Join(dir, "client.key"),
		}
		Expect(os.WriteFile(data.CA, certs.CA, 0o600)).To(Succeed())
		Expect(os.WriteFile(data.Cert, certs.ClientCert, 0o600)).To(Succeed())
		Expect(os.WriteFile(data.Key, certs.ClientKey, 0o600)).To(Succeed())
	})

	When("building a TLS config from files", func() {
		It("should load the CA and the client certificate", func() {
			tlsConfig, err := data.Build()
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsConfig.RootCAs).NotTo(BeNil())
			Expect(tlsConfig.Certificates).To(HaveLen(1))
		})

		It("should return an error if the CA file is missing", func() {
			data.CA = filepath.Join(GinkgoT().TempDir(), "missing.crt")
			_, err := data.Build()
			Expect(err).To(MatchError(ContainSubstring("error reading ca file")))
		})

		It("should return an error if the client cert file is missing", func() {
			data.Cert = filepath.Join(GinkgoT().TempDir(), "missing.crt")
			_, err := data.Build()
			Expect(err).To(MatchError(ContainSubstring("error reading client cert file")))
		})

		It("should return an error if the client key file is missing", func() {
			data.Key = filepath.Join(GinkgoT().TempDir(), "missing.key")
			_, err := data.Build()
			Expect(err).To(MatchError(ContainSubstring("error reading client key file")))
		})
	})
})

var _ = Describe("RawSASLData", func() {
	When("validating", func() {
		It("should accept a username and a password", func() {
			data := &config.RawSASLData{Username: "user", Password: "pwd"}
			Expect(data.Validate()).To(Succeed())
		})

		It("should return an error if the username is missing", func() {
			data := &config.RawSASLData{Password: "pwd"}
			Expect(data.Validate()).To(MatchError("username is required"))
		})

		It("should return an error if the password is missing", func() {
			data := &config.RawSASLData{Username: "user"}
			Expect(data.Validate()).To(MatchError("password is required"))
		})
	})
})

var _ = Describe("KafkaConfig", func() {
	When("validating the TLS client certificate", func() {
		var kc *config.KafkaConfig

		BeforeEach(func() {
			kc = &config.KafkaConfig{
				Brokers: []string{"localhost:9092"},
				Topic:   "t",
				RawTLS:  &config.RawTLSData{},
			}
		})

		It("should return an error if only the cert is set", func() {
			kc.RawTLS.Cert = "client.crt"
			Expect(kc.Validate()).To(MatchError("tls: cert is set but key is missing"))
		})

		It("should return an error if only the key is set", func() {
			kc.RawTLS.Key = "client.key"
			Expect(kc.Validate()).To(MatchError("tls: key is set but cert is missing"))
		})

		It("should accept both the cert and the key", func() {
			kc.RawTLS.Cert = "client.crt"
			kc.RawTLS.Key = "client.key"
			Expect(kc.Validate()).To(Succeed())
		})

		It("should accept neither the cert nor the key", func() {
			Expect(kc.Validate()).To(Succeed())
		})
	})

	When("getting the SASL mechanism", func() {
		DescribeTable("should return the configured mechanism",
			func(mechanism, name string) {
				kc := &config.KafkaConfig{RawSASL: &config.RawSASLData{
					Username:  "user",
					Password:  "pwd",
					Mechanism: mechanism,
				}}
				m, err := kc.GetSASL()
				Expect(err).NotTo(HaveOccurred())
				Expect(m.Name()).To(Equal(name))
			},
			Entry("plain", "plain", "PLAIN"),
			Entry("sha256", "sha256", "SCRAM-SHA-256"),
			Entry("sha512", "sha512", "SCRAM-SHA-512"),
		)

		It("should return an error for an unknown mechanism", func() {
			kc := &config.KafkaConfig{RawSASL: &config.RawSASLData{
				Username:  "user",
				Password:  "pwd",
				Mechanism: "SCRAM-SHA-256",
			}}
			_, err := kc.GetSASL()
			Expect(err).To(MatchError(ContainSubstring("unknown mechanism: SCRAM-SHA-256")))
		})
	})
})
