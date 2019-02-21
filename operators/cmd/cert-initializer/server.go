package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts"
)

// serveCSR serves the given csr via an HTTP server listening on the given port.
// It stops when stopChan provides a value or gets closed.
func serveCSR(port int, csr []byte, stopChan <-chan struct{}) error {
	srv := &http.Server{Addr: fmt.Sprintf(":%d", port)}
	http.HandleFunc(nodecerts.CertInitializerRoute, func(w http.ResponseWriter, r *http.Request) {
		log.Info("CSR request")
		w.Write(csr)
	})
	go func() {
		// stop the server when requested
		<-stopChan
		srv.Shutdown(context.Background())
	}()
	// run until stopped
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
