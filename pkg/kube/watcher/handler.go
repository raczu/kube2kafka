package watcher

import (
	"github.com/raczu/kube2kafka/pkg/kube"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"time"
)

func timeSinceOccurrence(event *corev1.Event) time.Duration {
	timestamp := event.LastTimestamp.Time
	if timestamp.IsZero() {
		timestamp = event.EventTime.Time
	}
	return time.Since(timestamp)
}

type EventHandler struct {
	output      *EventBuffer
	clusterName string
	maxEventAge time.Duration
	logger      *zap.Logger
}

func (eh *EventHandler) WriteToBuffer(event *corev1.Event) {
	ev := &kube.EnhancedEvent{
		Event:       *event.DeepCopy(),
		ClusterName: eh.clusterName,
	}

	eh.logger.Info(
		"received event",
		zap.String("namespace", event.Namespace),
		zap.String("name", event.Name),
		zap.String("reason", event.Reason),
		zap.String("regarding", event.InvolvedObject.Name),
	)
	eh.output.Write(ev)
}

func (eh *EventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	event := obj.(*corev1.Event)

	since := timeSinceOccurrence(event)
	if isInInitialList && since > eh.maxEventAge {
		eh.logger.Debug(
			"event does not meet the age criteria",
			zap.String("namespace", event.Namespace),
			zap.String("name", event.Name),
			zap.Duration("age", since),
		)
		return
	}
	eh.WriteToBuffer(event)
}

func (eh *EventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldEvent := oldObj.(*corev1.Event)
	newEvent := newObj.(*corev1.Event)
	if oldEvent.ResourceVersion != newEvent.ResourceVersion {
		eh.WriteToBuffer(newEvent)
	}
}

func (eh *EventHandler) OnDelete(_ interface{}) {
	// Do nothing as we are only interested in new or updated events.
}
