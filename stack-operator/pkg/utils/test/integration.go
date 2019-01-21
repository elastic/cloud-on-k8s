package test

import (
	"os"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	// ControlPlaneStartTimeout is the time to wait for control plane startup
	// in kubebuilder integration tests.
	// It is set at a relatively high value due to low resources in continuous integration.
	ControlPlaneStartTimeout = 2 * time.Minute
)

var Config *rest.Config
var log = logf.Log.WithName("integration-test")

// RunWithK8s starts a local Kubernetes server and runs tests in m.
func RunWithK8s(m *testing.M, crdPath string) {
	logf.SetLogger(logf.ZapLogger(true))
	t := &envtest.Environment{
		CRDDirectoryPaths:        []string{crdPath},
		ControlPlaneStartTimeout: ControlPlaneStartTimeout,
	}
	apis.AddToScheme(scheme.Scheme)

	var err error
	if Config, err = t.Start(); err != nil {
		log.Error(err, "failed to start")
	}

	code := m.Run()
	t.Stop()
	os.Exit(code)
}
