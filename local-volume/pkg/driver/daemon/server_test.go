package daemon

import (
	"net/http"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/drivers/empty"
	"github.com/elastic/k8s-operators/local-volume/pkg/k8s"
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
