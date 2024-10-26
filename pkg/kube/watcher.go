package kube

import (
	"context"
	"fmt"
	"github.com/raczu/kube2kafka/pkg/circular"
	log "github.com/raczu/kube2kafka/pkg/logger"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"time"
)

const (
	NoResyncPeriod        = 0 * time.Second
	DefaultMaxEventAge    = 5 * time.Minute
	DefaultEventBufferCap = 128
)

type EventBuffer = *circular.RingBuffer[EnhancedEvent]

func NewEventBuffer(capacity int) EventBuffer {
	return circular.NewRingBuffer[EnhancedEvent](capacity)
}

func timeSinceOccurrence(event *corev1.Event) time.Duration {
	timestamp := event.LastTimestamp.Time
	if timestamp.IsZero() {
		timestamp = event.EventTime.Time
	}
	return time.Since(timestamp)
}

type WatcherOption func(*Watcher)

type Watcher struct {
	informer    cache.SharedIndexInformer
	cluster     Cluster
	maxEventAge time.Duration
	logger      *zap.Logger
}

func NewWatcher(config *rest.Config, cluster Cluster, opts ...WatcherOption) *Watcher {
	client := kubernetes.NewForConfigOrDie(config)
	factory := informers.NewSharedInformerFactoryWithOptions(
		client,
		NoResyncPeriod,
		informers.WithNamespace(cluster.TargetNamespace),
	)
	informer := factory.Core().V1().Events().Informer()

	watcher := &Watcher{
		informer:    informer,
		cluster:     cluster,
		maxEventAge: DefaultMaxEventAge,
		logger:      log.New().Named("watcher"),
	}

	for _, opt := range opts {
		opt(watcher)
	}

	return watcher
}

func (w *Watcher) handleEvent(event *corev1.Event, out EventBuffer) {
	w.logger.Info(
		"observed event",
		zap.String("namespace", event.Namespace),
		zap.String("name", event.Name),
		zap.String("reason", event.Reason),
		zap.String("regarding", event.InvolvedObject.Name),
	)

	since := timeSinceOccurrence(event)
	if since > w.maxEventAge {
		w.logger.Debug(
			"event does not meet the max age criteria",
			zap.String("namespace", event.Namespace),
			zap.String("name", event.Name),
			zap.Duration("age", since),
		)
		return
	}

	ev := EnhancedEvent{
		Event:       *event.DeepCopy(),
		ClusterName: w.cluster.Name,
	}
	out.Write(ev)
}

// Watch starts the watcher, which writes observed events to the buffer
// until the context is canceled. It returns an error if the initial cache
// sync fails.
func (w *Watcher) Watch(ctx context.Context, out EventBuffer) error {
	handler, _ := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			event := obj.(*corev1.Event)
			w.handleEvent(event, out)
		},
		UpdateFunc: func(_, obj interface{}) {
			event := obj.(*corev1.Event)
			w.handleEvent(event, out)
		},
		DeleteFunc: func(obj interface{}) {
			// Do nothing as we are only interested in new or
			// updated events.
		},
	})

	if !cache.WaitForCacheSync(ctx.Done(), handler.HasSynced) {
		return fmt.Errorf("initial cache sync failed")
	}

	w.informer.Run(ctx.Done())
	return nil
}

// WithMaxEventAge sets the maximum age of an event to be considered for
// writing to the buffer.
func WithMaxEventAge(age time.Duration) WatcherOption {
	return func(w *Watcher) {
		w.maxEventAge = age
	}
}

// WithLogger sets the logger for the watcher.
func WithLogger(logger *zap.Logger) WatcherOption {
	return func(w *Watcher) {
		w.logger = logger
	}
}
