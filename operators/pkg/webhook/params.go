package webhook

import "sigs.k8s.io/controller-runtime/pkg/webhook"

// Parameters are params for the webhook server.
type Parameters struct {
	// Namespace is the namespace the webhook will run in.
	Namespace string
	// Bootstrap are bootstrap options for the webhook.
	Bootstrap webhook.BootstrapOptions
	// AutoInstall controls whether the operator will try to install the webhook and supporting resources itself.
	AutoInstall bool
}
