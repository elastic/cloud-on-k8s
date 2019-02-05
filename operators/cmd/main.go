package main

import (
	"github.com/elastic/k8s-operators/operators/cmd/manager"
	"github.com/elastic/k8s-operators/operators/cmd/snapshotter"
	"github.com/elastic/k8s-operators/operators/pkg/dev"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.KBLog.WithName("main")

func main() {
	var rootCmd = &cobra.Command{Use: "stack-operator"}
	rootCmd.AddCommand(manager.Cmd, snapshotter.Cmd)
	// development mode is only available as a command line flag to avoid accidentally enabling it
	rootCmd.PersistentFlags().BoolVar(&dev.Enabled, "development", false, "turns on development mode")

	cobra.OnInitialize(func() {
		logf.SetLogger(logf.ZapLogger(dev.Enabled))
	})

	if err := rootCmd.Execute(); err != nil {
		log.Error(err, "Unexpected error while running executing command")
	}
}
