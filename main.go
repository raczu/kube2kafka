package main

import (
	"flag"
	log "github.com/raczu/kube2kafka/pkg/logger"
)

func main() {
	opts := log.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logger := log.New(log.UseOptions(&opts))
	defer logger.Sync()
	logger.Info("Hello, world!")
}
