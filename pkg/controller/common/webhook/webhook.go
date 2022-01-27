package webhook

import (
	"context"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
)

// SetupValidatingWebhookWithConfig will register a set of validation functions
// at a given path, with a given controller manager, ensuring that the objects
// are within the namespaces that the operator manages.
func SetupValidatingWebhookWithConfig(config *Config) error {
	config.Manager.GetWebhookServer().Register(
		config.WebhookPath,
		&webhook.Admission{
			Handler: &validatingWebhook{
				validator:         config.Validator,
				managedNamespaces: set.Make(config.ManagedNamespace...)}})
	return nil
}

// Config is the configuration for setting up a webhook
type Config struct {
	Manager          ctrl.Manager
	WebhookPath      string
	ManagedNamespace []string
	Validator        admission.Validator
}

type validatingWebhook struct {
	decoder           *admission.Decoder
	managedNamespaces set.StringSet
	validator         admission.Validator
}

var _ admission.DecoderInjector = &validatingWebhook{}

// InjectDecoder injects the decoder automatically.
func (v *validatingWebhook) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

// Handle satisfies the admission.Handler interface
func (v *validatingWebhook) Handle(_ context.Context, req admission.Request) admission.Response {
	obj := &unstructured.Unstructured{}
	err := v.decoder.DecodeRaw(req.Object, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// If this instance is not within the set of managed namespaces
	// for this operator ignore this request.
	if !v.managedNamespaces.Has(obj.GetNamespace()) {
		return admission.Allowed("")
	}

	if req.Operation == admissionv1.Create {
		err = v.validator.ValidateCreate()
		if err != nil {
			return admission.Denied(err.Error())
		}
	}

	if req.Operation == admissionv1.Update {
		err = v.validator.ValidateUpdate(req.Object.Object)
		if err != nil {
			return admission.Denied(err.Error())
		}
	}

	if req.Operation == admissionv1.Delete {
		err = v.validator.ValidateDelete()
		if err != nil {
			return admission.Denied(err.Error())
		}
	}

	return admission.Allowed("")
}
