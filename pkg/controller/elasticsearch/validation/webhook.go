// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"context"
	"net/http"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"k8s.io/api/admission/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-elasticsearch-k8s-elastic-co-v1-elasticsearch,mutating=false,failurePolicy=ignore,groups=elasticsearch.k8s.elastic.co,resources=elasticsearches,verbs=create;update,versions=v1,name=elastic-es-validation-v1.k8s.elastic.co

const (
	webhookPath = "/validate-elasticsearch-k8s-elastic-co-v1-elasticsearch"
)

var eslog = logf.Log.WithName("es-validation")

func RegisterWebhook(mgr ctrl.Manager, validateStorageClass bool) {
	wh := &validatingWebhook{
		client:               k8s.WrapClient(mgr.GetClient()),
		validateStorageClass: validateStorageClass,
	}
	eslog.Info("Registering Elasticsearch validating webhook", "path", webhookPath)
	mgr.GetWebhookServer().Register(webhookPath, &webhook.Admission{Handler: wh})
}

type validatingWebhook struct {
	client               k8s.Client
	decoder              *admission.Decoder
	validateStorageClass bool
}

var _ admission.DecoderInjector = &validatingWebhook{}

// InjectDecoder injects the decoder automatically.
func (wh *validatingWebhook) InjectDecoder(d *admission.Decoder) error {
	wh.decoder = d
	return nil
}

func (wh *validatingWebhook) validateCreate(es esv1.Elasticsearch) error {
	eslog.V(1).Info("validate create", "name", es.Name)
	return ValidateElasticsearch(es)
}

func (wh *validatingWebhook) validateUpdate(old esv1.Elasticsearch, new esv1.Elasticsearch) error {
	eslog.V(1).Info("validate update", "name", new.Name)

	var errs field.ErrorList
	for _, val := range updateValidations(wh.client, wh.validateStorageClass) {
		if err := val(old, new); err != nil {
			errs = append(errs, err...)
		}
	}
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: "Elasticsearch"},
			new.Name, errs)
	}
	return ValidateElasticsearch(new)
}

func (wh *validatingWebhook) Handle(_ context.Context, req admission.Request) admission.Response {
	es := &esv1.Elasticsearch{}
	err := wh.decoder.DecodeRaw(req.Object, es)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if req.Operation == v1beta1.Create {
		err = wh.validateCreate(*es)
		if err != nil {
			return admission.Denied(err.Error())
		}
	}

	if req.Operation == v1beta1.Update {
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

func ValidateElasticsearch(es esv1.Elasticsearch) error {
	errs := check(es, validations)
	if len(errs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "elasticsearch.k8s.elastic.co", Kind: "Elasticsearch"},
			es.Name,
			errs,
		)
	}
	return nil
}
