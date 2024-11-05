package main

import (
	"context"
	"flag"
	"github.com/raczu/kube2kafka/internal/manager"
	log "github.com/raczu/kube2kafka/pkg/logger"
	"go.uber.org/zap"
	"os"
	"os/signal"
)

func main() {
	var kubeconfig, config string
	flag.StringVar(&kubeconfig, "kubeconfig", "", "path to the kubeconfig file")
	flag.StringVar(&config, "config", "", "path to the config file")

	opts := log.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	logger := log.New(log.UseOptions(&opts))
	defer logger.Sync()

	mgr := manager.New(
		manager.GetConfigOrDie(config, logger),
		manager.GetKubeConfigOrDie(kubeconfig, logger),
		logger.Named("manager"),
	)

	if err := mgr.Setup(); err != nil {
		logger.Fatal("failed to set up manager", zap.Error(err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	if err := mgr.Start(ctx); err != nil {
		logger.Fatal("manager failed", zap.Error(err))
	}
}
