package manager

import (
	"context"
	"crypto/tls"
	"errors"
	k2kconfig "github.com/raczu/kube2kafka/internal/config"
	"github.com/raczu/kube2kafka/pkg/exporter"
	"github.com/raczu/kube2kafka/pkg/kube/watcher"
	"github.com/raczu/kube2kafka/pkg/processor"
	"github.com/segmentio/kafka-go/sasl"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"sync"
)

var (
	ErrManagerNotSetup = errors.New("manager was not set up")
)

type Manager struct {
	watcher    *watcher.Watcher
	processor  *processor.Processor
	exporter   *exporter.Exporter
	config     *k2kconfig.Config
	kubeconfig *rest.Config
	logger     *zap.Logger
	wg         sync.WaitGroup
}

func New(cfg *k2kconfig.Config, kubeconfig *rest.Config, logger *zap.Logger) *Manager {
	return &Manager{
		config:     cfg,
		kubeconfig: kubeconfig,
		logger:     logger,
	}
}

func (m *Manager) Setup() error {
	events := watcher.NewEventBuffer(m.config.BufferSize)
	m.watcher = watcher.New(
		m.kubeconfig,
		m.config.GetCluster(),
		watcher.WithLogger(m.logger.Named("watcher")),
		watcher.WithMaxEventAge(m.config.MaxEventAge),
		watcher.WriteTo(events),
	)

	messages := processor.NewKafkaMessageBuffer(m.config.BufferSize)
	popts := []processor.Option{
		processor.WithLogger(m.logger.Named("processor")),
		processor.WriteTo(messages),
	}

	if len(m.config.Filters) > 0 {
		popts = append(popts, processor.WithFilters(m.config.Filters))
	}

	if len(m.config.Selectors) > 0 {
		popts = append(popts, processor.WithSelectors(m.config.Selectors))
	}

	m.processor = processor.New(
		events,
		popts...,
	)

	eopts := []exporter.Option{
		exporter.WithLogger(m.logger.Named("exporter")),
	}

	codec, err := m.config.Kafka.GetCompression()
	if err != nil {
		return err
	}
	eopts = append(eopts, exporter.WithCompression(codec))

	if m.config.Kafka.RawTLS != nil {
		var config *tls.Config
		config, err = m.config.Kafka.GetTLS()
		if err != nil {
			return err
		}
		eopts = append(eopts, exporter.UseTLS(config))
	}

	if m.config.Kafka.RawSASL != nil {
		var mechanism sasl.Mechanism
		mechanism, err = m.config.Kafka.GetSASL()
		if err != nil {
			return err
		}
		eopts = append(eopts, exporter.UseSASL(mechanism))
	}

	m.exporter = exporter.New(
		messages,
		m.config.Kafka.Topic,
		m.config.Kafka.Brokers,
		eopts...,
	)
	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	if m.watcher == nil || m.processor == nil || m.exporter == nil {
		return ErrManagerNotSetup
	}

	m.logger.Info("starting manager...")
	errs := make(chan error, 1)
	subctx, cancel := context.WithCancel(ctx)
	defer cancel()

	m.wg.Add(3)
	go func() {
		defer m.wg.Done()
		if err := m.watcher.Watch(subctx); err != nil {
			errs <- err
		}
	}()

	go func() {
		defer m.wg.Done()
		m.processor.Process(subctx)
	}()

	go func() {
		defer m.wg.Done()
		if err := m.exporter.Export(subctx); err != nil {
			errs <- err
		}
	}()

	var err error
	select {
	case <-ctx.Done():
		m.logger.Info("context canceled, stopping manager...")
	case err = <-errs:
		m.logger.Error("one of the components failed, stopping manager...")
		cancel()
	}

	m.wg.Wait()
	// Ensure that error is not lost even if the context was canceled.
	close(errs)
	if e, ok := <-errs; ok {
		err = e
	}

	return err
}
