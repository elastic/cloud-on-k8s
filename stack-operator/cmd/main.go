package main

import (
	"github.com/elastic/stack-operators/stack-operator/cmd/manager"
	"github.com/elastic/stack-operators/stack-operator/cmd/snapshotter"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func main() {
	logf.SetLogger(logf.ZapLogger(false))
	var rootCmd = &cobra.Command{Use: "stack-operator"}
	rootCmd.AddCommand(manager.Cmd, snapshotter.Cmd)
	rootCmd.Execute()
}
