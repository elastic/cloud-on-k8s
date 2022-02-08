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

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
)

// +kubebuilder:webhook:path=/validate-elasticsearch-k8s-elastic-co-v1-elasticsearch,mutating=false,failurePolicy=ignore,groups=elasticsearch.k8s.elastic.co,resources=elasticsearches,verbs=create;update,versions=v1,name=elastic-es-validation-v1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1;v1beta1,matchPolicy=Exact

const (
	webhookPath = "/validate-elasticsearch-k8s-elastic-co-v1-elasticsearch"
)

var eslog = ulog.Log.WithName("es-validation")

// RegisterWebhook will register the Elasticsearch validating webhook.
func RegisterWebhook(mgr ctrl.Manager, validateStorageClass bool, exposedNodeLabels NodeLabels, managedNamespaces []string) {
	wh := &validatingWebhook{
		client:               mgr.GetClient(),
		validateStorageClass: validateStorageClass,
		exposedNodeLabels:    exposedNodeLabels,
		managedNamespaces:    set.Make(managedNamespaces...),
	}
	eslog.Info("Registering Elasticsearch validating webhook", "path", webhookPath)
	mgr.GetWebhookServer().Register(webhookPath, &webhook.Admission{Handler: wh})
}

type validatingWebhook struct {
	client               k8s.Client
	decoder              *admission.Decoder
	validateStorageClass bool
	exposedNodeLabels    NodeLabels
	managedNamespaces    set.StringSet
}

var _ admission.DecoderInjector = &validatingWebhook{}

// InjectDecoder injects the decoder automatically.
func (wh *validatingWebhook) InjectDecoder(d *admission.Decoder) error {
	wh.decoder = d
	return nil
}

func (wh *validatingWebhook) validateCreate(es esv1.Elasticsearch) error {
	eslog.V(1).Info("validate create", "name", es.Name)
	return ValidateElasticsearch(es, wh.exposedNodeLabels)
}

func (wh *validatingWebhook) validateUpdate(prev esv1.Elasticsearch, curr esv1.Elasticsearch) error {
	eslog.V(1).Info("validate update", "name", curr.Name)
	var errs field.ErrorList
	for _, val := range updateValidations(wh.client, wh.validateStorageClass) {
		if err := val(prev, curr); err != nil {
			errs = append(errs, err...)
		}
	}
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: esv1.Kind},
			curr.Name, errs)
	}
	return ValidateElasticsearch(curr, wh.exposedNodeLabels)
}

// Handle is called when any request is sent to the webhook, satisfying the admission.Handler interface.
func (wh *validatingWebhook) Handle(_ context.Context, req admission.Request) admission.Response {
	es := &esv1.Elasticsearch{}
	err := wh.decoder.DecodeRaw(req.Object, es)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// If this Elasticsearch instance is not within the set of managed namespaces
	// for this operator ignore this request.
	if wh.managedNamespaces.Count() > 0 && !wh.managedNamespaces.Has(es.Namespace) {
		eslog.V(1).Info("Skip Elasticsearch resource validation", "name", es.Name, "namespace", es.Namespace)
		return admission.Allowed("")
	}

	if req.Operation == admissionv1.Create {
		err = wh.validateCreate(*es)
		if err != nil {
			return admission.Denied(err.Error())
		}
	}

	if req.Operation == admissionv1.Update {
		oldObj := &esv1.Elasticsearch{}
		err = wh.decoder.DecodeRaw(req.OldObject, oldObj)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		err = wh.validateUpdate(*oldObj, *es)
		if err != nil {
			return admission.Denied(err.Error())
		}
	}

	return admission.Allowed("")
}

// ValidateElasticsearch validates an Elasticsearch instance against a set of validation funcs.
func ValidateElasticsearch(es esv1.Elasticsearch, exposedNodeLabels NodeLabels) error {
	errs := check(es, validations(exposedNodeLabels))
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: esv1.Kind},
			es.Name,
			errs,
		)
	}
	return nil
}
