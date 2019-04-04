// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	pm "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/spf13/cobra"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	name = "process-manager"
	log  = logf.Log.WithName(name)
)

func main() {
	logf.SetLogger(logf.ZapLogger(false))

	cmd := &cobra.Command{
		Use: name,
		Run: func(cmd *cobra.Command, args []string) {

			cfg, err := NewConfigFromFlags()
			exitOnErr(err)

			procMgr, err := pm.NewProcessManager(cfg)
			exitOnErr(err)

			done := make(chan pm.ExitStatus)
			err = procMgr.Start(done)
			exitOnErr(err)

			// Waiting for the end of the process to exit the program in background
			go procMgr.WaitToExit(done)

			// Waiting for a signal to forward to the process
			sig := waitForSignal()
			err = procMgr.Forward(sig)
			if err != nil {
				exitOnErr(err)
			}

			// Waiting for the process to terminate
			select {}
		},
	}

	err := keystore.BindEnvToFlags(cmd)
	exitOnErr(err)

	err = BindFlagsToEnv(cmd)
	exitOnErr(err)

	err = cmd.Execute()
	exitOnErr(err)
}

func waitForSignal() os.Signal {
	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGTERM)
	sig := <-sigs
	return sig
}

// exitOnErr exits the program if err exists.
func exitOnErr(err error) {
	if err != nil {
		log.Error(err, "Fatal error")
		os.Exit(1)
	}
}
