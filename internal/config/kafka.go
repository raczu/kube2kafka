package config

import (
	"crypto/tls"
	"fmt"
	"github.com/raczu/kube2kafka/pkg/exporter"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"os"
)

type RawTLSData struct {
	CA         string `yaml:"cacert"`
	Cert       string `yaml:"cert"`
	Key        string `yaml:"key"`
	SkipVerify bool   `yaml:"skipVerify"`
}

func (r *RawTLSData) Build() (*tls.Config, error) {
	var data exporter.TLSData

	if r.CA != "" {
		ca, err := os.ReadFile(r.CA)
		if err != nil {
			return nil, fmt.Errorf("error reading ca file: %w", err)
		}
		data.CA = ca
	}

	if r.Cert != "" && r.Key != "" {
		cert, err := os.ReadFile(r.Cert)
		if err != nil {
			return nil, fmt.Errorf("error reading client cert file: %w", err)
		}
		data.Cert = cert

		key, err := os.ReadFile(r.Key)
		if err != nil {
			return nil, fmt.Errorf("error reading client key file: %w", err)
		}
		data.Key = key
	}

	data.SkipVerify = r.SkipVerify
	return data.Build()
}

type RawSASLData struct {
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	Mechanism string `yaml:"mechanism" default:"plain"`
}

func (r *RawSASLData) Validate() error {
	if r.Username == "" {
		return fmt.Errorf("username is required")
	}

	if r.Password == "" {
		return fmt.Errorf("password is required")
	}
	return nil
}

func (r *RawSASLData) Build() (sasl.Mechanism, error) {
	factory, err := exporter.MapMechanismString(r.Mechanism)
	if err != nil {
		return nil, fmt.Errorf("failed to map mechanism string: %w", err)
	}
	return factory(r.Username, r.Password)
}

type KafkaConfig struct {
	Brokers        []string     `yaml:"brokers"`
	Topic          string       `yaml:"topic"`
	RawCompression string       `yaml:"compression" default:"none"`
	RawTLS         *RawTLSData  `yaml:"tls"`
	RawSASL        *RawSASLData `yaml:"sasl"`
}

func (c *KafkaConfig) Validate() error {
	if len(c.Brokers) == 0 {
		return fmt.Errorf("at least one broker is required")
	}

	if c.Topic == "" {
		return fmt.Errorf("topic is required")
	}

	if c.RawSASL != nil {
		if err := c.RawSASL.Validate(); err != nil {
			return fmt.Errorf("sasl config has issues: %w", err)
		}
	}
	return nil
}

func (c *KafkaConfig) TLS() (*tls.Config, error) {
	if c.RawTLS == nil {
		return nil, nil
	}
	return c.RawTLS.Build()
}

func (c *KafkaConfig) SASL() (sasl.Mechanism, error) {
	if c.RawSASL == nil {
		return nil, nil
	}
	return c.RawSASL.Build()
}

func (c *KafkaConfig) Compression() (kafka.Compression, error) {
	compression, err := exporter.MapCodecString(c.RawCompression)
	if err != nil {
		return kafka.Compression(0), fmt.Errorf("failed to map codec string: %w", err)
	}
	return compression, nil
}
