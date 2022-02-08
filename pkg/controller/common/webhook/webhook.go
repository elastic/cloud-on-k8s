// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
)

var whlog = ulog.Log.WithName("common-webhook")

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
	obj, ok := v.validator.DeepCopyObject().(admission.Validator)
	if !ok {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("object (%T) to be validated couldn't be converted to admission.Validator", v.validator.DeepCopyObject()))
	}

	err := v.decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	metaObj, ok := obj.(metav1.Object)
	if !ok {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to convert webhook object (%T) into metav1.Object: %v", obj, obj))
	}

	// If this instance is not within the set of managed namespaces
	// for this operator ignore this request.
	if v.managedNamespaces.Count() > 0 && !v.managedNamespaces.Has(metaObj.GetNamespace()) {
		whlog.V(1).Info("Skipping resource validation", "name", metaObj.GetName(), "namespace", metaObj.GetNamespace())
		return admission.Allowed(fmt.Sprintf("object namespace (%s) outside of managed namespaces: %s", metaObj.GetNamespace(), v.managedNamespaces.AsSlice()))
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
