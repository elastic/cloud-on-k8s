package daemon

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/pvgc"

	"github.com/elastic/k8s-operators/local-volume/pkg/k8s"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/drivers"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
	log "github.com/sirupsen/logrus"
)

// Server handles the driver daemon logic
type Server struct {
	httpServer *http.Server
	driver     drivers.Driver
	k8sClient  *k8s.Client
	nodeName   string
}

// NewServer creates a driver daemon server according to the given arguments
func NewServer(nodeName string, driverKind string, driverOpts drivers.Options) (*Server, error) {
	driver, err := drivers.NewDriver(driverKind, driverOpts)
	if err != nil {
		return nil, err
	}
	k8sClient, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	server := Server{
		driver:    driver,
		k8sClient: k8sClient,
		nodeName:  nodeName,
	}
	server.httpServer = &http.Server{
		Handler: server.SetupRoutes(),
	}

	return &server, nil
}

// Start the server (runs forever)
func (s *Server) Start() error {
	log.Infof("Starting %s driver daemon", s.driver.Info())

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

	// start persistent volume garbage collection
	if err := s.StartPVGC(); err != nil {
		return err
	}

	// run forever (unless something is wrong)
	if err := s.httpServer.Serve(unixListener); err != nil {
		return err
	}
	unixListener.Close()
	return nil
}

// StartPVGC starts the persistent volume garbage collection in a goroutine
func (s *Server) StartPVGC() error {

	log.Info("Starting PV GC controller")

	controller, err := pvgc.NewController(pvgc.ControllerParams{
		Client: s.k8sClient.ClientSet, Driver: s.driver,
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
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

	return nil
}
