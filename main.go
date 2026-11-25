package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"go.uber.org/zap"

	k2kconfig "github.com/raczu/kube2kafka/internal/config"
	"github.com/raczu/kube2kafka/internal/manager"
	"github.com/raczu/kube2kafka/pkg/kube"
	log "github.com/raczu/kube2kafka/pkg/logger"
)

// buildVersion returns the version stamped by the Go toolchain from git: the tag for
// tagged builds, otherwise a pseudo-version (with "+dirty" for uncommitted changes).
// Builds without VCS information, e.g. from a source tree without .git, report "(devel)".
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return info.Main.Version + " (commit " + s.Value + ")"
		}
	}
	return info.Main.Version
}

func main() {
	var kubeconfig, config string
	var printVersion bool
	flag.StringVar(&kubeconfig, "kubeconfig", "", "path to the kubeconfig file")
	flag.StringVar(&config, "config", "", "path to the config file")
	flag.BoolVar(&printVersion, "version", false, "print the version and exit")

	opts := log.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	if printVersion {
		fmt.Println(buildVersion())
		return
	}

	logger := log.New(log.UseOptions(&opts))
	defer logger.Sync()
	logger.Info("starting kube2kafka", zap.String("version", buildVersion()))

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err = mgr.Start(ctx); err != nil {
		logger.Fatal("manager failed", zap.Error(err))
	}
}
