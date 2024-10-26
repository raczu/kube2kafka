package kube

import (
	"context"
	"fmt"
	"github.com/raczu/kube2kafka/pkg/circular"
	log "github.com/raczu/kube2kafka/pkg/logger"
	"go.uber.org/zap"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"time"
)

const (
	DefaultEventBufferCap = 128
	// noResyncPeriod is used to disable the resync of the informer to avoid
	// unnecessary writes to the buffer.
	noResyncPeriod = 0
	// defaultMaxEventAge is the default maximum age of an event to be considered
	// for writing to the buffer. This applies only during the initial cache sync.
	defaultMaxEventAge = 1 * time.Minute
	defaultSyncTimeout = 30 * time.Second
)

type EventBuffer = *circular.RingBuffer[EnhancedEvent]

func NewEventBuffer(capacity int) EventBuffer {
	return circular.NewRingBuffer[EnhancedEvent](capacity)
}

type WatcherOption func(*Watcher)

type Watcher struct {
	informer    cache.SharedIndexInformer
	maxEventAge time.Duration
	handler     *EventHandler
	output      EventBuffer
	logger      *zap.Logger
}

func NewWatcher(config *rest.Config, cluster Cluster, opts ...WatcherOption) *Watcher {
	client := kubernetes.NewForConfigOrDie(config)
	factory := informers.NewSharedInformerFactoryWithOptions(
		client,
		noResyncPeriod,
		informers.WithNamespace(cluster.TargetNamespace),
	)
	informer := factory.Core().V1().Events().Informer()

	watcher := &Watcher{
		informer:    informer,
		logger:      log.New().Named("watcher"),
		output:      NewEventBuffer(DefaultEventBufferCap),
		maxEventAge: defaultMaxEventAge,
	}

	for _, opt := range opts {
		opt(watcher)
	}

	watcher.handler = &EventHandler{
		output:      watcher.output,
		clusterName: cluster.Name,
		maxEventAge: watcher.maxEventAge,
		logger:      watcher.logger.Named("handler"),
	}
	return watcher
}

// GetBuffer returns the buffer where the watcher writes the observed events.
func (w *Watcher) GetBuffer() EventBuffer {
	return w.output
}

// Watch starts the watcher, which writes observed events to the buffer
// until the context is canceled. It returns an error if the initial cache
// sync fails.
func (w *Watcher) Watch(ctx context.Context) error {
	h, _ := w.informer.AddEventHandler(w.handler)
	go w.informer.Run(ctx.Done())

	sync, cancel := context.WithTimeout(ctx, defaultSyncTimeout)
	defer cancel()

	w.logger.Info("syncing initial watcher cache...")
	if !cache.WaitForCacheSync(sync.Done(), h.HasSynced) {
		return fmt.Errorf("initial cache sync for watcher failed: %w", sync.Err())
	}
	w.logger.Info("initial watcher cache synced")

	<-ctx.Done()
	return nil
}

// WithMaxEventAge sets the maximum age of an event to be considered for
// writing to the buffer. This applies only during the initial cache sync.
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

// WriteTo sets the buffer where the watcher writes the observed events.
func WriteTo(output EventBuffer) WatcherOption {
	return func(w *Watcher) {
		w.output = output
	}
}
