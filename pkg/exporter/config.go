package exporter

import (
	"fmt"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

var codec2compression = map[string]kafka.Compression{
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
