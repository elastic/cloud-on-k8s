package webhook

import "sigs.k8s.io/controller-runtime/pkg/webhook"

// Parameters are params for the webhook server.
type Parameters struct {
	// Bootstrap are bootstrap options for the webhook.
	Bootstrap webhook.BootstrapOptions
	// AutoInstall controls whether the operator will try to install the webhook and supporting resources itself.
	AutoInstall bool
}
