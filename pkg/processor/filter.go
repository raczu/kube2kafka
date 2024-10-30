package processor

import (
	"github.com/raczu/kube2kafka/pkg/assert"
	"github.com/raczu/kube2kafka/pkg/kube"
	"regexp"
)

// Filter is used to filter events based on their attributes. All fields in the filter
// are optional and are treated as regular expressions. If a field is empty, it is ignored.
type Filter struct {
	// Kind matches the kind of the involved object.
	Kind string `yaml:"kind"`
	// Namespace matches the namespace of the event.
	Namespace string `yaml:"namespace"`
	// Reason matches the reason of the event.
	Reason string `yaml:"reason"`
	// Message matches the human-readable message of the event.
	Message string `yaml:"message"`
	// Type matches the type of the event.
	Type string `yaml:"type"`
	// Component matches the component of the event source.
	Component string `yaml:"component"`
}

// Validate checks whether all non-empty fields of the filter are valid regular expressions.
func (f *Filter) Validate() error {
	patterns := []string{f.Kind, f.Namespace, f.Reason, f.Message, f.Type, f.Component}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}

		if _, err := regexp.Compile(pattern); err != nil {
			return err
		}
	}
	return nil
}

// MatchEvent checks if the provided event matches the filter. This means that the event
// must match all the non-empty fields of the filter.
func (f *Filter) MatchEvent(event *kube.EnhancedEvent) bool {
	pairs := map[string]string{
		f.Kind:      event.InvolvedObject.Kind,
		f.Namespace: event.Namespace,
		f.Reason:    event.Reason,
		f.Message:   event.Message,
		f.Type:      event.Type,
		f.Component: event.Source.Component,
	}

	for pattern, value := range pairs {
		if pattern == "" {
			continue
		}

		matched, err := regexp.MatchString(pattern, value)
		assert.NoError(err, "filter regexes should be pre-validated before use")
		if !matched {
			return false
		}
	}
	return true
}

// AnyFilterMatches returns true if any of the provided filters matches the event.
func AnyFilterMatches(filters []Filter, event *kube.EnhancedEvent) bool {
	for _, filter := range filters {
		if filter.MatchEvent(event) {
			return true
		}
	}
	return false
}
