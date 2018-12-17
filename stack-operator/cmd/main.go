package main

import (
	"github.com/elastic/stack-operators/stack-operator/cmd/manager"
	"github.com/elastic/stack-operators/stack-operator/cmd/snapshotter"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var log = logf.KBLog.WithName("main")

func main() {
	var rootCmd = &cobra.Command{Use: "stack-operator"}
	rootCmd.AddCommand(manager.Cmd, snapshotter.Cmd)

	if err := rootCmd.Execute(); err != nil {
		log.Error(err, "Unexpected error while running executing command")
	}
}
