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

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
)

// +kubebuilder:webhook:path=/validate-prevent-crd-deletion-k8s-elastic-co,mutating=false,failurePolicy=ignore,groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=delete,versions=v1,name=elastic-prevent-crd-deletion.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

const (
	webhookPath = "/validate-prevent-crd-deletion-k8s-elastic-co"
)

var whlog = ulog.Log.WithName("crd-delete-validation")

// RegisterCRDDeletionWebhook will register the crd deletion prevention webhook.
func RegisterCRDDeletionWebhook(mgr ctrl.Manager, managedNamespace []string) {
	wh := &crdDeletionWebhook{
		client:           mgr.GetClient(),
		decoder:          admission.NewDecoder(mgr.GetScheme()),
		managedNamespace: set.Make(managedNamespace...),
	}
	whlog.Info("Registering CRD deletion prevention validating webhook", "path", webhookPath)
	mgr.GetWebhookServer().Register(webhookPath, &webhook.Admission{Handler: wh})
}

type crdDeletionWebhook struct {
	client           k8s.Client
	decoder          *admission.Decoder
	managedNamespace set.StringSet
}

// Handle is called when any request is sent to the webhook, satisfying the admission.Handler interface.
func (wh *crdDeletionWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Delete {
		whlog.Info("Skipping webhook CRD request", "operation", req.Operation)
		return admission.Allowed("")
	}
	crd := &extensionsv1.CustomResourceDefinition{}
	err := wh.decoder.DecodeRaw(req.OldObject, crd)
	if err != nil {
		whlog.Error(err, "Failed to decode CRD during CRD deletion webhook")
		return admission.Errored(http.StatusBadRequest, err)
	}

	if isElasticCRD(crd) && wh.isInUse(crd) {
		whlog.Info("CRD is in use, denying deletion", "crd", crd.Name)
		return admission.Denied("deletion of Elastic CRDs is not allowed")
	}

	whlog.Info("CRD is not in use, allowing deletion", "crd", crd.Name)
	return admission.Allowed("")
}

func isElasticCRD(crd *extensionsv1.CustomResourceDefinition) bool {
	whlog.Info("Checking if CRD is an Elastic CRD", "group", crd.Spec.Group)
	return slices.Contains(
		[]string{
			"agent.k8s.elastic.co",
			"apm.k8s.elastic.co",
			"autoscaling.k8s.elastic.co",
			"beat.k8s.elastic.co",
			"elasticsearch.k8s.elastic.co",
			"enterprise-search.k8s.elastic.co",
			"kibana.k8s.elastic.co",
			"logstash.k8s.elastic.co",
			"maps.k8s.elastic.co",
			"stackconfigpolicy.k8s.elastic.co",
		}, crd.Spec.Group)
}

func (wh *crdDeletionWebhook) isInUse(crd *extensionsv1.CustomResourceDefinition) bool {
	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   crd.GroupVersionKind().Group,
		Kind:    crd.GroupVersionKind().Kind,
		Version: crd.GroupVersionKind().Version,
	})
	whlog.Info("Checking if CRD is in use", "crd", crd.Name, "group", crd.GroupVersionKind().Group, "version", crd.GroupVersionKind().Version, "kind", crd.GroupVersionKind().Kind)
	for _, ns := range wh.managedNamespace.AsSlice() {
		err := wh.client.List(context.Background(), ul, client.InNamespace(ns))
		if err != nil {
			whlog.Error(err, "Failed to list resources", "namespace", ns)
			return true
		}
		if len(ul.Items) > 0 {
			return true
		}
	}
	return false
}
