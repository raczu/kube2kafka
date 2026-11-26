package exporter

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/segmentio/kafka-go"
)

var _ = Describe("isFatalError", func() {
	certErr := &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}
	refusedErr := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}

	DescribeTable("should classify the error",
		func(err error, fatal bool) {
			Expect(isFatalError(err)).To(Equal(fatal))
		},
		Entry("broker cert rejected", certErr, true),
		Entry("broker cert rejected, wrapped", fmt.Errorf("dial: %w", certErr), true),
		Entry("SASL authentication failed", kafka.SASLAuthenticationFailed, true),
		Entry("connection refused", refusedErr, false),
		Entry("deadline exceeded", context.DeadlineExceeded, false),
	)
})
