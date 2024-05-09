// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
)

// +kubebuilder:webhook:path=/validate-logstash-k8s-elastic-co-v1alpha1-logstash,mutating=false,failurePolicy=ignore,groups=logstash.k8s.elastic.co,resources=logstashes,verbs=create;update,versions=v1alpha1,name=elastic-logstash-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1,matchPolicy=Exact

const (
	webhookPath = "/validate-logstash-k8s-elastic-co-v1alpha1-logstash"
)

var lslog = ulog.Log.WithName("ls-validation")

// RegisterWebhook will register the Logstash validating webhook.
func RegisterWebhook(mgr ctrl.Manager, validateStorageClass bool, managedNamespaces []string) {
	wh := &validatingWebhook{
		client:               mgr.GetClient(),
		decoder:              admission.NewDecoder(mgr.GetScheme()),
		validateStorageClass: validateStorageClass,
		managedNamespaces:    set.Make(managedNamespaces...),
	}
	lslog.Info("Registering Logstash validating webhook", "path", webhookPath)
	mgr.GetWebhookServer().Register(webhookPath, &webhook.Admission{Handler: wh})
}

type validatingWebhook struct {
	client               k8s.Client
	decoder              admission.Decoder
	validateStorageClass bool
	managedNamespaces    set.StringSet
}

func (wh *validatingWebhook) ValidateCreate(ls *lsv1alpha1.Logstash) error {
	lslog.V(1).Info("validate create", "name", ls.Name)
	return ValidateLogstash(ls)
}

func (wh *validatingWebhook) ValidateUpdate(ctx context.Context, prev *lsv1alpha1.Logstash, curr *lsv1alpha1.Logstash) error {
	lslog.V(1).Info("validate update", "name", curr.Name)
	var errs field.ErrorList
	for _, val := range updateValidations(ctx, wh.client, wh.validateStorageClass) {
		if err := val(prev, curr); err != nil {
			errs = append(errs, err...)
		}
	}
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "logstash.k8s.elastic.co", Kind: lsv1alpha1.Kind},
			curr.Name, errs)
	}
	return ValidateLogstash(curr)
}

// Handle is called when any request is sent to the webhook, satisfying the admission.Handler interface.
func (wh *validatingWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	ls := &lsv1alpha1.Logstash{}
	err := wh.decoder.DecodeRaw(req.Object, ls)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// If this Logstash instance is not within the set of managed namespaces
	// for this operator ignore this request.
	if wh.managedNamespaces.Count() > 0 && !wh.managedNamespaces.Has(ls.Namespace) {
		lslog.V(1).Info("Skip Logstash resource validation", "name", ls.Name, "namespace", ls.Namespace)
		return admission.Allowed("")
	}

	if req.Operation == admissionv1.Create {
		err = wh.ValidateCreate(ls)
		if err != nil {
			return admission.Denied(err.Error())
		}
	}

	if req.Operation == admissionv1.Update {
		oldObj := &lsv1alpha1.Logstash{}
		err = wh.decoder.DecodeRaw(req.OldObject, oldObj)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		err = wh.ValidateUpdate(ctx, oldObj, ls)
		if err != nil {
			return admission.Denied(err.Error())
		}
	}

	return admission.Allowed("")
}

// ValidateLogstash validates an Logstash instance against a set of validation funcs.
func ValidateLogstash(ls *lsv1alpha1.Logstash) error {
	errs := check(ls, validations())
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "logstash.k8s.elastic.co", Kind: lsv1alpha1.Kind},
			ls.Name,
			errs,
		)
	}
	return nil
}
