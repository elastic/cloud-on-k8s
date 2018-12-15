package main

import (
	"github.com/elastic/stack-operators/stack-operator/cmd/manager"
	"github.com/elastic/stack-operators/stack-operator/cmd/snapshotter"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func main() {
	var rootCmd = &cobra.Command{Use: "stack-operator"}
	rootCmd.AddCommand(manager.Cmd, snapshotter.Cmd)
	rootCmd.Execute()
}
