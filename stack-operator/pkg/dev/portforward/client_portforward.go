package portforward

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// newKubectlPortForwarder creates a new PortForwarder using kubectl tooling
func newKubectlPortForwarder(
	ctx context.Context,
	namespace, podName string,
	ports []string,
	readyChan chan struct{},
) (*portforward.PortForwarder, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	req := clientSet.RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward")

	u := url.URL{
		Scheme:   req.URL().Scheme,
		Host:     req.URL().Host,
		Path:     "/api/v1" + req.URL().Path,
		RawQuery: "timeout=32s",
	}

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return nil, err
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &u)

	// wrap stdout / stderr through logging
	w := &logWriter{keysAndValues: []interface{}{
		"namespace", namespace,
		"pod", podName,
		"ports", ports,
	}}
	return portforward.New(dialer, ports, ctx.Done(), readyChan, w, w)
}

// logWriter is a small utility that writes data from an io.Writer to a log
type logWriter struct {
	keysAndValues []interface{}
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	log.Info(strings.TrimSpace(string(p)), w.keysAndValues...)

	return len(p), nil
}
