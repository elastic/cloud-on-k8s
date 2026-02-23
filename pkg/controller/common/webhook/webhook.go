// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	eckadmission "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook/admission"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// validatableObject is the object to be validated, along with methods
// that allow retrieval of namespace and name to ignore any objects
// that are outside of the operator's managed namespaces.
type validatableObject interface {
	eckadmission.Validator
	metav1.Object
}

// SetupValidatingWebhookWithConfig will register a set of validation functions
// at a given path, with a given controller manager, ensuring that the objects
// are within the namespaces that the operator manages.
func SetupValidatingWebhookWithConfig(config *Config) {
	config.Manager.GetWebhookServer().Register(
		config.WebhookPath,
		ValidatingWebhookFor(
			config.Manager.GetScheme(),
			config.Validator,
			config.LicenseChecker,
			set.Make(config.ManagedNamespace...)),
	)
}

func ValidatingWebhookFor(
	scheme *runtime.Scheme,
	validator eckadmission.Validator,
	licenseChecker license.Checker,
	managedNamespaces set.StringSet,
) *webhook.Admission {
	return &webhook.Admission{
		Handler: &validatingWebhook{
			decoder:           admission.NewDecoder(scheme),
			validator:         validator,
			licenseChecker:    licenseChecker,
			managedNamespaces: managedNamespaces,
		},
	}
}

// Config is the configuration for setting up a webhook
type Config struct {
	Manager          ctrl.Manager
	WebhookPath      string
	ManagedNamespace []string
	LicenseChecker   license.Checker
	Validator        eckadmission.Validator
}

type validatingWebhook struct {
	decoder           admission.Decoder
	managedNamespaces set.StringSet
	licenseChecker    license.Checker
	validator         eckadmission.Validator
}

// Handle satisfies the admission.Handler interface
func (v *validatingWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	whlog := ulog.FromContext(ctx).WithName("common-webhook")

	obj, ok := v.validator.DeepCopyObject().(validatableObject)
	if !ok {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("object (%T) to be validated couldn't be converted to admission.Validator", v.validator))
	}

	err := v.decoder.Decode(req, obj)
	if err != nil {
		whlog.Error(err, "decoding object from webhook request into type (%T)", obj)
		return admission.Errored(http.StatusBadRequest, err)
	}

	// If this resource is not within the set of managed namespaces
	// for this operator ignore this request.
	if v.managedNamespaces.Count() > 0 && !v.managedNamespaces.Has(obj.GetNamespace()) {
		whlog.V(1).Info("Skip resource validation", "name", obj.GetName(), "namespace", obj.GetNamespace())
		return admission.Allowed("")
	}

	var warnings []string

	if err := v.commonValidations(ctx, req, obj); err != nil {
		return admission.Denied(err.Error()).WithWarnings(warnings...)
	}

	if req.Operation == admissionv1.Create {
		warnings, err = obj.ValidateCreate()
	}

	if req.Operation == admissionv1.Update {
		oldObj := v.validator.DeepCopyObject()
		err = v.decoder.DecodeRaw(req.OldObject, oldObj)
		if err != nil {
			whlog.Error(err, "decoding old object from webhook request into type (%T)", oldObj)
			return admission.Errored(http.StatusBadRequest, err).WithWarnings(warnings...)
		}
		warnings, err = obj.ValidateUpdate(oldObj)
	}

	if req.Operation == admissionv1.Delete {
		warnings, err = obj.ValidateDelete()
	}
	if err != nil {
		var apiStatus apierrors.APIStatus
		if errors.As(err, &apiStatus) {
			return DenyResponseFromStatus(apiStatus.Status()).WithWarnings(warnings...)
		}
		return admission.Denied(err.Error()).WithWarnings(warnings...)
	}

	return admission.Allowed("").WithWarnings(warnings...)
}

// DenyResponseFromStatus returns a response for denying a request with details from the provided Status object.
func DenyResponseFromStatus(status metav1.Status) admission.Response {
	resp := admission.Response{
		AdmissionResponse: admissionv1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		},
	}
	return resp
}
