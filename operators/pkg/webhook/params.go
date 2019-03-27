package webhook

import "sigs.k8s.io/controller-runtime/pkg/webhook"

// Parameters are params for the webhook server.
type Parameters struct {
	Namespace   string
	Bootstrap   webhook.BootstrapOptions
	AutoInstall bool
}
