package assert

// Assert panics if the condition is false.
func Assert(condition bool, message string) {
	if !condition {
		panic(message)
	}
}
