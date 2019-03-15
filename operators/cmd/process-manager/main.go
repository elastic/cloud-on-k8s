package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	logger = logf.Log.WithName("process-manager")

	processName string
	processCmd  string
)

func main() {
	logf.SetLogger(logf.ZapLogger(false))

	flag.StringVar(&processName, "name", "", "process name")
	flag.StringVar(&processCmd, "cmd", "", "process command")
	flag.Parse()

	pm := NewProcessManager()
	pm.Register(processName, processCmd)
	pm.Start()

	waitForStop()
	pm.Stop()
	os.Exit(0)
}

func waitForStop() {
	stop := make(chan os.Signal)
	signal.Notify(stop)
	signal.Ignore(syscall.SIGCHLD)
	<-stop
}
