package manager_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/raczu/kube2kafka/internal/kafkatest"
)

const (
	// specTimeout is the budget for a spec or a setup node that talks to Kafka.
	specTimeout = 30 * time.Second
	// cleanupTimeout is the budget for stopping a component running in the background.
	cleanupTimeout = 10 * time.Second
	// readTimeout bounds one read from Kafka, so a slow broker only means more polls.
	readTimeout = time.Second
)

func TestManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Manager Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	// The suite also has unit specs, so BeforeSuite runs under "make test-unit" too.
	// Only demand a broker when the label filter selects integration specs.
	if !Label("integration").MatchesLabelFilter(GinkgoLabelFilter()) {
		return
	}
	_, err := kafkatest.Broker(ctx)
	Expect(err).NotTo(HaveOccurred())
}, NodeTimeout(specTimeout))
