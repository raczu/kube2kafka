package kube

import (
	corev1 "k8s.io/api/core/v1"
	"time"
)

// EnhancedEvent is an event enriched with the cluster name and option to
// get the first occurrence timestamp in RFC3339 format. The enrichment
// with the cluster name allows to distinguish events from different clusters.
type EnhancedEvent struct {
	corev1.Event `json:",inline"`
	ClusterName  string `json:"clusterName"`
}

func (ev *EnhancedEvent) FirstOccurrence() time.Time {
	timestamp := ev.EventTime.Time
	if timestamp.IsZero() {
		timestamp = ev.FirstTimestamp.Time
	}
	return timestamp
}

func (ev *EnhancedEvent) GetRFC3339Timestamp() string {
	return ev.FirstOccurrence().Format(time.RFC3339)
}
