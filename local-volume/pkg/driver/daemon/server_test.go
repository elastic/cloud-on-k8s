// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package daemon

import (
	"net/http"

	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/daemon/drivers/empty"
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/k8s"
	"k8s.io/apimachinery/pkg/runtime"
)

// NewTestServer creates a Server with an empty driver and a fake k8s client,
// intended to be used for unit testing
func NewTestServer(k8sObj ...runtime.Object) *Server {
	server := Server{
		driver:    &empty.Driver{},
		nodeName:  "testNode",
		k8sClient: k8s.NewTestClient(k8sObj...),
	}
	server.httpServer = &http.Server{
		Handler: server.SetupRoutes(),
	}
	return &server
}
