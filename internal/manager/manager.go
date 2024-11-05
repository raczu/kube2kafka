package manager

import (
	"context"
	"crypto/tls"
	k2kconfig "github.com/raczu/kube2kafka/internal/config"
	"github.com/raczu/kube2kafka/pkg/exporter"
	"github.com/raczu/kube2kafka/pkg/kube"
	"github.com/raczu/kube2kafka/pkg/kube/watcher"
	"github.com/raczu/kube2kafka/pkg/processor"
	"github.com/segmentio/kafka-go/sasl"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"sync"
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
	m.logger.Info("starting manager...")
	ch := make(chan error, 1)
	subctx, cancel := context.WithCancel(ctx)
	defer cancel()

	m.wg.Add(3)
	go func() {
		defer m.wg.Done()
		if err := m.watcher.Watch(subctx); err != nil {
			ch <- err
			return
		}
	}()

	go func() {
		defer m.wg.Done()
		m.processor.Process(subctx)
	}()

	go func() {
		defer m.wg.Done()
		if err := m.exporter.Export(subctx); err != nil {
			ch <- err
		}
	}()

	var err error
	select {
	case <-subctx.Done():
		m.logger.Info("context canceled, shutting down manager...")
	case err = <-ch:
		m.logger.Error("one of the components failed", zap.Error(err))
		cancel()
	}

	m.wg.Wait()
	return err
}

// GetKubeConfigOrDie reads the kubeconfig, exiting the program if it fails.
func GetKubeConfigOrDie(kubeconfig string, logger *zap.Logger) *rest.Config {
	config, err := kube.GetKubeConfig(kubeconfig)
	if err != nil {
		logger.Fatal("error getting kubeconfig", zap.Error(err))
	}
	return config
}

// GetConfigOrDie reads the config file and validates it, exiting the program if it fails.
func GetConfigOrDie(config string, logger *zap.Logger) *k2kconfig.Config {
	cfg, err := k2kconfig.Read(config)
	if err != nil {
		logger.Fatal("error reading config", zap.Error(err))
	}
	cfg.SetDefaults()
	if err = cfg.Validate(); err != nil {
		logger.Fatal("failed to validate provided config", zap.Error(err))
	}
	return cfg
}
