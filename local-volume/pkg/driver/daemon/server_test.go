package daemon

import (
	"net/http"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/drivers/empty"
	"github.com/elastic/stack-operators/local-volume/pkg/k8s"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func NewPersistentVolumeStub(name string) *v1.PersistentVolume {
	return &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func NewTestServer(k8sObj ...runtime.Object) *Server {
	server := Server{
		driver: &empty.Driver{},
		k8sClient: &k8s.Client{
			ClientSet: fake.NewSimpleClientset(k8sObj...),
		},
		nodeName: "testNode",
	}
	server.httpServer = &http.Server{
		Handler: server.SetupRoutes(),
	}
	return &server
}
