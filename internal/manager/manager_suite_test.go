package manager_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/raczu/kube2kafka/internal/kafkatest"
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
}, NodeTimeout(10*time.Second))
