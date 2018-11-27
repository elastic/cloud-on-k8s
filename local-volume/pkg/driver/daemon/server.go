package daemon

import (
	"context"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/pvgc"
	"k8s.io/client-go/kubernetes"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/drivers"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	log "github.com/sirupsen/logrus"
)

func Start(driverKind string, driverOpts drivers.Options) error {
	// create a driver of the appropriate kind
	driver, err := drivers.NewDriver(driverKind, driverOpts)
	if err != nil {
		return err
	}

	cfg, err := pvgc.GetConfig()
	if err != nil {
		return err
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}

	log.Infof("Starting PV GC controller", driverKind)

	controller, err := pvgc.NewController(client, "", driver)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	go func() {
		if err := controller.Run(ctx); err != nil {
			if ctx.Err() == context.Canceled {
				log.Error(err)
			} else {
				log.Fatal(err)
			}
		}
	}()

	log.Infof("Starting driver daemon %s", driver.Info())

	// create the http server
	server := http.Server{
		Handler: SetupRoutes(driver),
	}

	// unlink the socket if already exists (previous pod)
	if err := syscall.Unlink(protocol.UnixSocket); err != nil {
		// ok to fail here
		log.Info("No socket to unlink (it's probably ok, might not exit yet): ", err.Error())
	}

	// bind to the unix domain socket
	unixListener, err := net.Listen("unix", protocol.UnixSocket)
	if err != nil {
		return err
	}

	// properly close socket on process termination
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Printf("Caught signal %s: shutting down.", sig)
		unixListener.Close()
		os.Exit(0)
	}()

	// run forever (unless something is wrong)
	if err := server.Serve(unixListener); err != nil {
		return err
	}
	unixListener.Close()
	return nil
}
