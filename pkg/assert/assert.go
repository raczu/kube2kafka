// Package assert provides a simple assertion mechanism for logging fatal
// error when a condition is not met. It is primarily used for development
// purposes to ensure that the code behaves as expected.
package assert

import log "github.com/raczu/kube2kafka/pkg/logger"

var logger = log.New(log.WithDevelopment())

// Assert logs a fatal message if the condition is false.
func Assert(condition bool, message string) {
	if !condition {
		logger.Fatal(message)
	}
}
