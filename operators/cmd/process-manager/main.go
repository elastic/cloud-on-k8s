// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"syscall"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	name   = "process-manager"
	logger = logf.Log.WithName(name)
)

func main() {
	logf.SetLogger(logf.ZapLogger(false))

	cmd := &cobra.Command{
		Use: name,
		Run: func(cmd *cobra.Command, args []string) {

			pm, err := NewProcessManager(cmd)
			fatal("Fail to create process manager", err)
			msg, err := pm.Start()
			fatal(msg, err)

			sig := waitForStop()
			logger.Info("Forward signal", "sig", sig)
			msg, err = pm.Stop(sig)
			if err != nil {
				if err.Error() == errNoSuchProcess {
					logger.Info("No process to stop")
				}
				os.Exit(1)
			}

			os.Exit(0)
		},
	}

	if err := cmd.Execute(); err != nil {
		logger.Error(err, "Unexpected error while running command")
	}
}

func waitForStop() os.Signal {
	stop := make(chan os.Signal)
	signal.Notify(stop)
	signal.Ignore(syscall.SIGCHLD)
	sig := <-stop
	return sig
}

func fatal(msg string, err error) {
	if err != nil {
		logger.Error(err, msg)
		os.Exit(1)
	}
}
