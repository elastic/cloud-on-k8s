// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
)

// serveCSR serves the given csr via an HTTP server listening on the given port.
// It stops when stopChan provides a value or gets closed.
func (i *CertInitializer) serveCSR(stopChan <-chan struct{}) error {
	srv := &http.Server{Addr: fmt.Sprintf(":%d", i.config.Port)}
	http.HandleFunc(certificates.CertInitializerRoute, func(w http.ResponseWriter, r *http.Request) {
		log.Info("CSR request")
		if _, err := w.Write(i.CSR); err != nil {
			log.Error(err, "Failed to write CSR to the HTTP response")
		}
	})
	go func() {
		// stop the server when requested
		<-stopChan
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Error(err, "Failed to shutdown the http server")
		}
	}()
	// run until stopped
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
