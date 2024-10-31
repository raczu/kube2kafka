package main

import (
	"context"
	"flag"
	"github.com/raczu/kube2kafka/pkg/kube"
	"github.com/raczu/kube2kafka/pkg/kube/watcher"
	log "github.com/raczu/kube2kafka/pkg/logger"
	"github.com/raczu/kube2kafka/pkg/processor"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"os"
	"os/signal"
	"sync"
)

func main() {
	opts := log.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logger := log.New(log.UseOptions(&opts))
	defer logger.Sync()

	config, _ := kube.GetKubeConfig("")
	cluster := kube.Cluster{
		Name:            "dev.kube2kafka.local",
		TargetNamespace: corev1.NamespaceAll,
	}
	w := watcher.New(config, cluster, watcher.WithLogger(logger.Named("watcher")))
	events := w.GetBuffer()

	selectors := []processor.Selector{
		processor.Selector{Key: "foo", Value: "{{ .Related.Kind }}"},
	}

	p := processor.New(
		events,
		processor.WithLogger(logger.Named("processor")),
		processor.WithSelectors(selectors),
	)
	messages := p.GetBuffer()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := w.Watch(ctx); err != nil {
			logger.Error("watcher error", zap.Error(err))
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		p.Process(ctx)
	}()

	sigs, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	<-sigs.Done()
	cancel()
	wg.Wait()

	logger.Info("processor output", zap.Int("size", messages.Size()))
}
