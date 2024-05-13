// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	autoscalev1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/maps/v1alpha1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// +kubebuilder:webhook:path=/validate-prevent-crd-deletion-k8s-elastic-co,mutating=false,failurePolicy=ignore,groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=delete,versions=v1,name=elastic-prevent-crd-deletion.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

const (
	webhookPath = "/validate-prevent-crd-deletion-k8s-elastic-co"
)

var whlog = ulog.Log.WithName("crd-delete-validation")

// RegisterCRDDeletionWebhook will register the crd deletion prevention webhook.
func RegisterCRDDeletionWebhook(mgr ctrl.Manager) {
	wh := &crdDeletionWebhook{
		client:  mgr.GetClient(),
		decoder: admission.NewDecoder(mgr.GetScheme()),
	}
	whlog.Info("Registering CRD deletion prevention validating webhook", "path", webhookPath)
	mgr.GetWebhookServer().Register(webhookPath, &webhook.Admission{Handler: wh})
}

type crdDeletionWebhook struct {
	client  k8s.Client
	decoder admission.Decoder
}

// Handle is called when any request is sent to the webhook, satisfying the admission.Handler interface.
func (wh *crdDeletionWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Delete {
		return admission.Allowed("")
	}
	crd := &extensionsv1.CustomResourceDefinition{}
	err := wh.decoder.DecodeRaw(req.OldObject, crd)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if isElasticCRD(crd) && wh.isInUse(crd) {
		return admission.Denied("deletion of Elastic CRDs is not allowed while in use")
	}

	return admission.Allowed("")
}

func isElasticCRD(crd *extensionsv1.CustomResourceDefinition) bool {
	return slices.Contains(
		[]string{
			agentv1alpha1.GroupVersion.Group,
			apmv1.GroupVersion.Group,
			autoscalev1alpha1.GroupVersion.Group,
			beatv1beta1.GroupVersion.Group,
			esv1.GroupVersion.Group,
			entv1.GroupVersion.Group,
			kbv1.GroupVersion.Group,
			logstashv1alpha1.GroupVersion.Group,
			emsv1alpha1.GroupVersion.Group,
			policyv1alpha1.GroupVersion.Group,
		}, crd.Spec.Group)
}

func (wh *crdDeletionWebhook) isInUse(crd *extensionsv1.CustomResourceDefinition) bool {
	ul := &unstructured.UnstructuredList{}
	for _, version := range crd.Spec.Versions {
		ul.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   crd.Spec.Group,
			Kind:    crd.Spec.Names.Kind,
			Version: version.Name,
		})
		err := wh.client.List(context.Background(), ul, client.InNamespace(""))
		if err != nil {
			whlog.Error(err, "while listing resources")
			return true
		}
		if len(ul.Items) > 0 {
			return true
		}
	}
	return false
}
