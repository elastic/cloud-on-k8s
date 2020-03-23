// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package webhook

import (
	"time"

	pkgerrors "github.com/pkg/errors"
	"k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	ControllerName = "webhook-certificates-controller"
)

var log = logf.Log.WithName(ControllerName)

var _ reconcile.Reconciler = &ReconcileWebhookResources{}

// ReconcileWebhookResources reconciles the certificates used by the webhook server.
type ReconcileWebhookResources struct {
	// k8s.Client is used to watch resources
	k8s.Client
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64

	// webhook parameters
	webhookParams Params
	// resources are updated with a native Kubernetes client
	clientset kubernetes.Interface
}

func (r *ReconcileWebhookResources) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "validating_webhook_configuration", &r.iteration)()
	res := r.reconcileInternal()
	return res.Aggregate()
}

func (r *ReconcileWebhookResources) reconcileInternal() *reconciler.Results {
	res := &reconciler.Results{}
	if err := r.webhookParams.ReconcileResources(r.clientset); err != nil {
		return res.WithError(err)
	}

	// Get the latest content of the webhook CA
	webhookServerSecret, err := r.clientset.CoreV1().Secrets(r.webhookParams.Namespace).Get(r.webhookParams.SecretName, metav1.GetOptions{})
	if err != nil {
		return res.WithError(err)
	}
	serverCA := certificates.BuildCAFromSecret(*webhookServerSecret)
	if serverCA == nil {
		return res.WithError(
			pkgerrors.Errorf("cannot find CA in webhook secret %s/%s", r.webhookParams.Namespace, r.webhookParams.SecretName),
		)
	}

	res.WithResult(reconcile.Result{
		RequeueAfter: certificates.ShouldRotateIn(time.Now(), serverCA.Cert.NotAfter, r.webhookParams.Rotation.RotateBefore),
	})
	return res
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, webhookParams Params, clientset kubernetes.Interface) *ReconcileWebhookResources {
	c := k8s.WrapClient(mgr.GetClient())
	return &ReconcileWebhookResources{
		Client:        c,
		webhookParams: webhookParams,
		clientset:     clientset,
	}
}

// Add adds a new Controller to mgr with r as the reconcile.Reconciler
func Add(mgr manager.Manager, webhookParams Params, clientset kubernetes.Interface) error {
	r := newReconciler(mgr, webhookParams, clientset)
	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	secret := types.NamespacedName{
		Namespace: webhookParams.Namespace,
		Name:      webhookParams.SecretName,
	}

	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &watches.NamedWatch{
		Name:    "webhook-server-cert",
		Watched: []types.NamespacedName{secret},
		Watcher: secret,
	}); err != nil {
		return err
	}

	webhookConfiguration := types.NamespacedName{
		Name: webhookParams.WebhookConfigurationName,
	}

	if err := c.Watch(&source.Kind{Type: &v1beta1.ValidatingWebhookConfiguration{}}, &watches.NamedWatch{
		Name:    "validatingwebhookconfiguration",
		Watched: []types.NamespacedName{webhookConfiguration},
		Watcher: webhookConfiguration,
	}); err != nil {
		return err
	}

	return nil
}
