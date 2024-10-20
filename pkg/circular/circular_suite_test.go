package circular_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCircular(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Circular Suite")
}
