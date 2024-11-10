package main

import (
	"context"
	"flag"
	k2kconfig "github.com/raczu/kube2kafka/internal/config"
	"github.com/raczu/kube2kafka/internal/manager"
	"github.com/raczu/kube2kafka/pkg/kube"
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

	kubecfg, err := kube.GetKubeConfig(kubeconfig)
	if err != nil {
		logger.Fatal("failed to get kubeconfig", zap.Error(err))
	}

	var cfg *k2kconfig.Config
	cfg, err = k2kconfig.Read(config)
	if err != nil {
		logger.Fatal("failed to read config", zap.Error(err))
	}

	cfg.SetDefaults()
	if err = cfg.Validate(); err != nil {
		logger.Fatal("failed to validate provided config", zap.Error(err))
	}

	mgr := manager.New(cfg, kubecfg, logger.Named("manager"))
	if err = mgr.Setup(); err != nil {
		logger.Fatal("failed to set up manager", zap.Error(err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	if err = mgr.Start(ctx); err != nil {
		logger.Fatal("manager failed", zap.Error(err))
	}
}
