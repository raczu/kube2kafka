package processor

import (
	"bytes"
	"fmt"
	"github.com/raczu/kube2kafka/pkg/assert"
	"github.com/raczu/kube2kafka/pkg/kube"
	"text/template"
)

// Selector is used to select a specific field from the event.
// The key is the name under which the value is stored in the payload struct,
// whereas the value is a template string that is used to extract the desired field.
type Selector struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

// Validate checks whether the selector is valid. This means that key must not be empty
// and the value must be a valid template string.
func (s *Selector) Validate() error {
	if s.Key == "" {
		return fmt.Errorf("key must not be empty")
	}
	if s.Value == "" {
		return fmt.Errorf("value must not be empty")
	}

	if _, err := template.New(s.Key).Parse(s.Value); err != nil {
		return fmt.Errorf("field selector template is not valid: %w", err)
	}
	return nil
}

type FieldSelectionFallback func(event *kube.EnhancedEvent, err error) map[string]string

// PayloadCustomizer is used to customize the final event payload by selecting specific fields
// from the event and storing them under a specific key in the payload struct.
type PayloadCustomizer struct {
	Selectors []Selector
	// OnSelectionError is a function that is called when an error occurs during the
	// selection of a field. As some fields in kube.EnhancedEvent are pointers, it is
	// possible that the field is nil, which would cause an error during the selection.
	// This function returns a map of key-value pairs that represents the final event payload.
	OnSelectionError FieldSelectionFallback
}

// Customize selects the fields from the event based on the selectors and return a map of
// key-value pairs that represents the final event payload.
func (pc *PayloadCustomizer) Customize(event *kube.EnhancedEvent) map[string]string {
	payload := make(map[string]string)
	for _, selector := range pc.Selectors {
		tmpl, err := template.New(selector.Key).Parse(selector.Value)
		assert.NoError(err, "selector should be pre-validated before use")

		buff := &bytes.Buffer{}
		if err = tmpl.Execute(buff, event); err != nil {
			return pc.OnSelectionError(event, err)
		}
		payload[selector.Key] = buff.String()
	}
	return payload
}

// RelevantFieldSelection returns a map of key-value pairs containing the relevant
// fields from the event.
func RelevantFieldSelection(event *kube.EnhancedEvent) map[string]string {
	return map[string]string{
		"cluster":   event.ClusterName,
		"kind":      event.InvolvedObject.Kind,
		"namespace": event.Namespace,
		"reason":    event.Reason,
		"message":   event.Message,
		"type":      event.Type,
		"component": event.Source.Component,
		"occurred":  event.GetRFC3339Timestamp(),
		"count":     fmt.Sprintf("%d", event.Count),
	}
}
