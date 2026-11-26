package processor_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	specTimeout    = 10 * time.Second
	cleanupTimeout = 10 * time.Second
	// quietPeriod is how long a spec checks that nothing is written to a buffer.
	quietPeriod = 500 * time.Millisecond
)

func TestProcessor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Processor Suite")
}
