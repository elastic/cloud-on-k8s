// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"syscall"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	name   = "process-manager"
	logger = logf.Log.WithName(name)

	procNameFlag = "name"
	procCmdFlag  = "cmd"
)

func main() {
	logf.SetLogger(logf.ZapLogger(false))

	cmd := &cobra.Command{
		Use: name,
		Run: func(cmd *cobra.Command, args []string) {

			// FIXME
			procName := viper.GetString(procNameFlag)
			procCmd := viper.GetString(procCmdFlag)

			pm := NewProcessManager()
			pm.Register(procName, procCmd)
			pm.Start()

			waitForStop()
			pm.Stop()
			os.Exit(0)
		},
	}

	// FIXME
	cmd.Flags().StringP(procNameFlag, "n", "", "process name to manage")
	cmd.Flags().StringP(procCmdFlag, "m", "", "process command to manage")

	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		logger.Error(err, "Unexpected error while binding flags")
		return
	}

	if err := cmd.Execute(); err != nil {
		logger.Error(err, "Unexpected error while running command")
	}
}

func waitForStop() {
	stop := make(chan os.Signal)
	signal.Notify(stop)
	signal.Ignore(syscall.SIGCHLD)
	<-stop
}
