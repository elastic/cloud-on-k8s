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

			procMgr, err := pm.NewProcessManager()
			exitOnErr(err)

			err = procMgr.Start()
			exitOnErr(err)

			sig := waitForStop()
			log.Info("Forward signal", "sig", sig)

			err = procMgr.Stop(sig)
			if err != nil && err.Error() == pm.ErrNoSuchProcess {
				exitOnErr(err)
			}
		},
	}

	err := keystore.BindEnvToFlags(cmd)
	exitOnErr(err)

	err = pm.BindFlagsToEnv(cmd)
	exitOnErr(err)

	err = cmd.Execute()
	exitOnErr(err)
}

func waitForStop() os.Signal {
	stop := make(chan os.Signal)
	signal.Notify(stop)
	signal.Ignore(syscall.SIGCHLD)
	sig := <-stop
	return sig
}

// exitOnErr exits the program if err exists.
func exitOnErr(err error) {
	if err != nil {
		log.Error(err, "Fatal error")
		os.Exit(1)
	}
}
