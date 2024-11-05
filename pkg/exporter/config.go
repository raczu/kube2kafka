package exporter

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

type TLSData struct {
	CA         []byte
	Cert       []byte
	Key        []byte
	SkipVerify bool
}

// Build creates a tls.Config from the TLSData.
func (t *TLSData) Build() (*tls.Config, error) {
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if t.CA != nil {
		config.RootCAs = x509.NewCertPool()
		if !config.RootCAs.AppendCertsFromPEM(t.CA) {
			return nil, fmt.Errorf("failed to parse ca certificate")
		}
	}

	if t.Cert != nil && t.Key != nil {
		cert, err := tls.X509KeyPair(t.Cert, t.Key)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to parse client certificate and its private key: %w",
				err,
			)
		}
		config.Certificates = []tls.Certificate{cert}
	}

	if t.SkipVerify {
		config.InsecureSkipVerify = true
	}
	return config, nil
}

var codec2compression = map[string]kafka.Compression{
	"none":   compress.None,
	"gzip":   kafka.Gzip,
	"snappy": kafka.Snappy,
	"lz4":    kafka.Lz4,
	"zstd":   kafka.Zstd,
}

// MapCodecString maps a codec string to a kafka.Compression.
func MapCodecString(codec string) (kafka.Compression, error) {
	if c, ok := codec2compression[codec]; ok {
		return c, nil
	}
	return 0, fmt.Errorf("unknown codec: %s", codec)
}

// MechanismFactory is a function that takes a username and a password
// and returns a sasl.Mechanism and potentially an error.
type MechanismFactory func(username, password string) (sasl.Mechanism, error)

var mechanism2factory = map[string]MechanismFactory{
	"plain": func(username, password string) (sasl.Mechanism, error) {
		return plain.Mechanism{Username: username, Password: password}, nil
	},
	"sha256": func(username, password string) (sasl.Mechanism, error) {
		return scram.Mechanism(scram.SHA256, username, password)
	},
	"sha512": func(username, password string) (sasl.Mechanism, error) {
		return scram.Mechanism(scram.SHA512, username, password)
	},
}

// MapMechanismString maps a mechanism string to a MechanismFactory.
func MapMechanismString(mechanism string) (MechanismFactory, error) {
	if h, ok := mechanism2factory[mechanism]; ok {
		return h, nil
	}
	return nil, fmt.Errorf("unknown mechanism: %s", mechanism)
}
